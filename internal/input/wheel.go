package input

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

// Neovim scrolls by whole steps of its mousescroll option, which defaults to
// "ver:3,hor:6": a wheel step moves the view three lines or six columns.
const (
	LinesPerStep = 3
	ColsPerStep  = 6
)

// maxStepsPerEvent keeps one huge delta (a fast fling) from flooding nvim.
const maxStepsPerEvent = 50

// gap is how long a touchpad may pause before the part of a step it has built
// up is forgotten, so that a new flick starts from zero.
const gap = 300 * time.Millisecond

// notchDIPs is how far one notch of a mouse wheel scrolls on each system, as
// MyGo reports it: 40 on macOS (a line, as in browsers), 33 per line of the
// system setting on Windows (99 at the default of three), 50 on Linux.
func notchDIPs(goos string) float64 {
	switch goos {
	case "windows":
		return 99
	case "linux":
		return 50
	}
	return 40
}

// Step is one nvim_input_mouse wheel call: Action is "up", "down", "left" or
// "right", and Modifier is the string of modifier letters ("" for none).
type Step struct {
	Action   string
	Modifier string
}

// Wheel turns MyGo's scroll events into wheel steps for Neovim. Main thread
// only.
//
// A mouse wheel (ui.InputEvent.Precise false) gives one step per notch, a few
// more when the system accelerates a fast turn. A touchpad (Precise) gives
// pixels: they build up until they reach what one Neovim step scrolls, three
// lines or six columns, and each time they do a step goes out, so that the
// view follows the fingers at the speed of a mouse wheel and a small move does
// nothing until it adds up. Momentum after the fingers leave arrives as more
// precise events and goes the same way. The sign is the system's: MyGo reports
// DY > 0 when the view moves down the content, after the user's natural
// scrolling setting.
type Wheel struct {
	// GOOS is the system whose wheel is read: runtime.GOOS, set by tests.
	GOOS string

	accX, accY float64
	last       time.Time
}

// NewWheel returns a Wheel for this system.
func NewWheel() *Wheel { return &Wheel{GOOS: runtime.GOOS} }

// Steps reads one InputScroll event. cellW and cellH are the size of a cell in
// DIPs. Vertical steps come first, then horizontal ones.
func (w *Wheel) Steps(ev ui.InputEvent, cellW, cellH float32) []Step {
	if ev.Kind != ui.InputScroll || cellW <= 0 || cellH <= 0 {
		return nil
	}
	mod := Modifier(ev.Mods)
	var out []Step
	if ev.Precise {
		if !w.last.IsZero() && !ev.Time.IsZero() && ev.Time.Sub(w.last) > gap {
			w.accX, w.accY = 0, 0
		}
		if !ev.Time.IsZero() {
			w.last = ev.Time
		}
		var n int
		n, w.accY = takeSteps(w.accY, float64(ev.DY), LinesPerStep*float64(cellH))
		out = appendSteps(out, "up", "down", n, mod)
		n, w.accX = takeSteps(w.accX, float64(ev.DX), ColsPerStep*float64(cellW))
		return appendSteps(out, "left", "right", n, mod)
	}
	w.accX, w.accY, w.last = 0, 0, time.Time{}
	notch := notchDIPs(w.GOOS)
	out = appendSteps(out, "up", "down", notches(float64(ev.DY), notch), mod)
	return appendSteps(out, "left", "right", notches(float64(ev.DX), notch), mod)
}

// notches is the signed count of notches in a mouse wheel delta. A delta
// smaller than a notch of the system's default (a lower "lines to scroll"
// setting) still counts as one.
func notches(d, notch float64) int {
	if d == 0 {
		return 0
	}
	n := int(abs(d)/notch + 0.5)
	if n < 1 {
		n = 1
	}
	if d < 0 {
		return -n
	}
	return n
}

// takeSteps adds d to the pixels carried over in acc and returns the whole
// steps of size they make (negative for the negative direction) and what is
// left over. A move the other way than the carried pixels drops them.
func takeSteps(acc, d, size float64) (steps int, rest float64) {
	if d == 0 {
		return 0, acc
	}
	if acc != 0 && (acc < 0) != (d < 0) {
		acc = 0
	}
	acc += d
	steps = int(acc / size) // truncates toward zero
	return steps, acc - float64(steps)*size
}

// appendSteps appends |n| steps, negative going to neg, positive to pos.
func appendSteps(out []Step, neg, pos string, n int, mod string) []Step {
	action := pos
	if n < 0 {
		action, n = neg, -n
	}
	n = min(n, maxStepsPerEvent)
	for ; n > 0; n-- {
		out = append(out, Step{Action: action, Modifier: mod})
	}
	return out
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// Modifier is the modifier string of nvim_input_mouse: S (shift), C (ctrl),
// A (alt) and D (super, Command on macOS), in that order.
func Modifier(m ui.Modifiers) string {
	var b [4]byte
	n := 0
	for _, x := range [...]struct {
		m ui.Modifiers
		c byte
	}{{ui.Shift, 'S'}, {ui.Ctrl, 'C'}, {ui.Alt, 'A'}, {ui.Super, 'D'}} {
		if m&x.m != 0 {
			b[n] = x.c
			n++
		}
	}
	return string(b[:n])
}

// CellAt returns the grid row and column under the point x, y (DIPs from the
// top left of the grid), clamped to a cols x rows grid. cellW and cellH are
// the size of a cell in DIPs.
func CellAt(x, y, cellW, cellH float32, cols, rows int) (row, col int) {
	if cellW <= 0 || cellH <= 0 {
		return 0, 0
	}
	col = int(max(x, 0) / cellW)
	row = int(max(y, 0) / cellH)
	return max(0, min(row, rows-1)), max(0, min(col, cols-1))
}
