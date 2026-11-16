package gui

import (
	"image/color"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

// fake is a Backend that records what the editor sends.
type fake struct {
	mu      sync.Mutex
	keys    []string
	resizes [][2]int
	cmds    []string
	mice    []mouseCall
}

type mouseCall struct {
	button, action, modifier string
	row, col                 int
}

func (f *fake) Input(k string) { f.mu.Lock(); f.keys = append(f.keys, k); f.mu.Unlock() }
func (f *fake) Resize(c, r int) {
	f.mu.Lock()
	f.resizes = append(f.resizes, [2]int{c, r})
	f.mu.Unlock()
}
func (f *fake) Mouse(button, action, modifier string, row, col int) {
	f.mu.Lock()
	f.mice = append(f.mice, mouseCall{button, action, modifier, row, col})
	f.mu.Unlock()
}
func (f *fake) Command(c string) { f.mu.Lock(); f.cmds = append(f.cmds, c); f.mu.Unlock() }
func (f *fake) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...)
}
func (f *fake) lastResize() [2]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.resizes) == 0 {
		return [2]int{}
	}
	return f.resizes[len(f.resizes)-1]
}

type env struct {
	ed  *Editor
	be  *fake
	tt  *ui.Tester
	inv int
	ttl string
}

func newEnv(t *testing.T, w, h int) *env {
	t.Helper()
	e := &env{be: &fake{}}
	e.ed = NewEditor(e.be, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{
		Invalidate: func() { e.inv++ },
		SetTitle:   func(s string) { e.ttl = s },
	})
	e.tt = ui.NewTester(e.ed.View, w, h)
	e.tt.SetScale(1)
	e.tt.Frame()
	return e
}

func ev(name string, args ...[]any) []any {
	u := []any{name}
	for _, a := range args {
		u = append(u, a)
	}
	return u
}

func i64(n ...int) []any {
	out := make([]any, len(n))
	for i, v := range n {
		out[i] = int64(v)
	}
	return out
}

func px(e *env, x, y int) color.RGBA {
	return e.tt.Image().RGBAAt(x, y)
}

func cellCenter(e *env, col, row int) (int, int) {
	return int((float32(col) + 0.5) * e.ed.cw), int((float32(row) + 0.5) * e.ed.ch)
}

func TestCellMetricsAreWholePixels(t *testing.T) {
	e := newEnv(t, 640, 400)
	if e.ed.cw < 4 || e.ed.ch < 8 {
		t.Fatalf("cell %.2fx%.2f is implausibly small", e.ed.cw, e.ed.ch)
	}
	if e.ed.cw != float32(int(e.ed.cw)) || e.ed.ch != float32(int(e.ed.ch)) {
		t.Errorf("cell %.3fx%.3f at scale 1 is not whole pixels", e.ed.cw, e.ed.ch)
	}
	e.tt.SetScale(2)
	e.tt.Frame()
	if e.ed.cw*2 != float32(int(e.ed.cw*2)) || e.ed.ch*2 != float32(int(e.ed.ch*2)) {
		t.Errorf("cell %.3fx%.3f at scale 2 is not whole device pixels", e.ed.cw, e.ed.ch)
	}
}

func TestResizeFollowsTheWindow(t *testing.T) {
	e := newEnv(t, 640, 400)
	want := [2]int{int(640 / e.ed.cw), int(400 / e.ed.ch)}
	if got := e.be.lastResize(); got != want {
		t.Errorf("first frame asked for %v, want %v", got, want)
	}
	e.tt.SetSize(800, 500)
	e.tt.Frame()
	want = [2]int{int(800 / e.ed.cw), int(500 / e.ed.ch)}
	if got := e.be.lastResize(); got != want {
		t.Errorf("after resizing the window asked for %v, want %v", got, want)
	}
	n := len(e.be.resizes)
	e.tt.Frame()
	e.tt.Frame()
	if len(e.be.resizes) != n {
		t.Errorf("an unchanged size was asked again (%d requests)", len(e.be.resizes)-n)
	}
}

