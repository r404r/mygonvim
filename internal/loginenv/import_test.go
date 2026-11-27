package loginenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func fakeShell(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bash")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportUsesOriginalEnvironmentAndBoundedRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell executable mode")
	}
	shell := fakeShell(t)
	base := []string{"SHELL=" + shell, "PATH=/original", "HOME=/original", "MYGONVIM_NVIM=/original", "FAKE_SECRET=do-not-log-this\nsecond-secret-line"}
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprint(debug), func(t *testing.T) {
			env := append([]string{}, base...)
			if debug {
				env = append(env, "MYGONVIM_LOGIN_ENV_DEBUG=1")
			}
			var logs strings.Builder
			calls := 0
			out := Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: env}, Home: "/home", Logf: func(f string, a ...any) { fmt.Fprintf(&logs, f, a...) }, Run: func(ctx context.Context, r Request) Output {
				calls++
				if deadline, ok := ctx.Deadline(); !ok || deadline.IsZero() {
					t.Fatal("missing deadline")
				}
				if r.Path != shell || r.Dir != "/home" || !reflect.DeepEqual(r.Args[:3], []string{"-l", "-i", "-c"}) {
					t.Fatal("wrong shell request")
				}
				if Value(r.Env, "MYGONVIM_LOGIN_ENV_PROBE") != "1" || Value(r.Env, "PATH") != "/original" {
					t.Fatal("probe did not inherit raw environment")
				}
				parts := strings.Fields(r.Args[3])
				marker := parts[2]
				return Output{Stdout: []byte("banner-secret\n" + marker + "PATH=/shell\x00FAKE_SECRET=do-not-log-this\nsecond-secret-line\x00MYGONVIM_NVIM=/bad\x00" + marker + "logout-secret"), ExitCode: 9}
			}})
			if calls != 1 || out.Status != "Imported" || Value(out.Env, "PATH") != "/shell:/original" || Value(out.Env, "MYGONVIM_NVIM") != "/original" {
				t.Fatal("snapshot not imported correctly")
			}
			for _, secret := range []string{"do-not-log-this", "second-secret-line", "banner-secret", "logout-secret", shell, "/shell"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatalf("log disclosed %q", secret)
				}
			}
			if strings.Contains(logs.String(), "FAKE_SECRET") != debug {
				t.Fatal("variable-name debug switch ignored")
			}
		})
	}
}

func TestImportSkipsWithoutRunningAndFallbackIsExact(t *testing.T) {
	base := []string{"PATH=/bin", "MYGONVIM_LOGIN_ENV_PROBE=1", "MYGONVIM_LOGIN_ENV=1"}
	out := Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: base}, Run: func(context.Context, Request) Output { t.Fatal("nested probe ran"); return Output{} }})
	if !reflect.DeepEqual(out.Env, base) || !strings.HasPrefix(out.Status, "Skipped:") {
		t.Fatal("skip changed environment")
	}
	if runtime.GOOS == "windows" {
		return
	}
	base = []string{"PATH=/original", "SHELL=" + fakeShell(t)}
	for _, failure := range []string{"output limit", "process unavailable", ""} {
		var logs strings.Builder
		out = Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: base}, Home: "/home", Run: func(context.Context, Request) Output {
			return Output{Failure: failure, Stdout: []byte("FAKE_SECRET=never-log"), ExitCode: 7}
		}, Logf: func(f string, a ...any) { fmt.Fprintf(&logs, f, a...) }})
		if !reflect.DeepEqual(out.Env, base) || !strings.HasPrefix(out.Status, "Fallback:") {
			t.Fatal("failure did not preserve inherited environment")
		}
		if strings.Contains(logs.String(), "never-log") || strings.Contains(logs.String(), "FAKE_SECRET") {
			t.Fatal("failure log disclosed shell output")
		}
	}
}

func TestImportResolvesUserShellAndRejectsUnsupportedShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell executable mode")
	}
	shell := fakeShell(t)
	calls := 0
	out := Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1}, Home: "/home", Username: "user", Run: func(_ context.Context, r Request) Output {
		calls++
		if r.Path == "/usr/bin/dscl" {
			return Output{Stdout: []byte("UserShell: " + shell + "\n")}
		}
		if r.Path != shell {
			t.Fatal("did not use UserShell")
		}
		marker := strings.Fields(r.Args[3])[2]
		return Output{Stdout: []byte(marker + "USER_SHELL=yes\x00" + marker)}
	}})
	if calls != 2 || out.Status != "Imported" || Value(out.Env, "USER_SHELL") != "yes" {
		t.Fatal("user shell resolution failed")
	}
	fish := filepath.Join(t.TempDir(), "fish")
	if err := os.WriteFile(fish, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	out = Import(context.Background(), Options{Inputs: Inputs{GOOS: "darwin", Parent: 1, Env: []string{"SHELL=" + fish}}, Home: "/home", Run: func(context.Context, Request) Output { t.Fatal("unsupported shell executed"); return Output{} }})
	if out.Status != "Fallback: unsupported shell" {
		t.Fatal(out.Status)
	}
}
