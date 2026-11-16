package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/r404r/mygonvim/internal/logfile"
)

type unavailableStderr struct{}

func (unavailableStderr) Write([]byte) (int, error) { return 0, errors.New("no console") }

func TestLoggingPersistsWithoutConsole(t *testing.T) {
	oldWriter := log.Writer()
	oldPath, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(oldWriter); mygo.App.SetPath(mygo.PathLogs, oldPath) })
	for _, path := range []string{filepath.Join(t.TempDir(), "app-logs"), filepath.Join(t.TempDir(), "data", "mygonvim", "logs")} {
		mygo.App.SetPath(mygo.PathLogs, path)
		file := startLogging(unavailableStderr{})
		if file == nil {
			t.Fatal("no log file")
		}
		log.Print("persist this without stderr")
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(path, logfile.Name))
		if err != nil || !strings.Contains(string(got), "persist this without stderr") {
			t.Fatalf("persistent log: %s, %v", got, err)
		}
	}
}

func TestLoggingFallsBackWhenFileCannotOpen(t *testing.T) {
	oldWriter := log.Writer()
	oldPath, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(oldWriter); mygo.App.SetPath(mygo.PathLogs, oldPath) })
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathLogs, filepath.Join(blocker, "logs"))
	var stderr bytes.Buffer
	if f := startLogging(&stderr); f != nil {
		f.Close()
		t.Fatal("expected file open failure")
	}
	log.Print("still running")
	if got := stderr.String(); !strings.Contains(got, "logs: cannot open log file") || !strings.Contains(got, "still running") {
		t.Fatalf("fallback log: %s", got)
	}
}

func TestLoggingReachesStderrWhenFileWriteFails(t *testing.T) {
	oldWriter := log.Writer()
	oldPath, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(oldWriter); mygo.App.SetPath(mygo.PathLogs, oldPath) })
	mygo.App.SetPath(mygo.PathLogs, t.TempDir())
	var stderr bytes.Buffer
	file := startLogging(&stderr)
	if file == nil {
		t.Fatal("no log file")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	log.Print("still visible after file write failure")
	if got := stderr.String(); !strings.Contains(got, "still visible after file write failure") {
		t.Fatalf("stderr log: %q", got)
	}
}
