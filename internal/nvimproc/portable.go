package nvimproc

import (
	"os"
	"path/filepath"
	"strings"
)

// PortableDir returns exeDir/data when that directory exists: the program is
// then run from a portable folder, and Neovim's config, data, state and cache
// live there instead of in the user's profile.
func PortableDir(exeDir string) (string, bool) {
	if exeDir == "" {
		return "", false
	}
	d := filepath.Join(exeDir, "data")
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d, true
	}
	return "", false
}

// PortableEnv returns env with the XDG_* variables Neovim reads for stdpath()
// pointed into dataDir (config/, share/, state/, cache/), replacing any the
// user's environment sets. env is not modified.
func PortableEnv(env []string, dataDir string) []string {
	set := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(dataDir, "config"),
		"XDG_DATA_HOME":   filepath.Join(dataDir, "share"),
		"XDG_STATE_HOME":  filepath.Join(dataDir, "state"),
		"XDG_CACHE_HOME":  filepath.Join(dataDir, "cache"),
	}
	out := make([]string, 0, len(env)+len(set))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if _, ours := set[strings.ToUpper(k)]; ours {
			continue
		}
		out = append(out, kv)
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		out = append(out, k+"="+set[k])
	}
	return out
}

// EnsurePortableDirs creates the four directories PortableEnv points at.
func EnsurePortableDirs(dataDir string) error {
	for _, d := range []string{"config", "share", "state", "cache"} {
		if err := os.MkdirAll(filepath.Join(dataDir, d), 0o755); err != nil {
			return err
		}
	}
	return nil
}
