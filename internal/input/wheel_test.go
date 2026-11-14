package input

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func scroll(dx, dy float32, precise bool, mods ui.Modifiers, at time.Time) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputScroll, DX: dx, DY: dy, Precise: precise, Mods: mods, Time: at}
}

func actions(steps []Step) string {
	var s string
	for _, st := range steps {
		s += st.Action[:1]
	}
	return s
}

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestMouseWheelNotchIsOneStepPerSystem(t *testing.T) {
	for _, c := range []struct {
		goos      string
		dy        float32
		want      string
		wantFirst string
	}{
		{"darwin", 40, "d", "down"},
		{"darwin", -40, "u", "up"},
		{"darwin", 200, "ddddd", "down"}, // an accelerated turn: five lines
		{"darwin", 16, "d", "down"},      // less than a notch still moves
		{"windows", 99, "d", "down"},     // 3 lines of 33 DIPs
		{"windows", -99, "u", "up"},
		{"windows", 33, "d", "down"}, // "lines to scroll" set to 1
		{"windows", 198, "dd", "down"},
		{"linux", 50, "d", "down"},
	} {
		w := &Wheel{GOOS: c.goos}
		got := w.Steps(scroll(0, c.dy, false, 0, t0), 8, 16)
		if actions(got) != c.want {
			t.Errorf("%s dy=%v: steps %q, want %q", c.goos, c.dy, actions(got), c.want)
		}
		if len(got) > 0 && got[0].Action != c.wantFirst {
			t.Errorf("%s dy=%v: first action %q, want %q", c.goos, c.dy, got[0].Action, c.wantFirst)
		}
	}
}

func TestHorizontalWheel(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	if got := actions(w.Steps(scroll(40, 0, false, 0, t0), 8, 16)); got != "r" {
		t.Errorf("DX>0 gave %q, want one right", got)
	}
	if got := actions(w.Steps(scroll(-80, 0, false, 0, t0), 8, 16)); got != "ll" {
		t.Errorf("DX=-80 gave %q, want two left", got)
	}
	// Both axes in one event: vertical first.
	if got := actions(w.Steps(scroll(40, 40, false, 0, t0), 8, 16)); got != "dr" {
		t.Errorf("diagonal gave %q, want down then right", got)
	}
}

func TestNoDeltaNoSteps(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	for _, precise := range []bool{false, true} {
		if got := w.Steps(scroll(0, 0, precise, 0, t0), 8, 16); len(got) != 0 {
			t.Errorf("precise=%v: zero delta gave %v", precise, got)
		}
	}
	if got := w.Steps(ui.InputEvent{Kind: ui.InputText, Text: "a"}, 8, 16); got != nil {
		t.Errorf("a text event gave %v", got)
	}
	if got := w.Steps(scroll(0, 40, false, 0, t0), 0, 0); got != nil {
		t.Errorf("unknown cell size gave %v", got)
	}
}

func TestTouchpadAccumulatesToOneStepPerThreeLines(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	const cw, ch = 8, 16 // a step is 3 lines = 48 DIPs down, 6 columns = 48 DIPs across
	// 5 DIPs at a time: nothing until 48 have built up, then one step.
	var total int
	var steps []Step
	for i := 0; i < 20; i++ {
		got := w.Steps(scroll(0, 5, true, 0, t0.Add(time.Duration(i)*16*time.Millisecond)), cw, ch)
		if len(got) > 0 && total+len(got) > 0 && i < 9 {
			t.Fatalf("event %d (%d DIPs so far) already sent %v", i, (i+1)*5, got)
		}
		total += len(got)
		steps = append(steps, got...)
	}
	// 100 DIPs in all: two whole steps of 48, 4 left over.
	if total != 2 {
		t.Errorf("100 DIPs gave %d steps, want 2", total)
	}
	for _, s := range steps {
		if s.Action != "down" {
			t.Errorf("step %v, want down", s)
		}
	}
}

func TestTouchpadBigDeltaGivesSeveralSteps(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	if got := actions(w.Steps(scroll(0, -150, true, 0, t0), 8, 16)); got != "uuu" {
		t.Errorf("-150 DIPs gave %q, want three ups (3 x 48)", got)
	}
	// 150 - 144 = 6 left over, in the same direction: 42 more finish a step.
	if got := actions(w.Steps(scroll(0, -42, true, 0, t0.Add(10*time.Millisecond)), 8, 16)); got != "u" {
		t.Errorf("the carried 6 DIPs plus 42 gave %q, want one up", got)
	}
}

func TestTouchpadStepSizeFollowsCellSize(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	if got := actions(w.Steps(scroll(0, 40, true, 0, t0), 8, 20)); got != "" { // 3 x 20 = 60
		t.Errorf("40 DIPs with 20 DIP lines gave %q, want none", got)
	}
	w = &Wheel{GOOS: "darwin"}
	if got := actions(w.Steps(scroll(0, 60, true, 0, t0), 8, 20)); got != "d" {
		t.Errorf("60 DIPs with 20 DIP lines gave %q, want one", got)
	}
	w = &Wheel{GOOS: "darwin"}
	if got := actions(w.Steps(scroll(48, 0, true, 0, t0), 8, 20)); got != "r" { // 6 x 8 = 48
		t.Errorf("48 DIPs across with 8 DIP columns gave %q, want one right", got)
	}
}

