package gui

import (
	"math"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	xwidth "golang.org/x/text/width"

	"github.com/r404r/mygonvim/internal/grid"
)

func rgb(n int) ui.Color {
	return ui.Color{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}

// measure sets the cell size for scale: the advance of a wide letter and the
// line height, rounded to whole device pixels so that rows and columns of cells
// meet without seams.
func (e *Editor) measure(scale float32) {
	if scale <= 0 {
		scale = 1
	}
	if e.metricsFor == scale {
		return
	}
	f := e.fonts[0]
	m := f.Metrics()
	adv := f.Size * 0.6
	if g := ui.Shape("0", f); len(g) > 0 && g[0].Advance > 0 {
		adv = g[0].Advance
	}
	px := func(v float32) float32 { return max(1, float32(math.Round(float64(v*scale)))) / scale }
	e.cw = px(adv)
	e.asc, e.desc = m.Ascent, m.Descent
	e.ch = px(m.Ascent + m.Descent + m.LineGap)
	e.base = float32(math.Round(float64((m.Ascent+(e.ch-m.Ascent-m.Descent)/2)*scale))) / scale
	e.metricsFor = scale
	clear(e.shaped) // shaping does not depend on scale, but advances were measured with it
}

// glyphsOf shapes text in the font of style, with a cache: a screen redraws the
// same words, and shaping is the costly part.
func (e *Editor) glyphsOf(text string, style uint8) []ui.Glyph {
	k := shapeKey{text, style}
	if g, ok := e.shaped[k]; ok {
		return g
	}
	if len(e.shaped) > 8192 {
		clear(e.shaped)
	}
	g := ui.Shape(text, e.fonts[style])
	e.shaped[k] = g
	return g
}

// draw paints the whole grid into r. MyGo may call it more than once a frame,
// so the only effect besides painting is the idempotent resize request.
func (e *Editor) draw(p *ui.Painter, r ui.Rect) {
	e.applyPendingFont()
	e.measure(p.Scale())
	cols, rows := max(1, int(r.W/e.cw)), max(1, int(r.H/e.ch))
	if cols != e.reqCols || rows != e.reqRows {
		e.reqCols, e.reqRows = cols, rows
		e.be.Resize(cols, rows)
	}
	e.Model.Read(func(s *grid.State) { e.paint(p, r, s) })
}

func (e *Editor) paint(p *ui.Painter, r ui.Rect, s *grid.State) {
	p.Fill(r, rgb(s.DefBg), 0)
	for y := 0; y < s.Rows; y++ {
		top := r.Y + float32(y)*e.ch
		if top > r.Y+r.H {
			break
		}
		row := s.Row(y)
		for x := 0; x < s.Cols; {
			hl := row[x].Hl
			x1 := x + 1
			for x1 < s.Cols && row[x1].Hl == hl {
				x1++
			}
			e.paintRun(p, r, s, row, y, x, x1, top)
			x = x1
		}
	}
	e.paintCursor(p, r, s)
	e.paintPreedit(p, r, s)
}

// styleOf is the index into Editor.fonts for an attribute.
func styleOf(a grid.Attr) uint8 {
	var st uint8
	if a.Has(grid.Bold) {
		st |= 1
	}
	if a.Has(grid.Italic) {
		st |= 2
	}
	return st
}

// paintRun draws cells [x0,x1) of a row, which share one highlight: the
// background, the text, then underline or strike-through.
func (e *Editor) paintRun(p *ui.Painter, r ui.Rect, s *grid.State, row []grid.Cell, y, x0, x1 int, top float32) {
	fg, bg, sp, a := s.Colors(row[x0].Hl)
	left := r.X + float32(x0)*e.cw
	width := float32(x1-x0) * e.cw
	if bg != s.DefBg {
		p.Fill(ui.Rect{X: left, Y: top, W: width, H: e.ch}, rgb(bg), 0)
	}
	e.txt, e.cols = e.txt[:0], e.cols[:0]
	for c := x0; c < x1; c++ {
		t := row[c].Text
		if t == "" || t == " " {
			continue
		}
		for _, rn := range t {
			e.txt = utf8.AppendRune(e.txt, rn)
			e.cols = append(e.cols, c-x0)
		}
	}
	if len(e.txt) > 0 {
		e.drawText(p, string(e.txt), e.cols, styleOf(a), left, top+e.base, rgb(fg))
	}
	if a.Flags&(grid.Underline|grid.Undercurl|grid.Underdouble|grid.Underdotted|grid.Underdashed|grid.Strikethrough) == 0 {
		return
	}
	px := 1 / max(p.Scale(), 1)
	lineW := max(px, float32(math.Round(float64(e.fonts[0].Size/14*p.Scale())))*px)
	uy := top + e.base + max(px*2, e.desc*0.4)
	switch {
	case a.Has(grid.Undercurl):
		var path ui.Path
		amp, period := max(e.ch*0.06, px*1.5), e.cw*0.5
		path.MoveTo(left, uy)
		for i, x := 1, left+period/2; x <= left+width+period/2; i, x = i+1, x+period/2 {
			dy := amp
			if i%2 == 1 {
				dy = -amp
			}
			path.LineTo(x, uy+dy)
		}
		p.Clip(ui.Rect{X: left, Y: top, W: width, H: e.ch}, 0, func() { p.StrokePath(&path, lineW, rgb(sp)) })
	case a.Has(grid.Underdouble):
		p.Line(left, uy-px*1.5, left+width, uy-px*1.5, lineW, rgb(sp))
		p.Line(left, uy+px*1.5, left+width, uy+px*1.5, lineW, rgb(sp))
	case a.Has(grid.Underdotted), a.Has(grid.Underdashed):
		var path ui.Path
		path.MoveTo(left, uy).LineTo(left+width, uy)
		dash, gap := e.cw*0.25, e.cw*0.25
		if a.Has(grid.Underdashed) {
			dash, gap = e.cw*0.6, e.cw*0.3
		}
		p.StrokePath(path.Dash(0, dash, gap), lineW, rgb(sp))
	case a.Has(grid.Underline):
		p.Line(left, uy, left+width, uy, lineW, rgb(sp))
	}
	if a.Has(grid.Strikethrough) {
		sy := top + e.base - e.asc*0.3
		p.Line(left, sy, left+width, sy, lineW, rgb(fg))
	}
}

// drawText shapes text and draws each glyph in the cell of its first rune:
// colsOf[i] is the column, from x, of rune i. The cells fix the positions, not
// the font's advances, so wide characters and fallback fonts keep the grid.
func (e *Editor) drawText(p *ui.Painter, text string, colsOf []int, style uint8, x, baseline float32, col ui.Color) {
	gl := e.glyphsOf(text, style)
	e.scratch = append(e.scratch[:0], gl...)
	for i := range e.scratch {
		c := e.scratch[i].Cluster
		if c >= 0 && c < len(colsOf) {
			e.scratch[i].X = float32(colsOf[c]) * e.cw
		}
	}
	p.Glyphs(e.scratch, x, baseline, col)
}

// paintCursor draws the cursor of the current mode: a block (the cell's text
// redrawn in the opposite colors), a bar at the left, or a bar at the bottom. A
// window without the keyboard gets an outline instead.
func (e *Editor) paintCursor(p *ui.Painter, r ui.Rect, s *grid.State) {
	if s.Busy || s.CurRow < 0 || s.CurRow >= s.Rows || s.CurCol < 0 || s.CurCol >= s.Cols {
		return
	}
	cell := s.At(s.CurRow, s.CurCol)
	cells := 1
	if s.CurCol+1 < s.Cols && s.At(s.CurRow, s.CurCol+1).Text == "" && cell.Text != "" {
		cells = 2
	}
	box := ui.Rect{X: r.X + float32(s.CurCol)*e.cw, Y: r.Y + float32(s.CurRow)*e.ch, W: float32(cells) * e.cw, H: e.ch}
	fg, bg := s.CursorColors()
	if !e.focused {
		px := 1 / max(p.Scale(), 1)
		p.Stroke(box, rgb(bg), 0, px)
		return
	}
	mi := s.ModeCursor()
	switch mi.Shape {
	case grid.CursorVertical:
		w := max(1/max(p.Scale(), 1), e.cw*float32(mi.CellPercentage)/100)
		p.Fill(ui.Rect{X: box.X, Y: box.Y, W: w, H: box.H}, rgb(bg), 0)
	case grid.CursorHorizontal:
		h := max(1/max(p.Scale(), 1), e.ch*float32(mi.CellPercentage)/100)
		p.Fill(ui.Rect{X: box.X, Y: box.Y + box.H - h, W: box.W, H: h}, rgb(bg), 0)
	default:
		p.Fill(box, rgb(bg), 0)
		if cell.Text != "" && cell.Text != " " {
			_, _, _, a := s.Colors(cell.Hl)
			e.drawText(p, cell.Text, repeatInt(0, utf8.RuneCountInString(cell.Text)), styleOf(a), box.X, box.Y+e.base, rgb(fg))
		}
	}
}

func repeatInt(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// runeCells is the number of grid cells a character takes: 2 for East Asian
// wide and fullwidth characters, 1 otherwise.
func runeCells(r rune) int {
	switch xwidth.LookupRune(r).Kind() {
	case xwidth.EastAsianWide, xwidth.EastAsianFullwidth:
		return 2
	}
	if r >= 0x1F300 && r <= 0x1FAFF { // emoji: two cells in Neovim
		return 2
	}
	return 1
}

// cellsBefore is the width in cells of the first n runes of s.
func cellsBefore(s string, n int) int {
	w, i := 0, 0
	for _, r := range s {
		if i >= n {
			break
		}
		w += runeCells(r)
		i++
	}
	return w
}

// paintPreedit draws the input method's composition at the cursor, underlined,
// over the grid: the text is not Neovim's until the method commits it.
func (e *Editor) paintPreedit(p *ui.Painter, r ui.Rect, s *grid.State) {
	pre := e.tr.Preedit
	if pre == "" {
		return
	}
	x0 := r.X + float32(s.CurCol)*e.cw
	top := r.Y + float32(s.CurRow)*e.ch
	total := cellsBefore(pre, utf8.RuneCountInString(pre))
	fg, bg, _, _ := s.Colors(0)
	p.Fill(ui.Rect{X: x0, Y: top, W: float32(total) * e.cw, H: e.ch}, rgb(bg), 0)
	var cols []int
	w := 0
	for _, rn := range pre {
		cols = append(cols, w)
		w += runeCells(rn)
	}
	e.drawText(p, pre, cols, 0, x0, top+e.base, rgb(fg))
	px := 1 / max(p.Scale(), 1)
	uy := top + e.base + max(px*2, e.desc*0.4)
	p.Line(x0, uy, x0+float32(total)*e.cw, uy, px, rgb(fg))
	cx := x0 + float32(cellsBefore(pre, e.tr.PreeditCaret))*e.cw
	p.Fill(ui.Rect{X: cx, Y: top, W: px, H: e.ch}, rgb(fg), 0)
}
