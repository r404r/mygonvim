package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/egoist/mygo"
	"github.com/r404r/mygonvim/internal/logfile"
)

func logStartup(version string, portable bool, nvim, path string) {
	log.Printf("startup: version=%q portable=%t nvim=%q log=%q", version, portable, nvim, path)
}

// startCrashLogging runs only after the application's portable paths are set.
func startCrashLogging() {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err == nil {
		err = enableCrashLog(dir)
	}
	if err != nil {
		log.Printf("logs: cannot enable crash log: %v", err)
	}
}

func enableCrashLog(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	// SetCrashOutput duplicates the descriptor; the runtime keeps its copy.
	defer f.Close()
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

func startLogging(stderr io.Writer) io.Closer {
	log.SetOutput(stderr)
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		log.Printf("logs: no log folder: %v", err)
		return nil
	}
	f, err := logfile.Open(dir)
	if err != nil {
		log.Printf("logs: cannot open log file: %v", err)
		return nil
	}
	// Write the file first, but always try both destinations so a failure in
	// either one cannot prevent the other from receiving the log.
	log.SetOutput(teeWriter{f, stderr})
	return f
}

// teeWriter writes to every destination even when an earlier one fails.
type teeWriter []io.Writer

func (t teeWriter) Write(p []byte) (int, error) {
	var first error
	for _, w := range t {
		n, err := w.Write(p)
		if err == nil && n != len(p) {
			err = io.ErrShortWrite
		}
		if first == nil {
			first = err
		}
	}
	return len(p), first
}
