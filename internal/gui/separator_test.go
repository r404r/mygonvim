package gui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/winlayout"
)

func TestHoverCursorAndRepaint(t *testing.T) {
	e := newEnv(t, 640, 400)
	cols, rows := e.ed.Model.Size()
	s := &winlayout.Snapshot{Cols: cols, Rows: rows, CmdHeight: 1, Windows: []winlayout.Window{
		{Width: 20, Height: 10},
	}}
	move := func(col, row int, want ui.Cursor, changed bool) {
		t.Helper()
		x, y := cellCenter(e, col, row)
		ev := ui.InputEvent{Kind: ui.InputPointerMove, Button: -1, X: float32(x), Y: float32(y)}
		if got := e.ed.handle(ev); got != changed {
			t.Fatalf("HandleInput at (%d,%d)=%v, want %v", col, row, got, changed)
		}
		e.tt.Move(float32(x), float32(y))
		e.tt.Frame()
		if got := e.tt.Cursor(); got != want {
			t.Fatalf("cursor at (%d,%d)=%v, want %v", col, row, got, want)
		}
	}
	move(20, 5, ui.CursorDefault, false) // cache not ready
	e.ed.OnWindowLayout(s)
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorResizeColumn {
		t.Fatal("new snapshot did not update the stationary pointer")
	}
	move(20, 6, ui.CursorResizeColumn, false)
	move(19, 6, ui.CursorDefault, true)
	move(21, 6, ui.CursorDefault, false)
	move(5, 10, ui.CursorResizeRow, true)
	e.ed.OnRedraw(ev("mouse_off"), ev("flush", []any{}))
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorDefault {
		t.Fatal("mouse_off did not restore the arrow")
	}
	e.ed.OnRedraw(ev("mouse_on"), ev("flush", []any{}))
	e.ed.OnWindowLayout(nil)
	e.tt.Frame()
	move(20, 5, ui.CursorDefault, false)
	if got := e.mouse(); len(got) != 0 {
		t.Fatalf("hover sent mouse events: %v", got)
	}
}

func TestHoverFloatAndStaleGridUseArrow(t *testing.T) {
	e := newEnv(t, 640, 400)
	cols, rows := e.ed.Model.Size()
	s := &winlayout.Snapshot{Cols: cols, Rows: rows, CmdHeight: 1, Windows: []winlayout.Window{
		{Width: 20, Height: rows - 2},
		{Row: 3, Col: 19, Width: 4, Height: 4, Floating: true},
	}}
	e.ed.OnWindowLayout(s)
	x, y := cellCenter(e, 20, 3)
	e.tt.Move(float32(x), float32(y))
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorDefault {
		t.Fatal("floating border used a resize cursor")
	}
	x, y = cellCenter(e, 20, 7)
	e.tt.Move(float32(x), float32(y))
	if e.tt.Cursor() != ui.CursorResizeColumn {
		t.Fatal("expected resize cursor outside the float")
	}
	stale := *s
	stale.Cols++
	e.ed.OnWindowLayout(&stale)
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorDefault {
		t.Fatal("geometry from another grid size was used")
	}
}

func TestSeparatorCursorSurvivesDragAndStationaryRelease(t *testing.T) {
	e := newEnv(t, 640, 400)
	cols, rows := e.ed.Model.Size()
	s := &winlayout.Snapshot{Cols: cols, Rows: rows, CmdHeight: 1, Windows: []winlayout.Window{
		{Width: 20, Height: rows - 2},
	}}
	e.ed.OnWindowLayout(s)
	x, y := cellCenter(e, 20, 5)
	e.tt.Move(float32(x), float32(y))
	e.tt.Press(float32(x), float32(y))
	x, y = cellCenter(e, 23, 5)
	e.tt.Move(float32(x), float32(y))
	// Until Neovim refreshes the geometry, this position is not a separator.
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorResizeColumn {
		t.Fatal("captured drag lost its press cursor")
	}
	next := *s
	next.Windows = []winlayout.Window{{Width: 23, Height: rows - 2}}
	e.ed.OnWindowLayout(&next)
	e.tt.Release(float32(x), float32(y))
	e.tt.Frame()
	if e.tt.Cursor() != ui.CursorResizeColumn {
		t.Fatal("release at the new separator used the old hover position")
	}
	x, y = cellCenter(e, 22, 5)
	e.tt.Move(float32(x), float32(y))
	if e.tt.Cursor() != ui.CursorDefault {
		t.Fatal("cursor stayed captured after release")
	}
}
