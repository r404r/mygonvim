package winlayout

import "testing"

func TestSeparatorCells(t *testing.T) {
	s := &Snapshot{Cols: 80, Rows: 24, CmdHeight: 1, GlobalStatus: true, Windows: []Window{
		{Row: 0, Col: 0, Width: 40, Height: 22},
		{Row: 0, Col: 41, Width: 39, Height: 22},
	}}
	for _, c := range []struct {
		row, col int
		want     Cursor
	}{{0, 40, Column}, {21, 40, Column}, {5, 39, Default}, {5, 41, Default},
		{22, 40, Default}, {23, 40, Default}, {-1, 40, Default}, {5, 80, Default}} {
		if got := s.At(c.row, c.col); got != c.want {
			t.Errorf("At(%d,%d)=%v, want %v", c.row, c.col, got, c.want)
		}
	}
	var empty *Snapshot
	if empty.At(0, 0) != Default || (&Snapshot{}).At(0, 0) != Default {
		t.Fatal("missing geometry must use the default pointer")
	}
}

func TestHorizontalSeparatorsAndWinbar(t *testing.T) {
	s := &Snapshot{Cols: 80, Rows: 24, CmdHeight: 1, GlobalStatus: true, Windows: []Window{
		{Row: 1, Col: 0, Width: 80, Height: 8, Winbar: 1},
		{Row: 11, Col: 0, Width: 80, Height: 10, Winbar: 1},
	}}
	for _, c := range []struct {
		row  int
		want Cursor
	}{{0, Default}, {1, Default}, {9, Default}, {10, Row}, {11, Default}, {22, Default}, {23, Default}} {
		if got := s.At(c.row, 5); got != c.want {
			t.Errorf("row %d: got %v, want %v", c.row, got, c.want)
		}
	}
	// With a local statusline, its bottom row is a draggable statusline.
	s.GlobalStatus = false
	if s.At(22, 5) != Row {
		t.Fatal("local bottom statusline lost")
	}
}

func TestCornersAndFloatingOcclusion(t *testing.T) {
	s := &Snapshot{Cols: 80, Rows: 24, CmdHeight: 1, Windows: []Window{
		{Row: 0, Col: 0, Width: 40, Height: 10, Winbar: 1},
	}}
	if s.At(0, 40) != Default || s.At(1, 40) != Column || s.At(11, 40) != Column {
		t.Fatal("unconnected corner or winbar classification")
	}
	s.Windows[0].StatusConnected = true
	if s.At(11, 40) != Row {
		t.Fatal("connected statusline takes priority at corner")
	}
	s.Windows = append(s.Windows, Window{Row: 2, Col: 38, Width: 5, Height: 5, Floating: true})
	for row := 2; row < 7; row++ {
		if s.At(row, 40) != Default {
			t.Fatalf("float body or border at row %d must mask the separator", row)
		}
	}
	if s.At(7, 40) != Column {
		t.Fatal("float mask widened beyond its outer rectangle")
	}
}
