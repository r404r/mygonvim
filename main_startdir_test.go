package main

import (
	"errors"
	"testing"
)

func TestNvimStartDir(t *testing.T) {
	lookupErr := errors.New("directory unavailable")
	for _, tt := range []struct {
		name    string
		goos    string
		cwd     string
		cwdErr  error
		home    string
		homeErr error
		want    string
	}{
		{name: "macOS root", goos: "darwin", cwd: "/", home: "/Users/editor", want: "/Users/editor"},
		{name: "macOS project", goos: "darwin", cwd: "/Users/editor/project", home: "/Users/editor"},
		{name: "Linux root", goos: "linux", cwd: "/", home: "/home/editor"},
		{name: "Windows root", goos: "windows", cwd: "/", home: `C:\Users\editor`},
		{name: "home lookup error", goos: "darwin", cwd: "/", home: "/Users/editor", homeErr: lookupErr},
		{name: "empty home", goos: "darwin", cwd: "/"},
		{name: "cwd lookup error", goos: "darwin", cwd: "/", cwdErr: lookupErr, home: "/Users/editor"},
		{name: "empty cwd", goos: "darwin", home: "/Users/editor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := nvimStartDir(tt.goos, tt.cwd, tt.cwdErr, tt.home, tt.homeErr); got != tt.want {
				t.Errorf("nvimStartDir = %q, want %q", got, tt.want)
			}
		})
	}
}