func TestDrawsBackgroundsAndText(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.ed.OnRedraw(
		ev("default_colors_set", i64(0xeeeeee, 0x102030, 0xff0000)),
		ev("hl_attr_define", []any{int64(1), map[string]any{"background": int64(0x00ff00), "foreground": int64(0x000000)}, map[string]any{}, []any{}}),
		ev("grid_clear", i64(1)),
		ev("grid_line", []any{int64(1), int64(2), int64(3), []any{[]any{"M", int64(1), int64(2)}, []any{"x", int64(0)}}, false}),
		ev("flush", []any{}),
	)
	if e.inv != 1 {
		t.Errorf("flush asked for %d frames", e.inv)
	}
	e.tt.Frame()
	if got := px(e, 600, 380); got != (color.RGBA{0x10, 0x20, 0x30, 255}) {
		t.Errorf("default background pixel %v", got)
	}
	// The highlighted cells (2 columns, row 2) are green where no glyph is.
	x0, y0 := int(3*e.ed.cw)+1, int(2*e.ed.ch)+1
	if got := px(e, x0, y0); got != (color.RGBA{0, 0xff, 0, 255}) {
		t.Errorf("highlighted cell corner %v", got)
	}
	x1 := int(4*e.ed.cw) + 1
	if got := px(e, x1, y0); got != (color.RGBA{0, 0xff, 0, 255}) {
		t.Errorf("second highlighted cell corner %v", got)
	}
	if got := px(e, int(5*e.ed.cw)+1, y0); got != (color.RGBA{0x10, 0x20, 0x30, 255}) {
		t.Errorf("cell after the run %v", got)
	}
	// Glyph ink: some pixel of the "M" cell is neither the background nor the cursor.
	if !hasInk(e, 3, 2, color.RGBA{0, 0xff, 0, 255}) {
		t.Error("no glyph ink in the M cell")
	}
	if !hasInk(e, 5, 2, color.RGBA{0x10, 0x20, 0x30, 255}) {
		t.Error("no glyph ink in the x cell")
	}
}

// hasInk reports whether any pixel of the cell differs from bg.
func hasInk(e *env, col, row int, bg color.RGBA) bool {
	x0, y0 := int(float32(col)*e.ed.cw), int(float32(row)*e.ed.ch)
	for y := y0; y < y0+int(e.ed.ch); y++ {
		for x := x0; x < x0+int(e.ed.cw); x++ {
			if px(e, x, y) != bg {
				return true
			}
		}
	}
	return false
}

func TestWideCharactersKeepTheirCells(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.ed.OnRedraw(
		ev("default_colors_set", i64(0xffffff, 0x000000, 0xff0000)),
		ev("grid_line", []any{int64(1), int64(0), int64(0), []any{[]any{"中", int64(0)}, []any{""}, []any{"a", int64(0)}}, false}),
		ev("flush", []any{}),
	)
	e.tt.Frame()
	// The a is in column 2 whatever the font's advance of 中 was.
	if !hasInk(e, 2, 0, color.RGBA{0, 0, 0, 255}) {
		t.Error("no ink in column 2")
	}
	if !hasInk(e, 0, 0, color.RGBA{0, 0, 0, 255}) && !hasInk(e, 1, 0, color.RGBA{0, 0, 0, 255}) {
		t.Log("no ink for the wide character: this machine has no CJK font (not an error)")
	}
	// Compare with the same glyph alone in column 2. Some system fonts have
	// ink beyond the advance; that overhang must be identical, not displaced
	// by the wide character or its fallback font.
	ref := newEnv(t, 640, 400)
	ref.ed.OnRedraw(
		ev("default_colors_set", i64(0xffffff, 0x000000, 0xff0000)),
		ev("grid_line", []any{int64(1), int64(0), int64(2), []any{[]any{"a", int64(0)}}, false}),
		ev("flush", []any{}),
	)
	ref.tt.Frame()
	for y := 0; y < int(e.ed.ch); y++ {
		for x := int(2 * e.ed.cw); x < int(5*e.ed.cw); x++ {
			if px(e, x, y) != px(ref, x, y) {
				t.Fatalf("glyph after wide character differs from column 2 reference at (%d, %d)", x, y)
			}
		}
	}
}

func TestBlockCursorInvertsTheCell(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	e.ed.OnRedraw(
		ev("default_colors_set", i64(0xffffff, 0x000000, 0xff0000)),
		ev("mode_info_set", []any{true, []any{
			map[string]any{"name": "normal", "cursor_shape": "block", "cell_percentage": int64(100)},
			map[string]any{"name": "insert", "cursor_shape": "vertical", "cell_percentage": int64(25)},
		}}),
		ev("mode_change", []any{"normal", int64(0)}),
		ev("grid_cursor_goto", i64(1, 4, 6)),
		ev("flush", []any{}),
	)
	e.tt.Frame()
	x, y := int(6*e.ed.cw)+1, int(4*e.ed.ch)+1
	if got := px(e, x, y); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("block cursor corner %v, want the default foreground", got)
	}
	// Insert mode: a bar a quarter of the cell wide.
	e.ed.OnRedraw(ev("mode_change", []any{"insert", int64(1)}), ev("flush", []any{}))
	e.tt.Frame()
	bar := int(e.ed.cw * 0.25)
	if bar < 1 {
		bar = 1
	}
	if got := px(e, x, y); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("bar cursor left pixel %v", got)
	}
	if got := px(e, int(6*e.ed.cw)+bar+1, y); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("past the bar the cell is background, got %v", got)
	}
}

