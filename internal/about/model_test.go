package about

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPathsUseProcessValuesAndOnlyExistingInitFiles(t *testing.T) {
	dir := t.TempDir()
	p := Process{Settings: filepath.Join(dir, "settings.json"), Log: filepath.Join(dir, "actual-log"), Nvim: filepath.Join(dir, "actual-nvim")}
	info := Info{Paths: map[string]string{"config": dir, "data": "custom-data", "state": "custom-state", "cache": "custom-cache"}}
	for _, names := range [][]string{nil, {"init.lua"}, {"init.lua", "init.vim"}} {
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		got := Paths(p, info, os.Stat)
		want := []string{p.Settings, p.Log, p.Nvim, dir}
		for _, name := range names {
			want = append(want, filepath.Join(dir, name))
		}
		want = append(want, "custom-data", "custom-state", "custom-cache")
		var values []string
		for _, path := range got {
			values = append(values, path.Value)
		}
		if !reflect.DeepEqual(values, want) {
			t.Fatalf("paths=%q want=%q", values, want)
		}
	}
	// A directory named init.vim is not an init file.
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, "init.vim"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := Paths(Process{}, Info{Paths: map[string]string{"config": other}}, os.Stat); len(got) != 7 {
		t.Fatalf("invented init file: %+v", got)
	}
	got := Paths(p, Info{Errors: map[string]string{"config": "RPC failed"}}, os.Stat)
	if len(got) != 7 || got[3].Value != "" || got[3].Error != "RPC failed" {
		t.Fatalf("unavailable paths: %+v", got)
	}
}

func TestPathActionsCopyExactTextAndHandleMissingLocations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "空 白 'quoted' & [file].txt")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	var copied, revealed, opened string
	s := Services{Stat: os.Stat, Copy: func(p string) error { copied = p; return nil }, Reveal: func(p string) { revealed = p }, OpenDirectory: func(p string) error { opened = p; return nil }}
	p := Path{Value: path}
	if got := s.Act("copy", p); got != "Copied path." || copied != path {
		t.Fatalf("copy=%q text=%q", got, copied)
	}
	if got := s.Act("reveal", p); !strings.Contains(got, "Requested") || revealed != path {
		t.Fatalf("reveal=%q path=%q", got, revealed)
	}
	if got := s.Act("open", p); got != "Opened directory." || opened != dir {
		t.Fatalf("open=%q path=%q", got, opened)
	}
	opened = ""
	if got := s.Act("open", Path{Value: dir}); got != "Opened directory." || opened != dir {
		t.Fatalf("open directory=%q path=%q", got, opened)
	}
	missing := Path{Value: filepath.Join(dir, "missing")}
	if got := s.Act("copy", missing); !strings.Contains(got, "Copied path; location unavailable") || copied != missing.Value {
		t.Fatalf("missing copy=%q", got)
	}
	revealed = ""
	opened = ""
	for _, action := range []string{"reveal", "open"} {
		if got := s.Act(action, missing); !strings.Contains(got, "Location unavailable") {
			t.Fatalf("missing %s=%q", action, got)
		}
	}
	if revealed != "" || opened != "" {
		t.Fatal("attempted shell action on missing location")
	}
	for _, action := range []string{"copy", "reveal", "open"} {
		if got := s.Act(action, Path{}); !strings.Contains(got, "Unavailable") {
			t.Fatal(got)
		}
	}
	s.Copy = func(string) error { return errors.New("denied") }
	if got := s.Act("copy", p); !strings.Contains(got, "Copy failed: denied") {
		t.Fatal(got)
	}
	s.OpenDirectory = func(string) error { return errors.New("denied") }
	if got := s.Act("open", p); !strings.Contains(got, "Open directory failed: denied") {
		t.Fatal(got)
	}
}
