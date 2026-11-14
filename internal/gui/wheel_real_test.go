package gui

import (
	"fmt"
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

// realWheel is a headless window on a real nvim --embed editing file, with the
// wheel set to read like macOS's. Skipped without nvim (see NVIM_BIN).
type realWheel struct {
	t    *testing.T
	ed   *Editor
	tt   *ui.Tester
	sess *nvimproc.Session
}

func startRealWheel(t *testing.T, file string, nvimArgs ...string) *realWheel {
	t.Helper()
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	var frames atomic.Int64
	ed := NewEditor(nil, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{
		Invalidate: func() { frames.Add(1) },
	})
	ed.wheel.GOOS = "darwin"
	sess, err := nvimproc.Start(nvimproc.Options{
		Bin: bin, Cols: 80, Rows: 24,
		Args:     append(append([]string{"--clean"}, nvimArgs...), file),
		Env:      nvimproc.PortableEnv(os.Environ(), filepath.Join(t.TempDir(), "data")),
		OnRedraw: func(u ...[]any) { ed.OnRedraw(u...) },
		Logf:     t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	ed.be = sess
	t.Cleanup(sess.Close)
	tt := ui.NewTester(ed.View, 800, 480)
	tt.SetScale(1)
	tt.SetFocused(true)
	tt.Frame()
	r := &realWheel{t: t, ed: ed, tt: tt, sess: sess}
	wantCols, wantRows := int(800/ed.cw), int(480/ed.ch)
	r.wait("the grid to take the window's size", func() bool {
		c, rows := ed.Model.Size()
		return c == wantCols && rows == wantRows
	})
	return r
}

func (r *realWheel) wait(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var dump []string
	r.ed.Model.Read(func(st *grid.State) {
		for i := 0; i < st.Rows; i++ {
			dump = append(dump, st.RowText(i))
		}
	})
	r.t.Fatalf("timed out waiting for %s; screen:\n%s", what, strings.Join(dump, "\n"))
}

func (r *realWheel) row(i int) (s string) {
	r.ed.Model.Read(func(st *grid.State) { s = strings.TrimRight(st.RowText(i), " ") })
	return
}

// eval asks nvim for an integer.
func (r *realWheel) eval(expr string) int {
	r.t.Helper()
	var n int
	if err := r.sess.Nvim().Eval(expr, &n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

// at is the middle of a cell, in DIPs.
func (r *realWheel) at(col, row int) (float32, float32) {
	return (float32(col) + 0.5) * r.ed.cw, (float32(row) + 0.5) * r.ed.ch
}

func writeLong(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	p := filepath.Join(t.TempDir(), "long.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRealNvimWheelScrollsALongBuffer(t *testing.T) {
	r := startRealWheel(t, writeLong(t), "--cmd", "set nonumber")
	r.wait("the file", func() bool { return r.row(0) == "line 1" })
	x, y := r.at(10, 8)

	// A mouse wheel: each notch is one step of three lines.
	r.tt.Wheel(x, y, 0, 40)
	r.wait("the first notch", func() bool { return r.row(0) == "line 4" })
	r.tt.Wheel(x, y, 0, 40)
	r.tt.Wheel(x, y, 0, 40)
	r.wait("three notches", func() bool { return r.row(0) == "line 10" })
	r.tt.Wheel(x, y, 0, -40)
	r.wait("a notch back", func() bool { return r.row(0) == "line 7" })

	// A touchpad: small moves add up to the same steps. Two whole steps in
	// twelve pieces move six lines.
	piece := 2 * 3 * r.ed.ch / 12
	for i := 0; i < 12; i++ {
		r.tt.Scroll(x, y, 0, piece+0.01)
	}
	r.wait("two touchpad steps", func() bool { return r.row(0) == "line 13" })
	// And back, in one big move.
	r.tt.Scroll(x, y, 0, -2*3*r.ed.ch-0.1)
	r.wait("two steps back", func() bool { return r.row(0) == "line 7" })
}

func TestRealNvimWheelStopsAtTheEndsOfTheBuffer(t *testing.T) {
	r := startRealWheel(t, writeLong(t), "--cmd", "set nonumber")
	r.wait("the file", func() bool { return r.row(0) == "line 1" })
	x, y := r.at(10, 8)
	r.tt.Wheel(x, y, 0, -40)
	r.tt.Wheel(x, y, 0, -40)
	time.Sleep(200 * time.Millisecond)
	if got := r.row(0); got != "line 1" {
		t.Errorf("scrolling up at the top moved the view to %q", got)
	}
	// A fast fling: one event is capped at 50 steps (150 lines), so it takes
	// a few to reach the end of 300 lines.
	for i := 0; i < 3; i++ {
		r.tt.Wheel(x, y, 0, 40*200)
	}
	// Neovim lets the last line scroll up to the top, and no further.
	r.wait("the end", func() bool { return r.row(0) == "line 300" })
	r.tt.Wheel(x, y, 0, 40)
	time.Sleep(200 * time.Millisecond)
	if got := r.row(0); got != "line 300" {
		t.Errorf("scrolling down at the end moved the view to %q", got)
	}
}

func TestRealNvimWheelScrollsTheWindowUnderThePointer(t *testing.T) {
	r := startRealWheel(t, writeLong(t), "--cmd", "set nonumber", "-c", "vsplit")
	r.wait("two windows", func() bool { return r.eval(`winnr('$')`) == 2 })
	cols, _ := r.ed.Model.Size()
	// Nvim put the new window on the left and made it current.
	left := r.eval(`win_getid(1)`)
	right := r.eval(`win_getid(2)`)
	top := func(id int) int { return r.eval(fmt.Sprintf(`getwininfo(%d)[0].topline`, id)) }

	x, y := r.at(cols-5, 8) // over the right window
	r.tt.Wheel(x, y, 0, 40)
	r.wait("the right window to scroll", func() bool { return top(right) == 4 })
	if got := top(left); got != 1 {
		t.Errorf("the left window scrolled too, its top line is %d", got)
	}
	x, y = r.at(5, 8)
	r.tt.Wheel(x, y, 0, 40)
	r.tt.Wheel(x, y, 0, 40)
	r.wait("the left window to scroll", func() bool { return top(left) == 7 })
	if got := top(right); got != 4 {
		t.Errorf("the right window's top line changed to %d", got)
	}
}

func TestRealNvimWheelSideways(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wide.txt")
	if err := os.WriteFile(p, []byte(strings.Repeat("0123456789", 40)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := startRealWheel(t, p, "--cmd", "set nowrap nonumber")
	r.wait("the file", func() bool { return strings.HasPrefix(r.row(0), "0123456789") })
	x, y := r.at(10, 0)
	r.tt.Wheel(x, y, 40, 0)
	r.wait("one step right", func() bool { return strings.HasPrefix(r.row(0), "6789012345") })
	r.tt.Wheel(x, y, -40, 0)
	r.wait("one step left", func() bool { return strings.HasPrefix(r.row(0), "0123456789") })
}

func TestRealNvimWheelFollowsTheMouseOption(t *testing.T) {
	r := startRealWheel(t, writeLong(t), "--cmd", "set nonumber")
	r.wait("the file", func() bool { return r.row(0) == "line 1" })
	x, y := r.at(10, 8)
	off := func() (b bool) { r.ed.Model.Read(func(s *grid.State) { b = s.MouseOff }); return }

	r.sess.Command("set mouse=")
	r.wait("Neovim to say the mouse is off", off)
	r.tt.Wheel(x, y, 0, 40)
	time.Sleep(300 * time.Millisecond)
	if got := r.row(0); got != "line 1" {
		t.Errorf("the wheel scrolled to %q with 'mouse' empty", got)
	}
	r.sess.Command("set mouse=a")
	r.wait("Neovim to say the mouse is on", func() bool { return !off() })
	r.tt.Wheel(x, y, 0, 40)
	r.wait("the wheel to work again", func() bool { return r.row(0) == "line 4" })
}
