package gui

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPointerCapturedOutsideWindowAndClamped(t *testing.T) {
	e := newEnv(t, 640, 400)
	x, y := cellCenter(e, 2, 3)
	e.tt.Press(float32(x), float32(y))
	e.tt.Move(float32(x+1), float32(y+1))
	e.tt.Move(-100, -100)
	e.tt.Move(10000, 10000)
	e.tt.Release(10000, 10000)
	cols, rows := e.ed.Model.Size()
	want := []mouseCall{
		{"left", "press", "", 3, 2},
		{"left", "drag", "", 0, 0},
		{"left", "drag", "", rows - 1, cols - 1},
		{"left", "release", "", rows - 1, cols - 1},
	}
	if got := e.mouse(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestPointerCaptureCancellationStillReleases(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	x, y := cellCenter(e, 2, 3)
	e.tt.Press(float32(x), float32(y))
	e.ed.OnRedraw(ev("mouse_off"))
	e.tt.SetFocused(false)
	want := []mouseCall{{"left", "press", "", 3, 2}, {"left", "release", "", 3, 2}}
	if got := e.mouse(); !reflect.DeepEqual(got, want) {
		t.Fatalf("lost canceled release: %v; want %v", got, want)
	}
}

func TestPointerMouseOffDropsWholeGesture(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.ed.OnRedraw(ev("mouse_off"))
	e.tt.Press(20, 20)
	e.ed.OnRedraw(ev("mouse_on"))
	e.tt.Move(100, 100)
	e.tt.Release(100, 100)
	if got := e.mouse(); len(got) != 0 {
		t.Fatalf("partial gesture: %v", got)
	}
	e.tt.ClickAt(20, 20)
	if got := e.mouse(); len(got) != 2 || got[0].action != "press" || got[1].action != "release" {
		t.Fatalf("next click: %v", got)
	}
}

func TestPointerButtonsAndModifiersReachBackend(t *testing.T) {
	for button, name := range []string{"left", "right", "middle"} {
		e := newEnv(t, 640, 400)
		x, y := cellCenter(e, 4, 5)
		for _, kind := range []ui.InputKind{ui.InputPointerDown, ui.InputPointerUp} {
			if !e.ed.handle(ui.InputEvent{Kind: kind, Button: button, X: float32(x), Y: float32(y), Mods: ui.Shift | ui.Ctrl | ui.Alt | ui.Super}) {
				t.Fatal("pointer not taken")
			}
		}
		want := []mouseCall{{name, "press", "SCAD", 5, 4}, {name, "release", "SCAD", 5, 4}}
		if got := e.mouse(); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v; want %v", got, want)
		}
	}
}

func TestPointerBeforeFirstFrameIsLeftAlone(t *testing.T) {
	be := &fake{}
	ed := NewEditor(be, Config{}, Hooks{})
	if ed.handle(ui.InputEvent{Kind: ui.InputPointerDown}) || len(be.mice) != 0 {
		t.Fatal("pointer taken before cell metrics are known")
	}
}
