package logfile

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileAppendAndBound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("old\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, Name)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old\nnew\n" {
		t.Fatalf("append: %q %v", got, err)
	}
	large := bytes.Repeat([]byte("x"), MaxSize)
	if _, err := f.Write(large); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("latest\n")); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil || string(got) != "latest\n" {
		t.Fatalf("truncate: length=%d, want only latest line: %v", len(got), err)
	}
	large = append(large, []byte("tail")...)
	if n, err := f.Write(large); n != len(large) || err != nil {
		t.Fatalf("oversized write: %d %v", n, err)
	}
	got, err = os.ReadFile(path)
	if err != nil || len(got) != MaxSize || !bytes.HasSuffix(got, []byte("tail")) {
		t.Fatalf("bound: length=%d %v", len(got), err)
	}
}

func TestExistingOversizedLogAndConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Name)
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), MaxSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if st, err := os.Stat(path); err != nil || st.Size() != 0 {
		t.Fatalf("startup bound: %v %v", st, err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				if _, err := f.Write(bytes.Repeat([]byte("log\n"), 1024)); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if st, err := os.Stat(path); err != nil || st.Size() > MaxSize || st.Size() == 0 {
		t.Fatalf("concurrent bound: %v %v", st, err)
	}
}

func TestOpenFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(dir, Name, "logs")); err == nil {
		t.Fatal("accepted a file as a parent directory")
	}
}
