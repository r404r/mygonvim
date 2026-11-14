package gui

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// wheelEnv is an editor whose wheel reads like macOS's: a notch is 40 DIPs.
func wheelEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, 640, 400)
	e.ed.wheel.GOOS = "darwin"
	return e
}

func (e *env) mouse() []mouseCall {
	e.be.mu.Lock()
	defer e.be.mu.Unlock()
	return append([]mouseCall(nil), e.be.mice...)
}

func TestWheelNotchScrollsTheCellUnderThePointer(t *testing.T) {
	e := wheelEnv(t)
	x, y := cellCenter(e, 10, 5)
	e.tt.Wheel(float32(x), float32(y), 0, 40)
	want := []mouseCall{{"wheel", "down", "", 5, 10}}
	if got := e.mouse(); !reflect.DeepEqual(got, want) {
		t.Fatalf("wheel down sent %v, want %v", got, want)
	}
	x, y = cellCenter(e, 70, 20)
	e.tt.Wheel(float32(x), float32(y), 0, -40)
	want = append(want, mouseCall{"wheel", "up", "", 20, 70})
	if got := e.mouse(); !reflect.DeepEqual(got, want) {
		t.Fatalf("wheel up sent %v, want %v", got, want)
	}
	if keys := e.be.sent(); len(keys) != 0 {
		t.Errorf("a scroll also sent keys %v", keys)
	}
}

func TestWheelHorizontal(t *testing.T) {
	e := wheelEnv(t)
	x, y := cellCenter(e, 3, 2)
	e.tt.Wheel(float32(x), float32(y), 40, 0)
	e.tt.Wheel(float32(x), float32(y), -40, 0)
	want := []mouseCall{{"wheel", "right", "", 2, 3}, {"wheel", "left", "", 2, 3}}
	if got := e.mouse(); !reflect.DeepEqual(got, want) {
		t.Errorf("sideways wheel sent %v, want %v", got, want)
	}
}

func TestTouchpadPixelsBuildUpToSteps(t *testing.T) {
	e := wheelEnv(t)
	x, y := cellCenter(e, 4, 4)
	step := 3 * e.ed.ch // what one Neovim step scrolls
	// A third of a step at a time: the third event completes the first step.
	for i := 0; i < 2; i++ {
		e.tt.Scroll(float32(x), float32(y), 0, step/3)
	}
	if got := e.mouse(); len(got) != 0 {
		t.Fatalf("two thirds of a step sent %v", got)
	}
	e.tt.Scroll(float32(x), float32(y), 0, step/3+0.5)
	if got, want := e.mouse(), []mouseCall{{"wheel", "down", "", 4, 4}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("a whole step sent %v, want %v", got, want)
	}
	// Two steps and a bit in the other way, in one event.
	e.tt.Scroll(float32(x), float32(y), 0, -2.2*step)
	got := e.mouse()
	if len(got) != 3 || got[1].action != "up" || got[2].action != "up" {
		t.Errorf("2.2 steps up gave %v", got[1:])
	}
}

func TestScrollModifiersReachNeovim(t *testing.T) {
	e := wheelEnv(t)
	x, y := cellCenter(e, 1, 1)
	ev := ui.InputEvent{Kind: ui.InputScroll, DY: 40, Mods: ui.Ctrl | ui.Shift, X: float32(x), Y: float32(y), Time: time.Now()}
	if !e.ed.handle(ev) {
		t.Fatal("the scroll was not taken")
	}
	if got, want := e.mouse(), []mouseCall{{"wheel", "down", "SC", 1, 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestScrollIsTakenSoNoContainerAroundItScrolls(t *testing.T) {
	e := wheelEnv(t)
	if !e.ed.handle(ui.InputEvent{Kind: ui.InputScroll, DY: 1, Precise: true, X: 5, Y: 5}) {
		t.Error("a scroll too small for a step was not taken")
	}
	if got := e.mouse(); len(got) != 0 {
		t.Errorf("a tiny scroll sent %v", got)
	}
}

func TestScrollBeforeTheFirstFrameIsLeftAlone(t *testing.T) {
	be := &fake{}
	ed := NewEditor(be, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{})
	if ed.handle(ui.InputEvent{Kind: ui.InputScroll, DY: 40}) {
		t.Error("took a scroll with no cell size known")
	}
	if len(be.mice) != 0 {
		t.Errorf("sent %v", be.mice)
	}
}

func TestScrollPointPastTheGridIsClamped(t *testing.T) {
	e := wheelEnv(t)
	cols, rows := e.ed.Model.Size()
	e.ed.handle(ui.InputEvent{Kind: ui.InputScroll, DY: 40, X: 100000, Y: 100000})
	if got, want := e.mouse(), []mouseCall{{"wheel", "down", "", rows - 1, cols - 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestWheelIsDroppedWhileNeovimHasTheMouseOff(t *testing.T) {
	e := wheelEnv(t)
	x, y := cellCenter(e, 2, 2)
	e.ed.OnRedraw(ev("mouse_off"))
	e.tt.Wheel(float32(x), float32(y), 0, 40)
	if got := e.mouse(); len(got) != 0 {
		t.Fatalf("sent %v with the mouse off", got)
	}
	// The event is still the editor's: no container around it scrolls.
	if !e.ed.handle(ui.InputEvent{Kind: ui.InputScroll, DY: 40, X: float32(x), Y: float32(y)}) {
		t.Error("the scroll was not taken with the mouse off")
	}
	e.ed.OnRedraw(ev("mouse_on"))
	e.tt.Wheel(float32(x), float32(y), 0, 40)
	if got, want := e.mouse(), []mouseCall{{"wheel", "down", "", 2, 2}}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v after mouse_on, want %v", got, want)
	}
}
