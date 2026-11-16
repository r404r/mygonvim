package main

import (
	"io"
	"log"

	"github.com/egoist/mygo"
	"github.com/r404r/mygonvim/internal/logfile"
)

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