func TestKeysGoToNvimInNotation(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	e.tt.TypeKey(0, ui.KeyI, "i")
	e.tt.Key(0, ui.KeyEscape)
	e.tt.Key(ui.Ctrl, ui.KeyW)
	e.tt.TypeKey(ui.Shift, ui.KeyComma, "<")
	e.tt.Key(ui.Shift, ui.KeyLeft)
	e.tt.Key(0, ui.KeyEnter)
	got := e.be.sent()
	want := []string{"i", "<Esc>", "<C-w>", "<lt>", "<S-Left>", "<CR>"}
	if len(got) != len(want) {
		t.Fatalf("sent %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("key %d: sent %q, want %q", i, got[i], want[i])
		}
	}
}

func TestInputMethodFollowsTheMode(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	active, noIME := e.tt.TextInputActive()
	if !active || !noIME {
		t.Fatalf("normal mode: active=%v noInputMethod=%v, want a text element with input methods off", active, noIME)
	}
	e.ed.OnRedraw(ev("mode_change", []any{"insert", int64(1)}), ev("grid_cursor_goto", i64(1, 3, 5)), ev("flush", []any{}))
	e.tt.Frame()
	active, noIME = e.tt.TextInputActive()
	if !active || noIME {
		t.Errorf("insert mode: active=%v noInputMethod=%v, want input methods on", active, noIME)
	}
	r, on := e.tt.TextCaret()
	wantX, wantY := float32(5)*e.ed.cw, float32(3)*e.ed.ch
	if !on || r.X != wantX || r.Y != wantY || r.H != e.ed.ch {
		t.Errorf("caret %+v (on=%v), want the cursor cell at %.0f,%.0f", r, on, wantX, wantY)
	}
	e.ed.OnRedraw(ev("mode_change", []any{"normal", int64(0)}), ev("flush", []any{}))
	e.tt.Frame()
	if _, noIME = e.tt.TextInputActive(); !noIME {
		t.Error("back in normal mode the input method should be off again")
	}
}

func TestCompositionIsDrawnAndCommitted(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	e.ed.OnRedraw(
		ev("default_colors_set", i64(0xffffff, 0x000000, 0xff0000)),
		ev("mode_change", []any{"insert", int64(1)}),
		ev("mode_info_set", []any{true, []any{map[string]any{"name": "normal"}, map[string]any{"name": "insert", "cursor_shape": "vertical", "cell_percentage": int64(10)}}}),
		ev("grid_cursor_goto", i64(1, 1, 2)),
		ev("flush", []any{}),
	)
	e.tt.Frame()
	if hasInk(e, 5, 1, color.RGBA{0, 0, 0, 255}) {
		t.Fatal("ink in column 5 before composing")
	}
	e.tt.Compose("你好ni", 3)
	e.tt.Frame()
	// 你好 take columns 2-5, n and i columns 6-7: all of them have ink.
	for _, col := range []int{2, 4, 6, 7} {
		if !hasInk(e, col, 1, color.RGBA{0, 0, 0, 255}) {
			t.Errorf("no preedit ink in column %d", col)
		}
	}
	if len(e.be.sent()) != 0 {
		t.Errorf("a composition went to nvim: %q", e.be.sent())
	}
	// The caret (after 你好n: 5 cells) moves the candidate window's anchor.
	r, _ := e.tt.TextCaret()
	if want := float32(2+5) * e.ed.cw; r.X != want {
		t.Errorf("caret x %.0f, want %.0f", r.X, want)
	}
	e.tt.Type("你好")
	if got := e.be.sent(); len(got) != 1 || got[0] != "你好" {
		t.Errorf("commit sent %q", got)
	}
	e.tt.Frame()
	if hasInk(e, 6, 1, color.RGBA{0, 0, 0, 255}) {
		t.Error("the preedit is still drawn after the commit")
	}
}

func TestTitleAndBellHooks(t *testing.T) {
	e := newEnv(t, 320, 200)
	e.ed.OnRedraw(ev("set_title", []any{"main.go (~/src) - NVIM"}), ev("flush", []any{}))
	if e.ttl != "main.go (~/src) - NVIM" {
		t.Errorf("title %q", e.ttl)
	}
	before := e.inv
	e.ed.OnRedraw(ev("grid_cursor_goto", i64(1, 0, 1))) // no flush yet: no frame
	if e.inv != before {
		t.Error("a frame was requested before flush")
	}
}

func TestUnfocusedCursorIsAnOutline(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.ed.OnRedraw(
		ev("default_colors_set", i64(0xffffff, 0x000000, 0xff0000)),
		ev("grid_cursor_goto", i64(1, 2, 2)),
		ev("flush", []any{}),
	)
	e.tt.SetFocused(false)
	e.tt.Frame()
	x0, y0 := int(2*e.ed.cw), int(2*e.ed.ch)
	if got := px(e, x0, y0); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("outline corner %v", got)
	}
	if got := px(e, x0+int(e.ed.cw/2), y0+int(e.ed.ch/2)); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("outline interior %v, want background", got)
	}
}
