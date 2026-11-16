package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/grid"
)

func startRealPointer(t *testing.T, args ...string) *realWheel {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mouse.txt")
	if err := os.WriteFile(p, []byte("alpha beta gamma\nsecond line here\nthird line here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := startRealWheel(t, p, append([]string{"--cmd", "set nonumber norelativenumber mouse=a mousetime=5000"}, args...)...)
	r.wait("mouse fixture", func() bool { return strings.HasPrefix(r.row(0), "alpha beta gamma") })
	// This mapping is an input barrier, ordered after queued mouse calls and
	// processed by Neovim after their effects, including the negative checks.
	if err := r.sess.Nvim().Command("let g:pointer_sync = 0 | noremap <F12> <Cmd>let g:pointer_sync += 1<CR>"); err != nil {
		t.Fatal(err)
	}
	return r
}

func pointerSync(r *realWheel) {
	r.t.Helper()
	want := r.eval("g:pointer_sync") + 1
	r.sess.Input("<F12>")
	r.wait("mouse input barrier", func() bool { return r.eval("g:pointer_sync") == want })
}

func pointerClick(r *realWheel, button, col, row int) {
	r.t.Helper()
	x, y := r.at(col, row)
	r.tt.PressButton(button, x, y)
	pointerSync(r)
	r.tt.ReleaseButton(button, x, y)
	pointerSync(r)
}

func TestRealNvimPointerClickMovesCursor(t *testing.T) {
	r := startRealPointer(t)
	pointerClick(r, 0, 7, 1)
	if line, col := r.eval("line('.')"), r.eval("col('.')"); line != 2 || col != 8 {
		t.Fatalf("cursor = (%d,%d), want (2,8)", line, col)
	}
	// A key after a click still goes to the focused editor.
	r.tt.Type("l")
	pointerSync(r)
	if col := r.eval("col('.')"); col != 9 {
		t.Fatalf("keyboard after click: col=%d, want 9", col)
	}
}

func TestRealNvimPointerClickAfterOverlappingButtons(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("focusLoss=%v", cancel), func(t *testing.T) {
			r := startRealPointer(t, "--cmd", "set mousemodel=extend")
			r.tt.SetFocused(true)
			x, y := r.at(2, 0)
			r.tt.Press(x, y)
			pointerSync(r)
			r.tt.PressButton(1, x, y)
			pointerSync(r)
			if cancel {
				r.tt.SetFocused(false)
			}
			r.tt.ReleaseButton(1, x, y)
			r.tt.Release(x, y)
			if cancel {
				r.tt.SetFocused(true)
			}
			pointerSync(r)
			pointerClick(r, 0, 7, 2)
			if line, col := r.eval("line('.')"), r.eval("col('.')"); line != 3 || col != 8 {
				t.Fatalf("next click: cursor=(%d,%d), want (3,8)", line, col)
			}
		})
	}
}

func TestRealNvimPointerDragSelectsText(t *testing.T) {
	r := startRealPointer(t)
	x, y := r.at(6, 0)
	r.tt.Press(x, y)
	pointerSync(r)
	x, y = r.at(9, 0)
	r.tt.Move(x, y)
	pointerSync(r)
	r.tt.Release(x, y)
	pointerSync(r)
	if r.eval("mode() ==# 'v'") != 1 || r.eval("getpos('v')[1]") != 1 || r.eval("getpos('v')[2]") != 7 || r.eval("col('.')") != 10 {
		t.Fatal("drag did not select columns 7 through 10 in Visual mode")
	}
	r.sess.Input("y")
	pointerSync(r)
	var selected string
	if err := r.sess.Nvim().Eval(`getreg('"')`, &selected); err != nil || selected != "beta" {
		t.Fatalf("selected %q, err=%v; want beta", selected, err)
	}
}

func TestRealNvimPointerClickSwitchesSplit(t *testing.T) {
	r := startRealPointer(t, "-c", "vsplit")
	r.wait("split", func() bool { return r.eval("winnr('$')") == 2 })
	left, right := r.eval("win_getid(1)"), r.eval("win_getid(2)")
	cols, _ := r.ed.Model.Size()
	pointerClick(r, 0, cols-5, 0)
	if got := r.eval("win_getid()"); got != right {
		t.Fatalf("current window=%d, want right %d", got, right)
	}
	pointerClick(r, 0, 3, 0)
	if got := r.eval("win_getid()"); got != left {
		t.Fatalf("current window=%d, want left %d", got, left)
	}
}

