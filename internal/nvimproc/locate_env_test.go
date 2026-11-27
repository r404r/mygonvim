package nvimproc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocateInUsesChildPathWithoutChangingProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows retains native PATH lookup")
	}
	process := os.Getenv("PATH")
	dir := t.TempDir()
	bin := filepath.Join(dir, "nvim")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := LocateIn("", "", nil, []string{"PATH=" + dir})
	if err != nil || got != bin {
		t.Fatalf("child PATH did not locate executable: %v", err)
	}
	if os.Getenv("PATH") != process {
		t.Fatal("mutated GUI environment")
	}
	// Explicit and bundled paths still take precedence over the imported PATH.
	got, err = LocateIn(bin, "/missing", nil, []string{"PATH=/missing"})
	if err != nil || got != bin {
		t.Fatal("explicit executable lost priority")
	}
	app := t.TempDir()
	bundled := filepath.Join(app, "nvim", "bin", "nvim")
	if err := os.MkdirAll(filepath.Dir(bundled), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundled, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err = LocateIn("", "", []string{app}, []string{"PATH=" + dir})
	if err != nil || got != bundled {
		t.Fatal("bundled executable lost priority")
	}
}

func TestLocateInInheritedPathMatchesExistingLookup(t *testing.T) {
	for _, explicit := range []string{"", "missing-nvim-test"} {
		want, wantErr := Locate(explicit, "", nil)
		got, err := LocateIn(explicit, "", nil, os.Environ())
		if got != want || (err == nil) != (wantErr == nil) {
			t.Fatalf("inherited lookup changed for %q", explicit)
		}
	}
}

func TestChildPathChecksExecutableAndRejectsRelativeEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable lookup")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nvim"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := lookPathIn("nvim", dir); err == nil {
		t.Fatal("non-executable accepted")
	}
	if err := os.Chmod(filepath.Join(dir, "nvim"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := lookPathIn("nvim", ":/missing"); !errors.Is(err, exec.ErrDot) {
		t.Fatalf("relative PATH entry accepted: %v", err)
	}
}
