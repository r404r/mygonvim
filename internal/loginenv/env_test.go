package loginenv

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDecide(t *testing.T) {
	for _, tc := range []struct {
		name, goos string
		parent     int
		tty, off   bool
		env        []string
		want       bool
	}{
		{"Finder", "darwin", 1, false, false, nil, true},
		{"terminal", "darwin", 100, true, false, nil, false},
		{"redirected terminal", "darwin", 100, false, false, nil, false},
		{"orphan tty", "darwin", 1, true, false, nil, false},
		{"background orphan", "darwin", 1, false, false, nil, true},
		{"Linux", "linux", 1, false, false, nil, false},
		{"Windows", "windows", 1, false, false, nil, false},
		{"forced Linux", "linux", 1, false, false, []string{"MYGONVIM_LOGIN_ENV=1"}, true},
		{"off", "darwin", 1, false, false, []string{"MYGONVIM_LOGIN_ENV=0"}, false},
		{"settings", "darwin", 1, false, true, nil, false},
		{"force overrides settings and tty", "darwin", 100, true, true, []string{"MYGONVIM_LOGIN_ENV=1"}, true},
		{"invalid switch uses heuristic", "darwin", 1, false, false, []string{"MYGONVIM_LOGIN_ENV=yes"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := Inputs{GOOS: tc.goos, Parent: tc.parent, StdoutTTY: tc.tty, SettingsOff: tc.off, Env: tc.env}
			got, _ := Decide(in)
			if got != tc.want {
				t.Fatalf("import=%v, want %v", got, tc.want)
			}
			for _, marker := range []string{"MYGONVIM_LOGIN_ENV_PROBE=1", "MYGONVIM_LOGIN_ENV_PROBE="} {
				in.Env = append(append([]string{}, tc.env...), marker)
				if should, _ := Decide(in); should {
					t.Fatal("inherited probe allowed recursion")
				}
			}
		})
	}
}

func TestParseSnapshot(t *testing.T) {
	got, err := Parse([]byte("banner\nMARKA=one=two\nthree\x00B=\xff\x00EMPTY=\x00MARKlogout"), "MARK")
	want := map[string]string{"A": "one=two\nthree", "B": "\xff", "EMPTY": ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parse=%v err=%v", got, err)
	}
	for _, data := range []string{"", "A=x\x00", "MARKA=x\x00", "MARKA=xMARK", "MARK=x\x00MARK", "MARKbad\x00MARK"} {
		if _, err := Parse([]byte(data), "MARK"); err == nil {
			t.Errorf("accepted incomplete or malformed snapshot %q", data)
		}
	}
	if got, err := Parse([]byte("MARKMARK"), "MARK"); err != nil || len(got) != 0 {
		t.Fatal("complete empty snapshot rejected")
	}
}

func TestMergePreservesProtectedAndUnsetVariables(t *testing.T) {
	base := []string{"PATH=/bin:/base:/bin", "TERM=original", "PWD=/work", "KEEP=old", "MYGONVIM_NVIM=/original", "MYGONVIM_LOGIN_ENV=1"}
	copyBase := append([]string{}, base...)
	shell := map[string]string{"PATH": "/tool:/bin:/tool", "KEEP": "new", "ADDED": "a=b\nc", "MYGONVIM_NVIM": "/other", "MYGONVIM_NEW": "bad", "TERM": "changed", "PWD": "/home", "OLDPWD": "x", "SHLVL": "99", "_": "x", "COLUMNS": "1", "LINES": "1", "BASH_FUNC_f%%": "() { true; }", "XDG_CONFIG_HOME": "/shell"}
	got := Merge(base, shell)
	for key, want := range map[string]string{"PATH": "/tool:/bin:/base", "TERM": "original", "PWD": "/work", "KEEP": "new", "ADDED": "a=b\nc", "MYGONVIM_NVIM": "/original", "MYGONVIM_LOGIN_ENV": "1", "XDG_CONFIG_HOME": "/shell"} {
		if Value(got, key) != want {
			t.Errorf("wrong %s", key)
		}
	}
	for _, key := range []string{"MYGONVIM_NEW", "OLDPWD", "SHLVL", "_", "COLUMNS", "LINES", "BASH_FUNC_f%%"} {
		if _, ok := lookup(got, key); ok {
			t.Errorf("imported %s", key)
		}
	}
	if !reflect.DeepEqual(base, copyBase) {
		t.Fatal("mutated inherited environment")
	}
	if Value(Merge([]string{"KEEP=old"}, nil), "KEEP") != "old" {
		t.Fatal("shell unset removed inherited variable")
	}
	if Value(Merge(nil, map[string]string{"PATH": ":/bin::/bin"}), "PATH") != ":/bin" {
		t.Fatal("empty PATH entries did not retain their first position")
	}
}

func TestValidateShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix absolute paths and executable modes")
	}
	for _, tc := range []struct {
		path string
		mode fs.FileMode
		want bool
	}{
		{"/bin/zsh", 0755, true}, {"/opt/homebrew/bin/bash", 0700, true},
		{"bash", 0755, false}, {"/bin/fish", 0755, false}, {"/bin/sh", 0755, false},
		{"/bin/bash", 0644, false}, {"/bin/bash", fs.ModeDir | 0755, false}, {"/bin/bash\x00", 0755, false},
	} {
		if got := ValidateShell(tc.path, tc.mode); got != tc.want {
			t.Errorf("%q valid=%v", tc.path, got)
		}
	}
}

func TestSettingsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if SettingsOff(path) {
		t.Fatal("missing settings disabled import")
	}
	for _, data := range []string{`{}`, `{"loginEnv":true}`, `{"loginEnv":null}`, `{"loginEnv":"false"}`, `broken`, `null`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if SettingsOff(path) {
			t.Fatalf("disabled for %s", data)
		}
	}
	if err := os.WriteFile(path, []byte(`{"fontSize":14,"loginEnv":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !SettingsOff(path) {
		t.Fatal("explicit false ignored")
	}
}