func TestRealNvimPointerMouseOptionNeedsGUIFilter(t *testing.T) {
	r := startRealPointer(t)
	if err := r.sess.Nvim().Command("set mouse="); err != nil {
		t.Fatal(err)
	}
	off := func() (b bool) { r.ed.Model.Read(func(s *grid.State) { b = s.MouseOff }); return }
	r.wait("mouse_off", off)
	pointerClick(r, 0, 7, 1)
	if r.eval("line('.')") != 1 || r.eval("col('.')") != 1 {
		t.Fatal("GUI forwarded a click with mouse disabled")
	}
	// Bypassing the GUI proves that the API itself does not enforce 'mouse'.
	r.sess.Mouse("left", "press", "", 1, 7)
	pointerSync(r)
	r.sess.Mouse("left", "release", "", 1, 7)
	pointerSync(r)
	if r.eval("line('.')") != 2 || r.eval("col('.')") != 8 {
		t.Fatal("direct Session.Mouse did not bypass 'mouse'")
	}
	if err := r.sess.Nvim().Command("set mouse=a"); err != nil {
		t.Fatal(err)
	}
	r.wait("mouse_on", func() bool { return !off() })
	pointerClick(r, 0, 2, 0)
	if r.eval("line('.')") != 1 || r.eval("col('.')") != 3 {
		t.Fatal("mouse_on did not restore clicks")
	}
}

func TestRealNvimPointerMultipleClicksSelectWordAndLine(t *testing.T) {
	r := startRealPointer(t)
	pointerClick(r, 0, 7, 0)
	pointerClick(r, 0, 7, 0)
	if r.eval("mode() ==# 'v'") != 1 || r.eval("getpos('v')[2]") != 7 || r.eval("col('.')") != 10 {
		t.Fatal("double click did not select beta")
	}
	pointerClick(r, 0, 7, 0)
	if r.eval("mode() ==# 'V'") != 1 {
		t.Fatal("third click did not select the line")
	}
}

func TestRealNvimPointerRightClickUsesNeovimPopup(t *testing.T) {
	r := startRealPointer(t)
	if r.eval(`&mousemodel ==# 'popup_setpos'`) != 1 {
		t.Fatal("expected Neovim's default mousemodel")
	}
	x, y := r.at(7, 1)
	r.tt.PressButton(1, x, y)
	// A popup runs a nested input loop; inspect redraw rather than making a
	// synchronous Eval request that may wait until the menu is dismissed.
	r.wait("Neovim popup", func() bool {
		var found bool
		r.ed.Model.Read(func(s *grid.State) {
			for row := 0; row < s.Rows; row++ {
				found = found || strings.Contains(s.RowText(row), "Inspect")
			}
		})
		return found
	})
	r.tt.ReleaseButton(1, x, y)
	r.sess.Input("<Esc>")
	pointerSync(r)
	if r.eval("line('.')") != 2 || r.eval("col('.')") != 8 {
		t.Fatal("popup_setpos did not move the cursor")
	}
}

func TestRealNvimPointerMiddleClickPutsRegister(t *testing.T) {
	r := startRealPointer(t)
	if err := r.sess.Nvim().Command(`let @a = 'PUT'`); err != nil {
		t.Fatal(err)
	}
	// An explicit register tests the default middle-button put without
	// depending on a runner's external system clipboard provider.
	r.sess.Input(`"a`)
	pointerClick(r, 2, 3, 0)
	var line string
	if err := r.sess.Nvim().Eval("getline(1)", &line); err != nil || line != "alpPUTha beta gamma" {
		t.Fatalf("middle put: %q, err=%v", line, err)
	}
}

func TestRealNvimPointerModifiersReachMappings(t *testing.T) {
	for _, mod := range []struct {
		name string
		mods ui.Modifiers
	}{{"S", ui.Shift}, {"C", ui.Ctrl}, {"A", ui.Alt}, {"D", ui.Super}} {
		t.Run(mod.name, func(t *testing.T) {
			r := startRealPointer(t)
			cmd := fmt.Sprintf("let g:modified = 0 | nnoremap <%s-LeftMouse> <Cmd>let g:modified += 1<CR>", mod.name)
			if err := r.sess.Nvim().Command(cmd); err != nil {
				t.Fatal(err)
			}
			x, y := r.at(2, 1)
			r.tt.ClickAtWith(mod.mods, x, y)
			pointerSync(r)
			if got := r.eval("g:modified"); got != 1 {
				t.Fatalf("%s mapping ran %d times", mod.name, got)
			}
		})
	}
}
