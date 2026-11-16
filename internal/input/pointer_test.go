package input

import (
	"fmt"
	"slices"
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
				if !taken || !slices.Equal(got, []MouseEvent{want}) {
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
		if !taken || len(got) != 0 {
			t.Fatalf("disabled gesture sent %+v, taken=%v", got, taken)
		}
	}
	got, _ := p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 1, true)
	if len(got) != 1 || got[0].Action != "press" {
		t.Fatalf("next gesture: %+v", got)
	}
}

func TestPointerReleasesAfterMouseOffOrCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		var p Pointer
		p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 2, true)
		if got, _ := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 3, 4, false); len(got) != 0 {
			t.Fatalf("drag after mouse_off: %+v", got)
		}
		got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerUp, Canceled: canceled}, 3, 4, false)
		if !taken || !slices.Equal(got, []MouseEvent{{"left", "release", "", 3, 4}}) {
			t.Fatalf("canceled=%v: %+v, taken=%v", canceled, got, taken)
		}
		if got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 5, 6, true); taken || len(got) != 0 {
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
		if got, taken := p.Handle(ev, 1, 1, true); taken || len(got) != 0 {
			t.Fatalf("took %+v: %+v", ev, got)
		}
	}
	p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 1, 1, true)
	if got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerMove}, 1, 1, true); !taken || len(got) != 0 {
		t.Fatalf("same cell: %+v, taken=%v", got, taken)
	}
}

func TestPointerNewDownReleasesPreviousGesture(t *testing.T) {
	for button, name := range []string{"left", "right", "middle"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%v", name, enabled), func(t *testing.T) {
				var p Pointer
				p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 8, 9, true)
				got, taken := p.Handle(ui.InputEvent{Kind: ui.InputPointerDown, Button: button, Mods: ui.Ctrl}, 1, 2, enabled)
				want := []MouseEvent{{"left", "release", "C", 1, 2}}
				if enabled {
					want = append(want, MouseEvent{name, "press", "C", 1, 2})
				}
				if !taken || !slices.Equal(got, want) {
					t.Fatalf("replacement: got %v, taken=%v; want %v", got, taken, want)
				}
				if button != 0 {
					for _, kind := range []ui.InputKind{ui.InputPointerMove, ui.InputPointerUp} {
						if got, taken := p.Handle(ui.InputEvent{Kind: kind}, 3, 4, true); taken || len(got) != 0 {
							t.Fatalf("old button: got %v, taken=%v", got, taken)
						}
					}
				}
				got, taken = p.Handle(ui.InputEvent{Kind: ui.InputPointerUp, Button: button, Canceled: true}, 3, 4, false)
				want = nil
				if enabled {
					want = []MouseEvent{{name, "release", "", 3, 4}}
				}
				if !taken || !slices.Equal(got, want) {
					t.Fatalf("release: got %v, taken=%v; want %v", got, taken, want)
				}
				got, taken = p.Handle(ui.InputEvent{Kind: ui.InputPointerDown}, 4, 5, true)
				if !taken || !slices.Equal(got, []MouseEvent{{"left", "press", "", 4, 5}}) {
					t.Fatalf("next press: got %v, taken=%v", got, taken)
				}
			})
		}
	}
}
