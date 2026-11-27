//go:build unix

package loginenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCollectorFailuresAndLogoutExit(t *testing.T) {
	shell := fakeShell(t)
	for _, tc := range []struct {
		name, collector, after string
		success                bool
	}{
		{"success", "/usr/bin/env -0", "", true},
		{"collector fails empty", "false", "", false},
		{"collector fails halfway", "{ printf 'PARTIAL=value\\000'; false; }", "", false},
		{"logout fails after snapshot", "/usr/bin/env -0", "; exit 17", true},
		{"shell exits before command", "exit 7", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: []string{"SHELL=" + shell, "PATH=/usr/bin:/bin", "CAPTURE_TEST=yes"}}, Home: t.TempDir(), Run: func(ctx context.Context, r Request) Output {
				r.Path = "/bin/sh"
				script := strings.Replace(r.Args[3], "/usr/bin/env -0", tc.collector, 1) + tc.after
				r.Args = []string{"-c", script}
				return runProcess(ctx, r)
			}})
			if (out.Status == "Imported") != tc.success {
				t.Fatalf("status=%s", out.Status)
			}
			if !tc.success && Value(out.Env, "PARTIAL") != "" {
				t.Fatal("partial environment imported")
			}
			if tc.success && Value(out.Env, "CAPTURE_TEST") != "yes" {
				t.Fatal("complete snapshot lost")
			}
		})
	}
}

func TestProcessNullStdinHomeAndProbe(t *testing.T) {
	shell := fakeShell(t)
	home := t.TempDir()
	physicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	out := Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: []string{"SHELL=" + shell, "PATH=/usr/bin:/bin"}}, Home: home, Run: func(ctx context.Context, r Request) Output {
		// Test the actual descriptor as well as EOF, before running the collector.
		r.Path = "/bin/sh"
		r.Args = []string{"-c", `[ /dev/stdin -ef /dev/null ] && ! read -r value && export PROBE_CWD="$PWD" PROBE_SEEN="$MYGONVIM_LOGIN_ENV_PROBE" && ` + r.Args[3]}
		return runProcess(ctx, r)
	}})
	if out.Status != "Imported" || Value(out.Env, "PROBE_CWD") != physicalHome || Value(out.Env, "PROBE_SEEN") != "1" {
		t.Fatalf("descriptor, cwd, or probe incorrect: status=%s", out.Status)
	}
	if Value(out.Env, "MYGONVIM_LOGIN_ENV_PROBE") != "" {
		t.Fatal("probe marker escaped into Neovim environment")
	}
}

func TestProcessOutputLimitBothStreams(t *testing.T) {
	for _, redirect := range []string{"", " >&2"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out := runProcess(ctx, Request{Path: "/bin/sh", Args: []string{"-c", "while :; do printf 'FAKE_SECRET_1234567890123456789012345678901234567890\\n'" + redirect + "; done"}, Env: []string{"PATH=/usr/bin:/bin"}})
		cancel()
		if out.Failure != "output limit" || len(out.Stdout) > outputLimit {
			t.Fatalf("failure=%s bytes=%d", out.Failure, len(out.Stdout))
		}
	}
}

func TestTimeoutKillsGrandchild(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	out := runProcess(ctx, Request{Path: "/bin/sh", Args: []string{"-c", `/bin/sh -c 'sleep 30 & echo $! > "$PIDFILE"; wait' & wait`}, Env: []string{"PATH=/usr/bin:/bin", "PIDFILE=" + pidfile}})
	if out.Failure != "cancelled" {
		t.Fatalf("failure=%s", out.Failure)
	}
	data, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal("grandchild never started:", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatal("invalid grandchild pid")
	}
	assertProcessStopped(t, pid)
}

func assertProcessStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || len(strings.TrimSpace(string(out))) == 0 || strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant remained alive after process group cleanup")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBackgroundStdoutDoesNotDelayStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	out := runProcess(ctx, Request{Path: "/bin/sh", Args: []string{"-c", `sleep 30 & printf 'MARKOK=yes\000MARK'`}, Env: []string{"PATH=/usr/bin:/bin"}})
	if time.Since(start) > 2*time.Second || out.Failure != "" {
		t.Fatalf("background stdout stalled: %s", out.Failure)
	}
	if vars, err := Parse(out.Stdout, "MARK"); err != nil || vars["OK"] != "yes" {
		t.Fatal("complete snapshot lost on WaitDelay")
	}
}

func TestImportDeadlineAndCancellationFallback(t *testing.T) {
	shell := fakeShell(t)
	for _, cancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		out := Import(ctx, Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: []string{"SHELL=" + shell, "KEEP=old"}}, Home: t.TempDir(), Run: func(ctx context.Context, _ Request) Output {
			return runProcess(ctx, Request{Path: "/bin/sh", Args: []string{"-c", "sleep 30"}, Env: []string{"PATH=/usr/bin:/bin"}})
		}})
		cancel()
		want := "Fallback: timeout"
		if cancelled {
			want = "Fallback: cancelled"
		}
		if out.Status != want || Value(out.Env, "KEEP") != "old" {
			t.Fatalf("status=%s", out.Status)
		}
	}
}
