package about

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/neovim/go-client/nvim"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

type blockedLua struct{ entered, release chan struct{} }

func (b *blockedLua) ExecLua(_ string, _ interface{}, _ ...interface{}) error {
	close(b.entered)
	<-b.release
	return errors.New("closed")
}

func TestQueryTimeoutBoundsOutstandingRequests(t *testing.T) {
	b := &blockedLua{make(chan struct{}), make(chan struct{})}
	q := &Query{Client: b}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := q.Load(ctx); done <- err }()
	<-b.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("timeout=%v", err)
	}
	if _, err := q.Load(context.Background()); err == nil {
		t.Fatal("second request accepted while first is pending")
	}
	close(b.release)
}

func TestQueryUsesRunningNeovimWithAppNameAndPortablePaths(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		bin, err = exec.LookPath("nvim")
		if err != nil {
			t.Skip("nvim unavailable")
		}
	}
	for _, portable := range []bool{false, true} {
		t.Run(map[bool]string{false: "appname", true: "portable"}[portable], func(t *testing.T) {
			root := t.TempDir()
			// Test both a custom app name and the environment layout used by portable mode.
			t.Setenv("NVIM_APPNAME", "about-test")
			for _, kind := range []string{"CONFIG", "DATA", "STATE", "CACHE"} {
				t.Setenv("XDG_"+kind+"_HOME", filepath.Join(root, kind))
			}
			env := os.Environ()
			configRoot := filepath.Join(root, "CONFIG")
			if portable {
				env = nvimproc.PortableEnv(env, root)
				configRoot = filepath.Join(root, "config")
			}
			nv, err := nvim.NewChildProcess(nvim.ChildProcessEnv(env), nvim.ChildProcessCommand(bin), nvim.ChildProcessArgs("--embed", "--headless", "-u", "NONE", "-i", "NONE"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { nv.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			q := &Query{Client: nv}
			info, err := q.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if info.Version == "" {
				t.Fatal("no version")
			}
			var want map[string]string
			if err := nv.ExecLua("return {config=vim.fn.stdpath('config'),data=vim.fn.stdpath('data'),state=vim.fn.stdpath('state'),cache=vim.fn.stdpath('cache')}", &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(info.Paths, want) {
				t.Fatalf("paths=%v want=%v", info.Paths, want)
			}
			if info.Paths["config"] != filepath.Join(configRoot, os.Getenv("NVIM_APPNAME")) {
				t.Fatalf("wrong app config: %s", info.Paths["config"])
			}
			if err := nv.ExecLua("local f=vim.fn.stdpath; vim.fn.stdpath=function(k) if k=='state' then error('state unavailable') end return f(k) end", nil); err != nil {
				t.Fatal(err)
			}
			info, err = q.Load(ctx)
			if err != nil || info.Paths["config"] != want["config"] || info.Errors["state"] == "" || info.Paths["state"] != "" {
				t.Fatalf("partial failure: %+v %v", info, err)
			}
		})
	}
}
