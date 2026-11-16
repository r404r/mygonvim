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
	// A GUI launch may have an unusable stderr. Write the file first so a
	// failure of stderr cannot prevent the persistent copy from being written.
	log.SetOutput(io.MultiWriter(f, stderr))
	return f
}
