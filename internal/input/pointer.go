package input

import "github.com/egoist/mygo/ui"

// MouseEvent is one nvim_input_mouse call.
type MouseEvent struct {
	Button, Action, Modifier string
	Row, Col                 int
}

type gesture struct {
	held, sent bool
	row, col   int
}

// Pointer tracks button gestures on the main thread. Its zero value is ready.
type Pointer struct {
	gesture gesture
	button  int
}

// Handle translates a pointer event at a clamped grid cell. Like MyGo, it tracks
// one gesture: a new Down releases any sent press before starting another, even
// for the same button. That release uses the current clamped cell and modifiers.
// A disabled press suppresses its gesture; a sent press gets its release even
// after mouse_off or cancellation. The bool takes the event so MyGo delivers
// the captured gesture outside the editor's bounds.
func (p *Pointer) Handle(ev ui.InputEvent, row, col int, enabled bool) ([]MouseEvent, bool) {
	buttons := [...]string{"left", "right", "middle"}
	if ev.Button < 0 || ev.Button >= len(buttons) {
		return nil, false
	}
	g := &p.gesture
	var events []MouseEvent
	emit := func(button int, action string) {
		events = append(events, MouseEvent{buttons[button], action, Modifier(ev.Mods), row, col})
	}
	action := ""
	switch ev.Kind {
	case ui.InputPointerDown:
		if g.sent {
			emit(p.button, "release")
		}
		*g = gesture{held: true, sent: enabled, row: row, col: col}
		p.button = ev.Button
		if enabled {
			action = "press"
		}
	case ui.InputPointerMove:
		if !g.held || ev.Button != p.button {
			return nil, false
		}
		if g.sent && enabled && (row != g.row || col != g.col) {
			action = "drag"
			g.row, g.col = row, col
		}
	case ui.InputPointerUp:
		if !g.held || ev.Button != p.button {
			return nil, false
		}
		if g.sent {
			action = "release"
		}
		*g = gesture{}
	default:
		return nil, false
	}
	if action != "" {
		emit(ev.Button, action)
	}
	return events, true
}