func TestTouchpadReversalDropsTheCarriedPixels(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	w.Steps(scroll(0, 40, true, 0, t0), 8, 16) // 40 of 48 carried
	// Back the other way: -20 must not cancel against the carried +40 and
	// must not be late either; it starts over from zero.
	if got := actions(w.Steps(scroll(0, -20, true, 0, t0.Add(10*time.Millisecond)), 8, 16)); got != "" {
		t.Errorf("-20 after +40 gave %q, want none", got)
	}
	if got := actions(w.Steps(scroll(0, -28, true, 0, t0.Add(20*time.Millisecond)), 8, 16)); got != "u" {
		t.Errorf("-48 in all after the reversal gave %q, want one up", got)
	}
}

func TestTouchpadPauseForgetsTheCarriedPixels(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	w.Steps(scroll(0, 40, true, 0, t0), 8, 16)
	if got := actions(w.Steps(scroll(0, 10, true, 0, t0.Add(2*time.Second)), 8, 16)); got != "" {
		t.Errorf("10 DIPs after a pause gave %q; the 40 from long ago must not count", got)
	}
	// Within the gap the carried pixels still count.
	w = &Wheel{GOOS: "darwin"}
	w.Steps(scroll(0, 40, true, 0, t0), 8, 16)
	if got := actions(w.Steps(scroll(0, 10, true, 0, t0.Add(100*time.Millisecond)), 8, 16)); got != "d" {
		t.Errorf("10 DIPs shortly after 40 gave %q, want one", got)
	}
}

func TestTouchpadAxesAccumulateSeparately(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	w.Steps(scroll(30, 30, true, 0, t0), 8, 16)
	got := w.Steps(scroll(18, 0, true, 0, t0.Add(10*time.Millisecond)), 8, 16)
	if actions(got) != "r" {
		t.Errorf("x: 30+18 gave %q, want one right", actions(got))
	}
	got = w.Steps(scroll(0, 18, true, 0, t0.Add(20*time.Millisecond)), 8, 16)
	if actions(got) != "d" {
		t.Errorf("y: 30+18 gave %q, want one down", actions(got))
	}
}

func TestMouseWheelEventClearsTouchpadRemainder(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	w.Steps(scroll(0, 40, true, 0, t0), 8, 16)
	w.Steps(scroll(0, 40, false, 0, t0.Add(time.Millisecond)), 8, 16)
	if got := actions(w.Steps(scroll(0, 10, true, 0, t0.Add(2*time.Millisecond)), 8, 16)); got != "" {
		t.Errorf("remainder survived a mouse wheel notch: %q", got)
	}
}

func TestHugeDeltaIsCapped(t *testing.T) {
	w := &Wheel{GOOS: "darwin"}
	if got := len(w.Steps(scroll(0, 1e6, true, 0, t0), 8, 16)); got != maxStepsPerEvent {
		t.Errorf("a huge touchpad delta gave %d steps, want the cap %d", got, maxStepsPerEvent)
	}
	if got := len(w.Steps(scroll(0, 1e6, false, 0, t0), 8, 16)); got != maxStepsPerEvent {
		t.Errorf("a huge wheel delta gave %d steps, want the cap %d", got, maxStepsPerEvent)
	}
}

func TestModifierStrings(t *testing.T) {
	for _, c := range []struct {
		m    ui.Modifiers
		want string
	}{
		{0, ""}, {ui.Shift, "S"}, {ui.Ctrl, "C"}, {ui.Alt, "A"}, {ui.Super, "D"},
		{ui.Shift | ui.Ctrl, "SC"}, {ui.Ctrl | ui.Alt, "CA"}, {ui.Shift | ui.Ctrl | ui.Alt | ui.Super, "SCAD"},
	} {
		if got := Modifier(c.m); got != c.want {
			t.Errorf("Modifier(%v) = %q, want %q", c.m, got, c.want)
		}
	}
	w := &Wheel{GOOS: "darwin"}
	got := w.Steps(scroll(0, 40, false, ui.Shift|ui.Alt, t0), 8, 16)
	if !reflect.DeepEqual(got, []Step{{"down", "SA"}}) {
		t.Errorf("modifiers not carried onto the step: %v", got)
	}
	got = w.Steps(scroll(0, 48, true, ui.Ctrl, t0), 8, 16)
	if !reflect.DeepEqual(got, []Step{{"down", "C"}}) {
		t.Errorf("modifiers not carried onto a touchpad step: %v", got)
	}
}

func TestCellAt(t *testing.T) {
	for _, c := range []struct {
		x, y     float32
		row, col int
	}{
		{0, 0, 0, 0},
		{7.9, 15.9, 0, 0},
		{8, 16, 1, 1},
		{85, 100, 6, 10},
		{-5, -5, 0, 0},         // above and left of the grid
		{10000, 10000, 23, 79}, // past the end
	} {
		row, col := CellAt(c.x, c.y, 8, 16, 80, 24)
		if row != c.row || col != c.col {
			t.Errorf("CellAt(%v, %v) = row %d col %d, want row %d col %d", c.x, c.y, row, col, c.row, c.col)
		}
	}
	if row, col := CellAt(50, 50, 0, 0, 80, 24); row != 0 || col != 0 {
		t.Errorf("unknown cell size gave %d,%d", row, col)
	}
}
