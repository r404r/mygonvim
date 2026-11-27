// Package loginenv imports a bounded snapshot from a login shell without
// changing the environment of the GUI process.
package loginenv

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type Inputs struct {
	GOOS        string
	Parent      int
	StdoutTTY   bool
	Env         []string
	SettingsOff bool
}

// Decide applies the explicit switches before the desktop-launch heuristic.
func Decide(in Inputs) (bool, string) {
	if _, ok := lookup(in.Env, "MYGONVIM_LOGIN_ENV_PROBE"); ok {
		return false, "nested probe"
	}
	switch Value(in.Env, "MYGONVIM_LOGIN_ENV") {
	case "0":
		return false, "disabled by environment"
	case "1":
		return true, "forced by environment"
	}
	if in.SettingsOff {
		return false, "disabled in settings"
	}
	if in.GOOS != "darwin" {
		return false, "not macOS"
	}
	if in.Parent == 1 && !in.StdoutTTY {
		return true, "desktop launch"
	}
	return false, "terminal launch"
}

func lookup(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		k, v, ok := strings.Cut(env[i], "=")
		if ok && k == key {
			return v, true
		}
	}
	return "", false
}

func Value(env []string, key string) string { v, _ := lookup(env, key); return v }

// ValidateShell accepts executable regular zsh/bash files, including symlinks
// whose target was inspected by the caller. The path is passed directly to
// exec, never interpolated into the shell command.
func ValidateShell(path string, mode fs.FileMode) bool {
	name := filepath.Base(path)
	return filepath.IsAbs(path) && !strings.ContainsRune(path, 0) &&
		(name == "zsh" || name == "bash") && mode.IsRegular() && mode.Perm()&0o111 != 0
}

// Parse accepts only a complete pair of markers and NUL-terminated records.
// Shell banners, stderr, and logout output are never returned in an error.
func Parse(out []byte, marker string) (map[string]string, error) {
	failure := errors.New("invalid environment snapshot")
	if marker == "" {
		return nil, failure
	}
	_, rest, ok := bytes.Cut(out, []byte(marker))
	if !ok {
		return nil, failure
	}
	data, _, ok := bytes.Cut(rest, []byte(marker))
	if !ok || (len(data) > 0 && data[len(data)-1] != 0) {
		return nil, failure
	}
	vars := map[string]string{}
	for len(data) > 0 {
		record, tail, _ := bytes.Cut(data, []byte{0})
		data = tail
		key, value, ok := strings.Cut(string(record), "=")
		if !ok || key == "" {
			return nil, failure
		}
		vars[key] = value
	}
	return vars, nil
}

func denied(key string) bool {
	switch key {
	case "PWD", "OLDPWD", "SHLVL", "_", "COLUMNS", "LINES", "TERM":
		return true
	}
	return strings.HasPrefix(key, "MYGONVIM_") ||
		(strings.HasPrefix(key, "BASH_FUNC_") && strings.HasSuffix(key, "%%"))
}

// Merge preserves inherited variables absent from the snapshot. Shell PATH
// entries precede process-only entries; duplicates (including empty ones)
// occur at most once. Portable overrides belong after this operation.
func Merge(base []string, vars map[string]string) []string {
	values := map[string]string{}
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range vars {
		if denied(key) {
			continue
		}
		if key == "PATH" {
			paths := strings.Split(value, ":")
			if inherited, ok := values[key]; ok {
				paths = append(paths, strings.Split(inherited, ":")...)
			}
			seen := map[string]bool{}
			unique := []string{}
			for _, p := range paths {
				if !seen[p] {
					unique = append(unique, p)
					seen[p] = true
				}
			}
			value = strings.Join(unique, ":")
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
