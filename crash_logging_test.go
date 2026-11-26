package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

func TestCrashLogChild(t *testing.T) {
	dir := os.Getenv("MYGONVIM_TEST_CRASH_DIR")
	if dir == "" {
		return
	}
	// Install the crash descriptor only in this expendable child process.
	if err := enableCrashLog(dir); err != nil {
		t.Fatal(err)
	}
	panic("crash log subprocess marker")
}

func TestCrashLogCapturesPanicAndAppends(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data", "mygonvim", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "crash.log")
	const previous = "previous crash must survive\n"
	if err := os.WriteFile(path, []byte(previous), 0o600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var earlier []byte
	for range 2 {
		cmd := exec.Command(exe, "-test.run=^TestCrashLogChild$")
		cmd.Env = append(os.Environ(), "MYGONVIM_TEST_CRASH_DIR="+dir, "GOTRACEBACK=single")
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("child did not panic: %s", out)
		} else if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("cannot run crash child: %v: %s", err, out)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(got, []byte(previous)) || !bytes.Contains(got, []byte("panic: crash log subprocess marker")) {
			t.Fatalf("crash output missing or overwritten: %s", got)
		}
		if earlier != nil && (!bytes.HasPrefix(got, earlier) || len(got) <= len(earlier)) {
			t.Fatalf("second crash did not append: %s", got)
		}
		earlier = got
	}
	if _, err := os.Stat(filepath.Join(dir, "mygonvim.log")); !os.IsNotExist(err) {
		t.Fatalf("crash output created an application log: %v", err)
	}
}

func TestCrashLoggingFailureAllowsStartup(t *testing.T) {
	oldWriter := log.Writer()
	oldPath, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(oldWriter); mygo.App.SetPath(mygo.PathLogs, oldPath) })
	dir := t.TempDir()
	// A directory in place of the file makes opening it fail on all platforms.
	if err := os.Mkdir(filepath.Join(dir, "crash.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathLogs, dir)
	var out bytes.Buffer
	log.SetOutput(&out)
	startCrashLogging()
	logStartup("0.0.8", true, "nvim.exe", filepath.Join(dir, "mygonvim.log"))
	if got := out.String(); !strings.Contains(got, "logs: cannot enable crash log:") || !strings.Contains(got, "startup:") {
		t.Fatalf("startup did not continue after crash file failure: %s", got)
	}
}

func TestStartupLogRecordsProcessPaths(t *testing.T) {
	oldWriter := log.Writer()
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	for _, portable := range []bool{false, true} {
		var out bytes.Buffer
		log.SetOutput(&out)
		logStartup("0.0.8", portable, "C:\\Program Files\\Neovim\\nvim.exe", "C:\\app\\logs\\mygonvim.log")
		want := `startup: version="0.0.8" portable=false nvim="C:\\Program Files\\Neovim\\nvim.exe" log="C:\\app\\logs\\mygonvim.log"`
		if portable {
			want = strings.Replace(want, "portable=false", "portable=true", 1)
		}
		if got := out.String(); !strings.Contains(got, want) || strings.Count(got, "\n") != 1 {
			t.Fatalf("startup log = %q, want one line containing %q", got, want)
		}
	}
}
