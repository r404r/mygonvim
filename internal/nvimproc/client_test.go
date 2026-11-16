package nvimproc

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neovim/go-client/msgpack/rpc"
	"github.com/neovim/go-client/nvim"

	"github.com/r404r/mygonvim/internal/logfile"
)

func requiredNvim(t *testing.T) string {
	t.Helper()
	bin, err := Locate(os.Getenv("NVIM_BIN"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestClientInfoAgainstRealNvim(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    nvim.ClientVersion
	}{
		{"dev", nvim.ClientVersion{Prerelease: "dev"}},
		{"0.0.3", nvim.ClientVersion{Patch: 3}},
		{"v1.2.3-beta.1+build", nvim.ClientVersion{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.1"}},
	} {
		t.Run(tc.version, func(t *testing.T) {
			s, err := Start(Options{
				Bin: requiredNvim(t), Args: []string{"--clean"}, Version: tc.version,
				Env:      PortableEnv(os.Environ(), t.TempDir()),
				InitCmds: []string{"autocmd UIEnter * let g:client_at_attach = nvim_get_chan_info(v:event.chan).client"},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ch, err := s.Nvim().ChannelInfo(s.Nvim().ChannelID())
			if err != nil {
				t.Fatal(err)
			}
			if ch.Client == nil {
				t.Fatal("no client identification")
			}
			var atAttach nvim.Client
			if err := s.Nvim().Var("client_at_attach", &atAttach); err != nil {
				t.Fatal(err)
			}
			for _, client := range []*nvim.Client{ch.Client, &atAttach} {
				if client.Name != "mygonvim" || client.Type != nvim.UIClientType || client.Version != tc.want {
					t.Fatalf("client = %+v, want mygonvim/ui/%+v", client, tc.want)
				}
				if client.Attributes["website"] != "https://github.com/r404r/mygonvim" || client.Attributes["license"] != "MIT" {
					t.Fatalf("attributes = %v", client.Attributes)
				}
			}
		})
	}
}

func TestClientVersionFallback(t *testing.T) {
	for _, version := range []string{"snapshot", "1.2", "1.-2.3", "1.x.3"} {
		if got := clientVersion(version); got != (nvim.ClientVersion{Prerelease: version}) {
			t.Fatalf("%q: %+v", version, got)
		}
	}
	if got := clientVersion(""); got.Prerelease != "dev" {
		t.Fatalf("empty: %+v", got)
	}
}

func waitLog(t *testing.T, path string, fragments ...string) string {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var text string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text = string(b)
		found := true
		for _, fragment := range fragments {
			found = found && strings.Contains(text, fragment)
		}
		if found {
			return text
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing %q in log: %s", fragments, text)
	return ""
}

func TestRequestErrorReachesLogFile(t *testing.T) {
	dir := t.TempDir()
	f, err := logfile.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := Start(Options{Bin: requiredNvim(t), Args: []string{"--clean"}, Env: PortableEnv(os.Environ(), t.TempDir()), Logf: log.New(f, "", 0).Printf})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Nvim().InputMouse("bogus", "press", "", 0, 0, 0); err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("request error = %v", err)
	}
	s.Mouse("bogus", "press", "", 0, 0)
	text := waitLog(t, filepath.Join(dir, logfile.Name), "nvim_input_mouse request failed", "validation", "invalid button or action")
	if strings.Contains(text, "nvim_error_event:") {
		t.Fatalf("request unexpectedly produced notification: %s", text)
	}
	s.Command("NoSuchEditorCommand")
	waitLog(t, filepath.Join(dir, logfile.Name), "nvim_command request failed", "exception", "E492")
}

func TestNotificationErrorReachesLogFile(t *testing.T) {
	dir := t.TempDir()
	f, err := logfile.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := Start(Options{Bin: requiredNvim(t), Args: []string{"--clean"}, Env: PortableEnv(os.Environ(), t.TempDir()), Logf: log.New(f, "", 0).Printf})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var address string
	if err := s.Nvim().Call("serverstart", &address, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ep, err := rpc.NewEndpoint(conn, conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.registerErrors(func(name string, handler any) error { return ep.Register(name, handler) }); err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- ep.Serve() }()
	defer func() { ep.Close(); <-served }()
	s.calling("nvim_input_mouse")
	// The invalid API notification is sent to Neovim, which generates the
	// nvim_error_event itself; no synthetic error event is injected.
	if err := ep.Notify("nvim_input_mouse", "bogus", "press", "", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	waitLog(t, filepath.Join(dir, logfile.Name), "nvim_error_event: type=1", "invalid button or action", "recent_call_hint=\"nvim_input_mouse\"", "not a request ID")
}
