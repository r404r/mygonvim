// Package about presents process information without changing editor state.
package about

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	Website    = "https://github.com/r404r/mygonvim"
	LicenseURL = Website + "/blob/main/LICENSE"
)

type Process struct {
	Version             string
	Portable            bool
	Settings, Log, Nvim string
}

type Info struct {
	Version string            `msgpack:"version"`
	Paths   map[string]string `msgpack:"paths"`
	Errors  map[string]string `msgpack:"errors"`
}

type Path struct {
	Label, Value, Error string
}

// Paths lists only locations reported by this process and its running Neovim.
func Paths(p Process, info Info, stat func(string) (os.FileInfo, error)) []Path {
	out := []Path{{Label: "settings.json", Value: p.Settings}, {Label: "mygonvim.log", Value: p.Log}, {Label: "Neovim executable", Value: p.Nvim}}
	for _, name := range []string{"config", "data", "state", "cache"} {
		out = append(out, Path{Label: "Neovim " + name, Value: info.Paths[name], Error: info.Errors[name]})
		if name == "config" && info.Paths[name] != "" {
			for _, init := range []string{"init.lua", "init.vim"} {
				path := filepath.Join(info.Paths[name], init)
				if fi, err := stat(path); err == nil && !fi.IsDir() {
					out = append(out, Path{Label: init, Value: path})
				}
			}
		}
	}
	return out
}

type Services struct {
	Copy          func(string) error
	Reveal        func(string)
	OpenDirectory func(string) error
	OpenExternal  func(string) error
	Stat          func(string) (os.FileInfo, error)
}

// Act returns visible feedback, including when a location does not exist yet.
func (s Services) Act(action string, p Path) string {
	if p.Value == "" {
		return "Unavailable: no path was reported."
	}
	fi, err := s.Stat(p.Value)
	if action == "copy" {
		if copyErr := s.Copy(p.Value); copyErr != nil {
			return "Copy failed: " + copyErr.Error()
		}
		if err != nil {
			return "Copied path; location unavailable: " + err.Error()
		}
		return "Copied path."
	}
	if err != nil {
		return "Location unavailable: " + err.Error()
	}
	switch action {
	case "reveal":
		s.Reveal(p.Value)
		// MyGo's reveal API has no error result on any desktop platform.
		return "Requested file manager to show the location."
	case "open":
		dir := p.Value
		if !fi.IsDir() {
			dir = filepath.Dir(dir)
		}
		if err := s.OpenDirectory(dir); err != nil {
			return "Open directory failed: " + err.Error()
		}
		return "Opened directory."
	}
	return fmt.Sprintf("Unknown action: %s", action)
}
