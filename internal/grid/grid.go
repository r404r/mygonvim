// Package grid is the in-memory screen of an embedded Neovim: the cell grid,
// the highlight table, the default colors, the cursor and the mode, kept up to
// date from the "redraw" notifications of the ext_linegrid UI protocol.
//
// It knows nothing about windows or drawing, so it is tested on its own.
package grid

import "sync"

// Cell is one cell of the grid. Text is the grapheme cluster shown in it. A
// cell right of a double-width character has empty Text: Neovim sends "" there,
// and the renderer skips it.
type Cell struct {
	Text string
	Hl   int
}

// AttrFlags are the boolean attributes of a highlight.
type AttrFlags uint16

// Attribute flags, from the keys of hl_attr_define's rgb_attr map.
const (
	Reverse AttrFlags = 1 << iota
	Bold
	Italic
	Strikethrough
	Underline
	Undercurl
	Underdouble
	Underdotted
	Underdashed
)

// Unset marks a color a highlight does not set.
const Unset = -1

// Attr is a highlight: colors as 0xRRGGBB, or Unset, and flags.
type Attr struct {
	Fg, Bg, Sp int
	Flags      AttrFlags
	// Blend is the 0-100 blend of a floating window; kept, not drawn yet.
	Blend int
}

// Has reports whether every flag in f is set.
func (a Attr) Has(f AttrFlags) bool { return a.Flags&f == f }

// CursorShape is the shape of the cursor in a mode.
type CursorShape int

// Cursor shapes, from mode_info_set's cursor_shape.
const (
	CursorBlock CursorShape = iota
	CursorVertical
	CursorHorizontal
)

// ModeInfo describes the cursor in one mode (mode_info_set).
type ModeInfo struct {
	Name string
	// Shape and CellPercentage: the share of the cell a vertical or
	// horizontal cursor covers, 1-100.
	Shape          CursorShape
	CellPercentage int
	// AttrID is the highlight of the cursor; 0 means the cell's own colors
	// reversed.
	AttrID int
}

// State is the screen. Read it inside Model.Read, which holds the lock the
// redraw goroutine's Apply takes.
type State struct {
	Rows, Cols int
	cells      []Cell
	Attrs      map[int]Attr

	DefFg, DefBg, DefSp int

	CurRow, CurCol int
	Mode           string
	ModeIdx        int
	Modes          []ModeInfo
	// CursorStyleEnabled is mode_info_set's first argument: when false the
	// cursor is always a block.
	CursorStyleEnabled bool

	Title string
	Busy  bool
	// MouseOff is true while Neovim says its 'mouse' option is off for the
	// current mode (mouse_off); mouse_on ends it. A new model has the mouse on.
	MouseOff bool
	// GuiFont is the value of the guifont option, "" until Neovim sets it.
	GuiFont string
	// Background is the option, distinct from cell highlight colors.
	Background        string
	BackgroundVersion uint64
	// Flushes counts "flush" events, which end a batch of updates.
	Flushes uint64
}

// Row returns row r of the grid, or nil outside it. The slice aliases the
// grid; do not keep it past Model.Read.
func (s *State) Row(r int) []Cell {
	if r < 0 || r >= s.Rows {
		return nil
	}
	return s.cells[r*s.Cols : (r+1)*s.Cols]
}

// At returns the cell at row r, column c; a blank cell outside the grid.
func (s *State) At(r, c int) Cell {
	if r < 0 || r >= s.Rows || c < 0 || c >= s.Cols {
		return Cell{Text: " "}
	}
	return s.cells[r*s.Cols+c]
}

// RowText returns the text of row r as a string, with the placeholder cells of
// double-width characters left out.
func (s *State) RowText(r int) string {
	var out []byte
	for _, c := range s.Row(r) {
		out = append(out, c.Text...)
	}
	return string(out)
}

// AttrOf returns highlight id; the zero Attr (unset colors) for 0 and unknown ids.
func (s *State) AttrOf(id int) Attr {
	if a, ok := s.Attrs[id]; ok {
		return a
	}
	return Attr{Fg: Unset, Bg: Unset, Sp: Unset}
}

// Colors resolves a highlight to the foreground, background and special colors
// it draws with: unset colors take the defaults, and Reverse swaps foreground
// and background.
func (s *State) Colors(id int) (fg, bg, sp int, a Attr) {
	a = s.AttrOf(id)
	fg, bg, sp = a.Fg, a.Bg, a.Sp
	if fg == Unset {
		fg = s.DefFg
	}
	if bg == Unset {
		bg = s.DefBg
	}
	if sp == Unset {
		sp = fg
	}
	if a.Has(Reverse) {
		fg, bg = bg, fg
	}
	return fg, bg, sp, a
}

// ModeCursor returns the cursor description of the current mode; a block when
// the mode is unknown or cursor styling is off.
func (s *State) ModeCursor() ModeInfo {
	if s.CursorStyleEnabled && s.ModeIdx >= 0 && s.ModeIdx < len(s.Modes) {
		return s.Modes[s.ModeIdx]
	}
	return ModeInfo{Name: s.Mode, Shape: CursorBlock, CellPercentage: 100}
}

// CursorColors returns the colors the cursor cell is drawn with: the text color
// and the block color. A cursor highlight of the mode wins; otherwise the cell's
// own colors are swapped.
func (s *State) CursorColors() (fg, bg int) {
	mi := s.ModeCursor()
	cell := s.At(s.CurRow, s.CurCol)
	if mi.AttrID != 0 {
		fg, bg, _, _ = s.Colors(mi.AttrID)
		return fg, bg
	}
	cfg, cbg, _, _ := s.Colors(cell.Hl)
	return cbg, cfg
}

// Model is a State kept up to date by Apply and read by the UI thread.
type Model struct {
	mu sync.Mutex
	s  State
}

// New returns a model of a blank cols x rows grid.
func New(cols, rows int) *Model {
	m := &Model{}
	m.s.Attrs = map[int]Attr{}
	m.s.DefFg, m.s.DefBg, m.s.DefSp = 0xffffff, 0x000000, 0xff0000
	m.s.CursorStyleEnabled = true
	m.s.Mode = "normal"
	m.s.resize(cols, rows)
	return m
}

// Read calls fn with the state, under the model's lock.
func (m *Model) Read(fn func(s *State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.s)
}

// Size returns the grid's columns and rows.
func (m *Model) Size() (cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.s.Cols, m.s.Rows
}

func blank() Cell { return Cell{Text: " "} }

func (s *State) resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	next := make([]Cell, cols*rows)
	for i := range next {
		next[i] = blank()
	}
	// Keep what overlaps, so a half-sent redraw shows no hole.
	for r := 0; r < min(rows, s.Rows); r++ {
		copy(next[r*cols:r*cols+min(cols, s.Cols)], s.cells[r*s.Cols:r*s.Cols+min(cols, s.Cols)])
	}
	s.cells, s.Cols, s.Rows = next, cols, rows
	s.CurRow = min(s.CurRow, rows-1)
	s.CurCol = min(s.CurCol, cols-1)
}

func (s *State) clear() {
	for i := range s.cells {
		s.cells[i] = blank()
	}
}
