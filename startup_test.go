package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/r404r/mygonvim/internal/fontcfg"
	"github.com/r404r/mygonvim/internal/loginenv"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

func TestPrepareEnvironmentPortableOverridesAndSettingsPreserved(t *testing.T) {
	data := t.TempDir()
	base := []string{"PATH=/bin", "XDG_CONFIG_HOME=/inherited", "MYGONVIM_NVIM=/chosen", "MYGONVIM_LOGIN_ENV_PROBE=1"}
	out := prepareEnvironment(context.Background(), loginenv.Options{Inputs: loginenv.Inputs{GOOS: "darwin", Parent: 1, Env: base}, Run: func(context.Context, loginenv.Request) loginenv.Output {
		t.Fatal("nested probe executed")
		return loginenv.Output{}
	}}, data)
	if loginenv.Value(out.Env, "XDG_CONFIG_HOME") != filepath.Join(data, "config") || loginenv.Value(out.Env, "MYGONVIM_NVIM") != "/chosen" {
		t.Fatal("portable or explicit override lost")
	}
	path := filepath.Join(data, "settings.json")
	if err := os.WriteFile(path, []byte(`{"loginEnv":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fontcfg.Save(path, fontcfg.Choice{Family: "monospace", Size: 14}); err != nil {
		t.Fatal(err)
	}
	if !loginenv.SettingsOff(path) {
		t.Fatal("font save removed loginEnv=false")
	}
}

func TestImportPortableAndFailureLocateIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	dir := t.TempDir()
	shell := filepath.Join(dir, "bash")
	bin := filepath.Join(dir, "nvim")
	for _, path := range []string{shell, bin} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	base := append(os.Environ(), "SHELL="+shell, "MYGONVIM_LOGIN_ENV=1", "MYGONVIM_NVIM=/chosen")
	data := t.TempDir()
	for _, failed := range []bool{false, true} {
		out := prepareEnvironment(context.Background(), loginenv.Options{Inputs: loginenv.Inputs{GOOS: "darwin", Env: base}, Home: dir, Run: func(_ context.Context, r loginenv.Request) loginenv.Output {
			if loginenv.Value(r.Env, "XDG_CONFIG_HOME") == filepath.Join(data, "config") {
				t.Fatal("shell inherited portable rewrites")
			}
			if failed {
				return loginenv.Output{Failure: "process unavailable"}
			}
			mark := strings.Fields(r.Args[3])[2]
			return loginenv.Output{Stdout: []byte(mark + "PATH=" + dir + "\x00XDG_CONFIG_HOME=/shell\x00MYGONVIM_NVIM=/shell\x00" + mark)}
		}}, data)
		if loginenv.Value(out.Env, "XDG_CONFIG_HOME") != filepath.Join(data, "config") || loginenv.Value(out.Env, "MYGONVIM_NVIM") != "/chosen" {
			t.Fatal("shell overrode protected values")
		}
		got, err := nvimproc.LocateIn("", "", nil, out.Env)
		if !failed {
			if err != nil || got != bin {
				t.Fatal("merged PATH not used")
			}
		} else {
			want, wantErr := nvimproc.Locate("", "", nil)
			if got != want || (err == nil) != (wantErr == nil) {
				t.Fatal("fallback changed lookup")
			}
		}
	}
}

func TestStartupCloseCancelsAndJoinsWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var finished atomic.Bool
	s := &startup{cancel: cancel, done: make(chan struct{}), late: &lateBackend{}}
	go func() { <-ctx.Done(); finished.Store(true); close(s.done) }()
	s.Close()
	if !finished.Load() {
		t.Fatal("close returned while startup worker remained active")
	}
	// A repeated close is safe, including after normal process exit.
	s.Close()
}

func TestLateBackendReplaysLatestResize(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		bin, err = exec.LookPath("nvim")
		if err != nil {
			t.Skip("Neovim not installed")
		}
	}
	sess, err := nvimproc.Start(nvimproc.Options{Bin: bin, Args: []string{"--clean", "-n"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	late := &lateBackend{}
	late.Resize(90, 30)
	late.Resize(123, 41)
	late.Command("let g:startup_queued = 42")
	late.attach(sess)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var size []int
		if err := sess.Nvim().ExecLua("return {vim.o.columns, vim.o.lines, vim.g.startup_queued or 0}", &size); err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(size, []int{123, 41, 42}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("latest pre-session resize never reached Neovim")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
