package main

import (
	"context"
	"errors"
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
	late := &lateBackend{}
	late.Resize(90, 30)
	sess, err := late.startSession(context.Background(), nvimproc.Options{Bin: testNvimBin(t), Args: []string{"--clean", "-n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	// A change between the initial UI attachment and backend publication must
	// still reach Neovim before any queued input.
	late.Resize(123, 41)
	late.Command("let g:startup_queued = [&columns, &lines]")
	if !late.attach(sess) {
		t.Fatal("session not attached")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var size []int
		if err := sess.Nvim().ExecLua("return vim.g.startup_queued or {}", &size); err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(size, []int{123, 41}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("latest pre-session resize never reached Neovim")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func testNvimBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		bin, err = exec.LookPath("nvim")
		if err != nil {
			t.Skip("Neovim not installed")
		}
	}
	return bin
}

func startTestNvim(t *testing.T) *nvimproc.Session {
	t.Helper()
	bin := testNvimBin(t)
	sess, err := nvimproc.Start(nvimproc.Options{Bin: bin, Args: []string{"--clean", "-n"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	return sess
}

func waitStartupBuffer(t *testing.T, sess *nvimproc.Session, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		lines, err := sess.Nvim().BufferLines(0, 0, -1, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) == 1 && string(lines[0]) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending inputs not replayed in order (want %d bytes)", len(want))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func flushStartupInput(t *testing.T, late *lateBackend, sess *nvimproc.Session) {
	t.Helper()
	if err := sess.Nvim().ExecLua(`vim.keymap.set({'n','i'}, '<F12>', function() vim.g.startup_flushed = 1 end)`, nil); err != nil {
		t.Fatal(err)
	}
	late.Input("<F12>")
	deadline := time.Now().Add(10 * time.Second)
	for {
		var done int
		if err := sess.Nvim().ExecLua("return vim.g.startup_flushed or 0", &done); err != nil {
			t.Fatal(err)
		}
		if done == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("startup input barrier never reached Neovim")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLateBackendReplaysInputAndPasteInArrivalOrder(t *testing.T) {
	sess := startTestNvim(t)
	if err := sess.Nvim().ExecLua(`vim.keymap.set('n', '<F8>', function() vim.g.startup_size = {vim.o.columns, vim.o.lines} end)`, nil); err != nil {
		t.Fatal(err)
	}
	late := &lateBackend{}
	late.Resize(111, 33)
	late.Input("<F8>iA")
	late.Paste("B")
	late.Input("C")
	late.Paste("D")
	if !late.attach(sess) {
		t.Fatal("session not attached")
	}
	// Newly arriving input must follow the pending sequence as well.
	late.Input("E")
	late.Paste("F")
	flushStartupInput(t, late, sess)
	waitStartupBuffer(t, sess, "ABCDEF")
	var size []int
	if err := sess.Nvim().ExecLua("return vim.g.startup_size", &size); err != nil || !reflect.DeepEqual(size, []int{111, 33}) {
		t.Fatal("pending input ran before the saved resize")
	}
}

func TestLateBackendPendingLimits(t *testing.T) {
	for _, kind := range []string{"bytes", "operations"} {
		t.Run(kind, func(t *testing.T) {
			sess := startTestNvim(t)
			late := &lateBackend{}
			late.Input("i")
			want := ""
			if kind == "bytes" {
				want = strings.Repeat("x", maxPendingBytes/2)
				late.Paste(want)
				late.Paste(want) // The complete second operation exceeds the byte cap.
			} else {
				want = strings.Repeat("x", maxPendingInputs-1)
				for i := 0; i < maxPendingInputs+10; i++ {
					late.Input("x")
				}
			}
			if !late.attach(sess) {
				t.Fatal("session not attached")
			}
			flushStartupInput(t, late, sess)
			waitStartupBuffer(t, sess, want)
		})
	}
}

func TestStartupFailureLeavesOnlyErrorWindowAndDiscardsInput(t *testing.T) {
	sess := startTestNvim(t)
	late := &lateBackend{}
	late.Input("iBAD")
	late.Paste("PASTE")
	late.Command("let g:should_not_run = 1")
	windows := 1
	errExpected := errors.New("cannot start")
	startupFailure(late, errExpected, func(err error) {
		if err != errExpected {
			t.Fatal("incorrect error window")
		}
		windows++
	}, func() {
		if windows != 2 {
			t.Fatal("last window closed before replacement opened")
		}
		windows--
	})
	late.Input("iAFTER")
	late.Paste("AFTER")
	if late.attach(sess) {
		t.Fatal("failed startup replayed input to another session")
	}
	waitStartupBuffer(t, sess, "")
	var command int
	if err := sess.Nvim().ExecLua("return vim.g.should_not_run or 0", &command); err != nil || command != 0 {
		t.Fatal("failed startup replayed a command")
	}
	if windows != 1 {
		t.Fatal("empty editor remained beside the error window")
	}
}

func TestStartupClosePreservesVimLeavePre(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess, err := nvimproc.StartContext(ctx, nvimproc.Options{Bin: testNvimBin(t), Args: []string{"--clean", "-n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	marker := filepath.Join(t.TempDir(), "leave.txt")
	if err := sess.Nvim().ExecLua(`local path = ...; vim.api.nvim_create_autocmd('VimLeavePre', {callback = function() vim.fn.writefile({'left'}, path) end})`, nil, marker); err != nil {
		t.Fatal(err)
	}
	late := &lateBackend{}
	late.attach(sess)
	done := make(chan struct{})
	close(done)
	(&startup{cancel: cancel, done: done, late: late}).Close()
	if data, err := os.ReadFile(marker); err != nil || string(data) != "left\n" {
		t.Fatal("startup.Close skipped VimLeavePre")
	}
}
