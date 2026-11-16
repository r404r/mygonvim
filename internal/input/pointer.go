package input

import "github.com/egoist/mygo/ui"

// MouseEvent is one nvim_input_mouse call. An empty Button means no call.
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
	buttons [3]gesture
}

// Handle translates a pointer event at a clamped grid cell. A disabled press
// suppresses the whole gesture; a sent press always gets its release, including
// canceled releases and releases after mouse_off. The bool takes the event so
// MyGo keeps delivering the captured gesture outside the editor's bounds.
func (p *Pointer) Handle(ev ui.InputEvent, row, col int, enabled bool) (MouseEvent, bool) {
	if ev.Button < 0 || ev.Button >= len(p.buttons) {
		return MouseEvent{}, false
	}
	g := &p.buttons[ev.Button]
	action := ""
	switch ev.Kind {
	case ui.InputPointerDown:
		if g.held {
			return MouseEvent{}, true
		}
		*g = gesture{held: true, sent: enabled, row: row, col: col}
		if enabled {
			action = "press"
		}
	case ui.InputPointerMove:
		if !g.held {
			return MouseEvent{}, false
		}
		if g.sent && enabled && (row != g.row || col != g.col) {
			action = "drag"
			g.row, g.col = row, col
		}
	case ui.InputPointerUp:
		if !g.held {
			return MouseEvent{}, false
		}
		if g.sent {
			action = "release"
		}
		*g = gesture{}
	default:
		return MouseEvent{}, false
	}
	if action == "" {
		return MouseEvent{}, true
	}
	return MouseEvent{
		Button: [...]string{"left", "right", "middle"}[ev.Button],
		Action: action, Modifier: Modifier(ev.Mods), Row: row, Col: col,
	}, true
}
