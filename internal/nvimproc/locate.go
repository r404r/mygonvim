// Package nvimproc finds Neovim, starts it with --embed and talks to it over
// msgpack-rpc: the process side of the editor.
package nvimproc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// BinaryName is the nvim executable's file name on this system.
func BinaryName() string { return binaryName(runtime.GOOS) }

func binaryName(goos string) string {
	if goos == "windows" {
		return "nvim.exe"
	}
	return "nvim"
}

// Candidates lists, in the order they are tried, the places nvim is looked
// for, for a program whose own files are in dirs (the executable's directory,
// and the app resources directory when that is another): the -nvim flag and
// $MYGONVIM_NVIM first when given, then a bundled copy in each dir (the
// portable layout is nvim/bin/nvim.exe, the layout of Neovim's zip; nvim/nvim
// also works), then PATH, then the usual install places a GUI app's short PATH
// misses on macOS.
func Candidates(goos, flagPath, envPath string, dirs []string) []string {
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	add(flagPath)
	add(envPath)
	name := binaryName(goos)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		add(filepath.Join(d, "nvim", "bin", name))
		add(filepath.Join(d, "nvim", name))
	}
	out = append(out, "") // "" stands for PATH
	if goos == "darwin" {
		add("/opt/homebrew/bin/nvim")
		add("/usr/local/bin/nvim")
	}
	return out
}

// Locate returns the absolute path of the first usable candidate. An
// explicit -nvim or $MYGONVIM_NVIM that does not exist is an error, not a
// reason to look elsewhere.
func Locate(flagPath, envPath string, dirs []string) (string, error) {
	path, err := locate(runtime.GOOS, flagPath, envPath, dirs, fileExists, exec.LookPath)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func locate(goos, flagPath, envPath string, dirs []string, exists func(string) bool, lookPath func(string) (string, error)) (string, error) {
	for _, explicit := range []string{flagPath, envPath} {
		if explicit == "" {
			continue
		}
		if p, err := lookPath(explicit); err == nil {
			return p, nil
		} else if !exists(explicit) {
			return "", errors.New("nvim not found at " + explicit + ": " + err.Error())
		}
		return explicit, nil
	}
	for _, c := range Candidates(goos, "", "", dirs) {
		if c == "" {
			if p, err := lookPath(binaryName(goos)); err == nil {
				return p, nil
			}
			continue
		}
		if exists(c) {
			return c, nil
		}
	}
	return "", errors.New("nvim not found: install Neovim, put it on PATH, pass -nvim, or place it in an nvim/ folder next to the program")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
