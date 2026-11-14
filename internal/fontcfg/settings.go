package fontcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// File is the name of the settings file in the app's user data directory.
const File = "settings.json"

// Sizes the saved file may hold; anything else is dropped as unusable.
const (
	minSize = 4
	maxSize = 200
)

type fileFields struct {
	Family string  `json:"fontFamily,omitempty"`
	Size   float64 `json:"fontSize,omitempty"`
}

// Load reads the font chosen in the menu from the settings file at path. A
// file that does not exist gives the zero Choice and no error. A file that is
// not valid JSON gives the zero Choice and an error, for the caller to log and
// carry on with. Unusable values (a blank family, a size that is not a
// number between 4 and 200) are dropped one by one.
func Load(path string) (Choice, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Choice{}, nil
	}
	if err != nil {
		return Choice{}, err
	}
	var f fileFields
	if err := json.Unmarshal(data, &f); err != nil {
		return Choice{}, fmt.Errorf("%s: %w", path, err)
	}
	var c Choice
	if fam := strings.TrimSpace(f.Family); fam != "" && !strings.ContainsAny(fam, "\x00\n\r") {
		c.Family = fam
	}
	if f.Size >= minSize && f.Size <= maxSize && !math.IsNaN(f.Size) {
		c.Size = float32(f.Size)
	}
	return c, nil
}

// Save writes c to the settings file at path, creating its directory, and
// keeps any other keys the file already has. A file that cannot be read as a
// JSON object is replaced. The write goes through a temporary file and a
// rename, so a crash leaves the old file or the new one.
func Save(path string, c Choice) error {
	fields := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &fields) != nil || fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}
	set := func(key string, v any, keep bool) error {
		if !keep {
			delete(fields, key)
			return nil
		}
		b, err := json.Marshal(v)
		fields[key] = b
		return err
	}
	if err := set("fontFamily", c.Family, c.Family != ""); err != nil {
		return err
	}
	if err := set("fontSize", c.Size, c.Size != 0); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(fields); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, File+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
