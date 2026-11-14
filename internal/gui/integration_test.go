package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

// The whole path against a real nvim --embed, with a headless MyGo window (the
// ui.Tester, software rendering): keys in, grid out, a file edited and saved.
// Skipped without nvim; set NVIM_BIN, or put nvim on PATH. Set
// MYGONVIM_SCREENSHOT=/path/x.png to keep a picture of the window.
func TestRealNvimEditAndSave(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	dir := t.TempDir()
	sample := filepath.Join(dir, "sample.go")
	os.WriteFile(sample, []byte("package main\n\nimport \"fmt\"\n\n// main says hello.\nfunc main() {\n\tfmt.Println(\"hello, 世界\")\n}\n"), 0o644)
	notes := filepath.Join(dir, "notes.txt")

	var frames atomic.Int64
	var title atomic.Value
	var ed *Editor
	ed = NewEditor(nil, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{
		Invalidate: func() { frames.Add(1) },
		SetTitle:   func(s string) { title.Store(s) },
	})
	sess, err := nvimproc.Start(nvimproc.Options{
		Bin: bin, Cols: 80, Rows: 24,
		Args:     []string{"--clean", "-c", "set termguicolors | colorscheme habamax | set number | syntax on", sample},
		Env:      nvimproc.PortableEnv(os.Environ(), filepath.Join(dir, "data")),
		OnRedraw: func(u ...[]any) { ed.OnRedraw(u...) },
		Logf:     t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	ed.be = sess
	defer sess.Close()

	tt := ui.NewTester(ed.View, 800, 480)
	tt.SetScale(1)
	tt.SetFocused(true)
	tt.Frame() // the first frame asks nvim for the window's grid

	read := func(f func(s *grid.State)) { ed.Model.Read(f) }
	rowText := func(r int) (s string) { read(func(st *grid.State) { s = st.RowText(r) }); return }
	cols := func() (c, r int) { c, r = ed.Model.Size(); return }
	wait := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		var dump []string
		read(func(st *grid.State) {
			for r := 0; r < st.Rows; r++ {
				dump = append(dump, st.RowText(r))
			}
		})
		t.Fatalf("timed out waiting for %s; screen:\n%s", what, strings.Join(dump, "\n"))
	}

	// 1. The window's size reached nvim, which answered with a grid of that size.
	wantCols, wantRows := int(800/ed.cw), int(480/ed.ch)
	wait("the grid to take the window's size", func() bool { c, r := cols(); return c == wantCols && r == wantRows })

	// 2. The file is shown, with line numbers, and the title is set.
	wait("sample.go on screen", func() bool { return strings.Contains(rowText(0), "package main") })
	wait("a window title", func() bool { v, _ := title.Load().(string); return v != "" })
	t.Logf("title: %q", title.Load())
	if !strings.Contains(rowText(6), "世界") {
		t.Errorf("line 7 is %q, want the CJK text", rowText(6))
	}

	// 3. Typing: edit another file in insert mode, with CJK through the input
	// method path (a composition, then its commit) and an emoji.
	// Use the command line the way a user would.
	tt.Key(0, ui.KeyEscape)
	for _, ch := range ":e " + notes {
		typeKey(tt, ch)
	}
	tt.Key(0, ui.KeyEnter)
	wait("notes.txt to open", func() bool { v, _ := title.Load().(string); return strings.Contains(v, "notes.txt") })
	typeKey(tt, 'i')
	wait("insert mode", func() bool { var m string; read(func(s *grid.State) { m = s.Mode }); return m == "insert" })
	tt.Frame()
	if _, noIME := tt.TextInputActive(); noIME {
		t.Error("in insert mode the input method should be on")
	}
	for _, ch := range "hello " {
		typeKey(tt, ch)
	}
	tt.Compose("shijie", 6)
	tt.Frame()
	tt.Type("世界") // the method commits
	tt.Type(" 😀")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	for _, ch := range "a < b" { // "<" must not start a key name
		typeKey(tt, ch)
	}
	tt.Key(0, ui.KeyEscape)
	wait("the typed text on screen", func() bool {
		return strings.Contains(rowText(0), " hello 世界 😀") && strings.Contains(rowText(1), " a < b")
	})
	wait("normal mode", func() bool { var m string; read(func(s *grid.State) { m = s.Mode }); return m == "normal" })
	tt.Frame()
	if _, noIME := tt.TextInputActive(); !noIME {
		t.Error("back in normal mode the input method should be off")
	}

	// 4. Save with :w and read the file back.
	for _, ch := range ":w" {
		typeKey(tt, ch)
	}
	tt.Key(0, ui.KeyEnter)
	wait("the file to be written", func() bool {
		b, err := os.ReadFile(notes)
		return err == nil && string(b) == "hello 世界 😀\na < b\n"
	})

	// 5. Open sample.go again for the picture: syntax colors, number column, cursor.
	for _, ch := range ":e " + sample {
		typeKey(tt, ch)
	}
	tt.Key(0, ui.KeyEnter)
	wait("sample.go again", func() bool { return strings.Contains(rowText(0), "package main") })
	typeKey(tt, 'j')
	typeKey(tt, 'j')
	typeKey(tt, 'V')
	typeKey(tt, 'j')
	typeKey(tt, 'j')
	time.Sleep(150 * time.Millisecond)
	tt.Frame()
	if path := os.Getenv("MYGONVIM_SCREENSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		t.Logf("screenshot: %s", path)
	}
	if frames.Load() == 0 {
		t.Error("no frame was ever requested by a flush")
	}
}

// typeKey presses the key that types ch on a US keyboard: down, the text, up.
func typeKey(tt *ui.Tester, ch rune) {
	mods := ui.Modifiers(0)
	var key ui.Key
	switch {
	case ch >= 'a' && ch <= 'z':
		key = ui.KeyA + ui.Key(ch-'a')
	case ch >= 'A' && ch <= 'Z':
		key, mods = ui.KeyA+ui.Key(ch-'A'), ui.Shift
	case ch >= '0' && ch <= '9':
		key = ui.Key0 + ui.Key(ch-'0')
	default:
		switch ch {
		case ' ':
			key = ui.KeySpace
		case ':':
			key, mods = ui.KeySemicolon, ui.Shift
		case '<':
			key, mods = ui.KeyComma, ui.Shift
		case '/':
			key = ui.KeySlash
		case '.':
			key = ui.KeyPeriod
		case '-':
			key = ui.KeyMinus
		case '_':
			key, mods = ui.KeyMinus, ui.Shift
		default:
			panic("typeKey: no key for " + string(ch))
		}
	}
	tt.TypeKey(mods, key, string(ch))
}
