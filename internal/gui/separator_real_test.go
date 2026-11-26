package gui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/winlayout"
)

func separatorHover(r *realWheel, row, col int, want ui.Cursor) {
	r.t.Helper()
	x, y := r.at(col, row)
	r.tt.Move(x, y)
	r.tt.Frame()
	if got := r.tt.Cursor(); got != want {
		r.t.Fatalf("cursor at (%d,%d)=%v, want %v", row, col, got, want)
	}
}

func separatorDrag(r *realWheel, row, col, drow, dcol int) {
	r.t.Helper()
	x, y := r.at(col, row)
	r.tt.Press(x, y)
	pointerSync(r)
	x, y = r.at(col+dcol, row+drow)
	r.tt.Move(x, y)
	pointerSync(r)
	r.tt.Release(x, y)
	pointerSync(r)
}

func TestRealNvimVerticalSeparatorCursorMatchesDrag(t *testing.T) {
	r := startRealPointer(t, "-c", "set laststatus=3 | vsplit")
	left := r.eval("win_getid(1)")
	sep := r.eval("winwidth(1)")
	r.wait("vertical geometry", func() bool { return r.ed.layout.Load().At(2, sep) == winlayout.Column })
	if err := r.sess.Nvim().Command(`let g:hover_moves = 0 | set mousemoveevent | nnoremap <MouseMove> <Cmd>let g:hover_moves += 1<CR>`); err != nil {
		t.Fatal(err)
	}
	separatorHover(r, 2, sep, ui.CursorResizeColumn)
	separatorHover(r, 2, sep-1, ui.CursorDefault)
	separatorHover(r, 2, sep+1, ui.CursorDefault)
	pointerSync(r)
	if r.eval("g:hover_moves") != 0 {
		t.Fatal("hover was forwarded as MouseMove")
	}
	separatorHover(r, 2, sep, ui.CursorResizeColumn)
	separatorDrag(r, 2, sep, 0, 3)
	if got := r.eval(fmt.Sprintf("nvim_win_get_width(%d)", left)); got != sep+3 {
		t.Fatalf("dragged width=%d, want %d", got, sep+3)
	}
	r.wait("geometry after drag", func() bool {
		s := r.ed.layout.Load()
		return s.At(2, sep) == winlayout.Default && s.At(2, sep+3) == winlayout.Column
	})
	separatorHover(r, 2, sep+3, ui.CursorResizeColumn)
	if err := r.sess.Nvim().Command("only"); err != nil {
		t.Fatal(err)
	}
	r.wait("geometry after close", func() bool { return r.ed.layout.Load().At(2, sep+3) == winlayout.Default })
	r.tt.Frame()
	if r.tt.Cursor() != ui.CursorDefault {
		t.Fatal("closed separator retained its cursor")
	}
}

func TestRealNvimGlobalStatusHorizontalCursorMatchesDrag(t *testing.T) {
	r := startRealPointer(t, "-c", "set laststatus=3 | split")
	top := r.eval("win_getid(1)")
	height := r.eval("winheight(1)")
	row := r.eval("win_screenpos(2)[0]-2")
	r.wait("horizontal geometry", func() bool { return r.ed.layout.Load().At(row, 4) == winlayout.Row })
	separatorHover(r, row, 4, ui.CursorResizeRow)
	separatorHover(r, row-1, 4, ui.CursorDefault)
	separatorHover(r, row+1, 4, ui.CursorDefault)
	separatorDrag(r, row, 4, 2, 0)
	if got := r.eval(fmt.Sprintf("nvim_win_get_height(%d)", top)); got != height+2 {
		t.Fatalf("dragged height=%d, want %d", got, height+2)
	}
	global := r.eval("&lines-&cmdheight-1")
	separatorHover(r, global, 4, ui.CursorDefault)
	heights := r.eval("winheight(1)*1000+winheight(2)")
	// With cmdheight=1, dragging downward cannot change window heights.
	separatorDrag(r, global, 4, 2, 0)
	if got := r.eval("winheight(1)*1000+winheight(2)"); got != heights {
		t.Fatalf("global statusline changed window heights: %d -> %d", heights, got)
	}
}

func TestRealNvimSeparatorIgnoresTextFloatAndMouseOff(t *testing.T) {
	r := startRealPointer(t, "-c", "set laststatus=3 | vsplit")
	sep := r.eval("winwidth(1)")
	if err := r.sess.Nvim().ExecLua(`vim.api.nvim_buf_set_lines(0,0,1,false,{'│─ text'})`, nil); err != nil {
		t.Fatal(err)
	}
	r.wait("separator characters in buffer text", func() bool { return strings.HasPrefix(r.row(0), "│─") })
	r.wait("separator geometry", func() bool { return r.ed.layout.Load().At(2, sep) == winlayout.Column })
	separatorHover(r, 0, 0, ui.CursorDefault)
	separatorHover(r, 0, 1, ui.CursorDefault)
	if err := r.sess.Nvim().ExecLua(`vim.api.nvim_open_win(vim.api.nvim_create_buf(false,true),false,
	  {relative='editor',row=2,col=...,width=6,height=3,border='single'})`, nil, sep); err != nil {
		t.Fatal(err)
	}
	r.wait("floating border geometry", func() bool {
		s := r.ed.layout.Load()
		return len(s.Windows) == 3 && s.At(2, sep) == winlayout.Default
	})
	separatorHover(r, 2, sep, ui.CursorDefault)
	separatorHover(r, 3, sep, ui.CursorDefault)
	separatorHover(r, 7, sep, ui.CursorResizeColumn)
	if err := r.sess.Nvim().Command("set mouse="); err != nil {
		t.Fatal(err)
	}
	r.wait("mouse disabled", func() bool { return r.ed.pointerCursor() == ui.CursorDefault })
	r.tt.Frame()
	if r.tt.Cursor() != ui.CursorDefault {
		t.Fatal("mouse disabled but resize cursor still visible")
	}
}

func TestRealNvimSeparatorWinbarAndCorners(t *testing.T) {
	for _, c := range []struct {
		name, command string
		cursor        ui.Cursor
		kind          winlayout.Cursor
		drow, dcol    int
	}{
		{"unconnected", "vsplit | split", ui.CursorResizeColumn, winlayout.Column, 0, 2},
		{"connected", "split | vsplit", ui.CursorResizeRow, winlayout.Row, 2, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := startRealPointer(t)
			if err := r.sess.Nvim().Command("set laststatus=2 winbar=bar | " + c.command); err != nil {
				t.Fatal(err)
			}
			id := r.eval("win_getid(1)")
			col := r.eval("winwidth(1)")
			row := r.eval("getwininfo(win_getid(1))[0].height + getwininfo(win_getid(1))[0].winbar")
			height := r.eval(fmt.Sprintf("nvim_win_get_height(%d)", id))
			r.wait("corner geometry", func() bool { return r.ed.layout.Load().At(row, col) == c.kind })
			separatorHover(r, 0, col, ui.CursorDefault) // winbar is not a separator
			separatorHover(r, 1, col, ui.CursorResizeColumn)
			separatorHover(r, row, col, c.cursor)
			separatorDrag(r, row, col, c.drow, c.dcol)
			if got := r.eval(fmt.Sprintf("nvim_win_get_width(%d)", id)); got != col+c.dcol {
				t.Fatalf("corner drag width=%d, want %d", got, col+c.dcol)
			}
			if got := r.eval(fmt.Sprintf("nvim_win_get_height(%d)", id)); got != height+c.drow {
				t.Fatalf("corner drag height=%d, want %d", got, height+c.drow)
			}
		})
	}
}
