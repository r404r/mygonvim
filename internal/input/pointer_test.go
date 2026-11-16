package input

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPointerButtonsActionsAndModifiers(t *testing.T) {
	for button, name := range []string{"left", "right", "middle"} {
		t.Run(name, func(t *testing.T) {
			var p Pointer
			for i, kind := range []ui.InputKind{ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp} {
				ev := ui.InputEvent{Kind: kind, Button: button, Mods: ui.Shift | ui.Ctrl | ui.Alt | ui.Super, Clicks: 3}
				got, taken := p.Handle(ev, i, i+1, true)
				want := MouseEvent{name, []string{"press", "drag", "release"}[i], "SCAD", i, i + 1}
				if !taken || got != want {
					t.Fatalf("event %d: got %+v, taken=%v; want %+v", i, got, taken, want)
				}
			}
		})
	}
}

func TestPointerSuppressesDisabledGestureUntilRelease(t *testing.T) {
	var p Pointer
	for i, kind := range []ui.InputKind{ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp} {
		got, taken := p.Handle(ui.InputEvent{Kind: kind}, i, i, i != 0)
		if !taken || got.Button != "" {
			t.Fatalf("disabled gesture sent %+v, taken=%v", got, taken)
		}
	}
	got, _ := p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 1, true)
	if got.Action != "press" {
		t.Fatalf("next gesture: %+v", got)
	}
}

func TestPointerReleasesAfterMouseOffOrCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		var p Pointer
		p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 2, true)
		if got, _ := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 3, 4, false); got.Button != "" {
			t.Fatalf("drag after mouse_off: %+v", got)
		}
		got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerUp, Canceled: canceled}, 3, 4, false)
		if !taken || got != (MouseEvent{"left", "release", "", 3, 4}) {
			t.Fatalf("canceled=%v: %+v, taken=%v", canceled, got, taken)
		}
		if got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 5, 6, true); taken || got.Button != "" {
			t.Fatalf("drag after release: %+v, taken=%v", got, taken)
		}
	}
}

func TestPointerIgnoresHoverUnknownButtonsAndStationaryDrag(t *testing.T) {
	var p Pointer
	for _, ev := range []ui.InputEvent{
		{Kind: ui.InputPointerMove, Button: -1},
		{Kind: ui.InputPointerMove}, {Kind: ui.InputPointerUp},
		{Kind: ui.InputPointerDown, Button: 3}, {Kind: ui.InputText},
	} {
		if got, taken := p.Handle(ev, 1, 1, true); taken || got.Button != "" {
			t.Fatalf("took %+v: %+v", ev, got)
		}
	}
	p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 1, true)
	if got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 1, 1, true); !taken || got.Button != "" {
		t.Fatalf("same cell: %+v, taken=%v", got, taken)
	}
	// A second button has its own press and must not release the first.
	p.Handle(ui.InputEvent{Kind: ui.InputPointerDown, Button: 1}, 1, 1, true)
	p.Handle(ui.InputEvent{Kind: ui.InputPointerUp, Button: 1}, 1, 1, true)
	if got, _ := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 2, 2, true); got.Action != "drag" || got.Button != "left" {
		t.Fatalf("lost first button: %+v", got)
	}
}
