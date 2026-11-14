package fontcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsEmptyNotAnError(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope", File))
	if err != nil || c != (Choice{}) {
		t.Errorf("Load = %+v, %v; want the zero Choice and no error", c, err)
	}
}

func TestSaveThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "mygonvim", File) // directories do not exist yet
	want := Choice{Family: "JetBrainsMono Nerd Font", Size: 15.5}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v; want %+v", got, err, want)
	}
	// The file is JSON with readable keys.
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("the saved file is not JSON: %v\n%s", err, data)
	}
	if m["fontFamily"] != "JetBrainsMono Nerd Font" || m["fontSize"] != 15.5 {
		t.Errorf("saved file = %s", data)
	}
	// Saving again replaces the choice and leaves no temporary files behind.
	if err := Save(path, Choice{Family: "Menlo", Size: 12}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got != (Choice{Family: "Menlo", Size: 12}) {
		t.Errorf("after a second save Load = %+v", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want only %s", len(entries), File)
	}
}

func TestSaveKeepsOtherKeysAndDropsEmptyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	os.WriteFile(path, []byte(`{"theme": {"name": "dark"}, "fontSize": 20, "fontFamily": "Old"}`), 0o644)
	if err := Save(path, Choice{Family: "New"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["theme"]) == "" || !strings.Contains(string(m["theme"]), "dark") {
		t.Errorf("another key was lost: %s", data)
	}
	if _, ok := m["fontSize"]; ok {
		t.Errorf("an unset size stayed in the file: %s", data)
	}
	if got, _ := Load(path); got != (Choice{Family: "New"}) {
		t.Errorf("Load = %+v", got)
	}
}

func TestSaveReplacesAFileThatIsNotAnObject(t *testing.T) {
	for _, content := range []string{"", "not json", "[1,2]", "null", `"text"`} {
		path := filepath.Join(t.TempDir(), File)
		os.WriteFile(path, []byte(content), 0o644)
		if err := Save(path, Choice{Family: "Menlo", Size: 12}); err != nil {
			t.Errorf("Save over %q: %v", content, err)
			continue
		}
		if got, err := Load(path); err != nil || got != (Choice{Family: "Menlo", Size: 12}) {
			t.Errorf("after saving over %q Load = %+v, %v", content, got, err)
		}
	}
}

func TestLoadCorruptFileGivesAnErrorAndNoChoice(t *testing.T) {
	for _, content := range []string{"", "not json", "{", `{"fontSize": "big"}`, "[1,2]", "\x00\x01\x02"} {
		path := filepath.Join(t.TempDir(), File)
		os.WriteFile(path, []byte(content), 0o644)
		c, err := Load(path)
		if err == nil {
			t.Errorf("Load of %q: no error, got %+v", content, c)
		}
		if c != (Choice{}) {
			t.Errorf("Load of %q gave %+v, want nothing", content, c)
		}
	}
}

func TestLoadDropsUnusableValues(t *testing.T) {
	cases := []struct {
		json string
		want Choice
	}{
		{`{}`, Choice{}},
		{`{"fontFamily": "  Menlo  ", "fontSize": 14}`, Choice{Family: "Menlo", Size: 14}},
		{`{"fontFamily": "   "}`, Choice{}},
		{`{"fontFamily": "A\nB", "fontSize": 12}`, Choice{Size: 12}},
		{`{"fontSize": 0}`, Choice{}},
		{`{"fontSize": -5}`, Choice{}},
		{`{"fontSize": 3}`, Choice{}},
		{`{"fontSize": 1000}`, Choice{}},
		{`{"fontFamily": "Menlo", "fontSize": 1e9}`, Choice{Family: "Menlo"}},
		{`{"fontFamily": 7, "fontSize": 14}`, Choice{}}, // wrong type: not usable at all
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), File)
		os.WriteFile(path, []byte(c.json), 0o644)
		got, err := Load(path)
		if c.json == `{"fontFamily": 7, "fontSize": 14}` {
			if err == nil {
				t.Errorf("Load(%s): no error for a wrong type", c.json)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Load(%s) = %+v, %v; want %+v", c.json, got, err, c.want)
		}
	}
}

func TestLoadUnreadablePathIsAnError(t *testing.T) {
	dir := t.TempDir() // a directory where the file should be
	if _, err := Load(dir); err == nil {
		t.Error("Load of a directory: no error")
	}
}
