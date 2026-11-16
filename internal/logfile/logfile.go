// Package logfile provides a bounded application log for GUI launches.
package logfile

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	Name    = "mygonvim.log"
	MaxSize = 1024 * 1024
)

// File appends logs and starts over when the next write would exceed MaxSize.
// Writes and Close may be called from different goroutines.
type File struct {
	mu   sync.Mutex
	f    *os.File
	size int64
}

func Open(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, Name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err == nil && size > MaxSize {
		err = f.Truncate(0)
		if err == nil {
			size, err = f.Seek(0, io.SeekStart)
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return &File{f: f, size: size}, nil
}

func (f *File) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	original := len(p)
	if len(p) > MaxSize {
		p = p[len(p)-MaxSize:]
	}
	if f.size+int64(len(p)) > MaxSize {
		if err := f.f.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := f.f.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		f.size = 0
	}
	n, err := f.f.Write(p)
	f.size += int64(n)
	if err != nil {
		return n, err
	}
	return original, nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.f.Close()
}
