package grid

import (
	"strings"
	"testing"
)

// ev builds one update of a redraw notification: [name, args...].
func ev(name string, args ...[]any) []any {
	u := []any{name}
	for _, a := range args {
		u = append(u, a)
	}
	return u
}

func line(row, col int, cells ...any) []any {
	return []any{int64(1), int64(row), int64(col), cells, false}
}

func cell(text string, rest ...any) []any { return append([]any{text}, rest...) }

func rowText(m *Model, r int) string {
	var s string
	m.Read(func(st *State) { s = st.RowText(r) })
	return s
}

func TestGridLineRepeatAndHighlightCarry(t *testing.T) {
	m := New(10, 2)
	m.Apply(ev("grid_line", line(0, 1,
		cell("a", int64(3)),           // hl 3
		cell("b"),                     // carries hl 3
		cell("-", int64(0), int64(3)), // hl 0, repeated 3 times
		cell("z", int64(5)),
	)))
	m.Read(func(s *State) {
		want := []Cell{{" ", 0}, {"a", 3}, {"b", 3}, {"-", 0}, {"-", 0}, {"-", 0}, {"z", 5}}
		for i, w := range want {
			if got := s.At(0, i); got != w {
				t.Errorf("cell %d = %+v, want %+v", i, got, w)
			}
		}
	})
}

func TestWideCharPlaceholderIsSkippedInRowText(t *testing.T) {
	m := New(8, 1)
	m.Apply(ev("grid_line", line(0, 0, cell("中", int64(0)), cell(""), cell("x"), cell("😀"), cell(""))))
	if got := rowText(m, 0); !strings.HasPrefix(got, "中x😀") {
		t.Errorf("row text %q", got)
	}
	m.Read(func(s *State) {
		if s.At(0, 1).Text != "" || s.At(0, 4).Text != "" {
			t.Errorf("placeholders lost: %+v %+v", s.At(0, 1), s.At(0, 4))
		}
	})
}

func TestGridLineClipsToTheGrid(t *testing.T) {
	m := New(4, 1)
	m.Apply(ev("grid_line", line(0, 2, cell("a", int64(0), int64(10)))))
	m.Apply(ev("grid_line", line(5, 0, cell("q")))) // row out of range
	m.Apply(ev("grid_line", line(0, -1, cell("q"))))
	if got := rowText(m, 0); got != "  aa" {
		t.Errorf("row %q", got)
	}
}

func TestScrollUpAndDown(t *testing.T) {
	m := New(3, 4)
	for r, s := range []string{"aaa", "bbb", "ccc", "ddd"} {
		var cs []any
		for _, ch := range s {
			cs = append(cs, cell(string(ch)))
		}
		m.Apply(ev("grid_line", line(r, 0, cs...)))
	}
	// Scroll the whole grid up by one: bbb ccc ddd ddd.
	m.Apply(ev("grid_scroll", []any{int64(1), int64(0), int64(4), int64(0), int64(3), int64(1), int64(0)}))
	var got []string
	for r := 0; r < 4; r++ {
		got = append(got, rowText(m, r))
	}
	if strings.Join(got, ",") != "bbb,ccc,ddd,ddd" {
		t.Errorf("after scroll up: %v", got)
	}
	// Scroll rows 1..3 down by one: bbb bbb ccc ddd.
	m.Apply(ev("grid_scroll", []any{int64(1), int64(1), int64(4), int64(0), int64(3), int64(-1), int64(0)}))
	got = nil
	for r := 0; r < 4; r++ {
		got = append(got, rowText(m, r))
	}
	if strings.Join(got, ",") != "bbb,ccc,ccc,ddd" {
		t.Errorf("after scroll down: %v", got)
	}
}

func TestScrollRegionOnlyMovesItsColumns(t *testing.T) {
	m := New(4, 2)
	m.Apply(ev("grid_line", line(0, 0, cell("a"), cell("b"), cell("c"), cell("d"))))
	m.Apply(ev("grid_line", line(1, 0, cell("e"), cell("f"), cell("g"), cell("h"))))
	m.Apply(ev("grid_scroll", []any{int64(1), int64(0), int64(2), int64(1), int64(3), int64(1), int64(0)}))
	if got := rowText(m, 0); got != "afgd" {
		t.Errorf("row 0 %q, want afgd", got)
	}
}

func TestClearAndResize(t *testing.T) {
	m := New(3, 2)
	m.Apply(ev("grid_line", line(0, 0, cell("x"), cell("y"), cell("z"))))
	r := m.Apply(ev("grid_resize", []any{int64(1), int64(5), int64(3)}))
	if !r.Resized {
		t.Error("resize not reported")
	}
	if c, rws := m.Size(); c != 5 || rws != 3 {
		t.Fatalf("size %dx%d", c, rws)
	}
	if got := rowText(m, 0); got != "xyz  " {
		t.Errorf("overlap not kept: %q", got)
	}
	m.Apply(ev("grid_clear", []any{int64(1)}))
	if got := rowText(m, 0); got != "     " {
		t.Errorf("not cleared: %q", got)
	}
}

func TestOtherGridsAreIgnored(t *testing.T) {
	m := New(3, 1)
	m.Apply(ev("grid_line", []any{int64(2), int64(0), int64(0), []any{cell("x")}, false}))
	m.Apply(ev("grid_resize", []any{int64(2), int64(9), int64(9)}))
	m.Apply(ev("grid_cursor_goto", []any{int64(2), int64(0), int64(2)}))
	if got := rowText(m, 0); got != "   " {
		t.Errorf("grid 2 leaked: %q", got)
	}
	m.Read(func(s *State) {
		if s.Cols != 3 || s.CurCol != 0 {
			t.Errorf("state changed: %dx%d cursor %d", s.Cols, s.Rows, s.CurCol)
		}
	})
}

func TestHighlightsAndDefaultColors(t *testing.T) {
	m := New(2, 1)
	m.Apply(
		ev("default_colors_set", []any{int64(0xd0d0d0), int64(0x101010), int64(0xff0000), int64(7), int64(0)}),
		ev("hl_attr_define",
			[]any{int64(7), map[string]any{"foreground": int64(0x112233), "bold": true, "italic": true}, map[string]any{}, []any{}},
			[]any{int64(8), map[string]any{"reverse": true, "undercurl": true, "special": uint64(0x00ff00)}, map[string]any{}, []any{}},
			[]any{int64(9), map[string]any{"background": int64(0x445566)}, map[string]any{}, []any{}},
		),
	)
	m.Read(func(s *State) {
		fg, bg, _, a := s.Colors(7)
		if fg != 0x112233 || bg != 0x101010 || !a.Has(Bold|Italic) || a.Has(Underline) {
			t.Errorf("hl 7: fg %x bg %x flags %b", fg, bg, a.Flags)
		}
		fg, bg, sp, a := s.Colors(8)
		if fg != 0x101010 || bg != 0xd0d0d0 || sp != 0x00ff00 || !a.Has(Undercurl) {
			t.Errorf("hl 8 (reverse): fg %x bg %x sp %x", fg, bg, sp)
		}
		fg, bg, _, _ = s.Colors(9)
		if fg != 0xd0d0d0 || bg != 0x445566 {
			t.Errorf("hl 9: fg %x bg %x", fg, bg)
		}
		fg, bg, _, _ = s.Colors(0)
		if fg != 0xd0d0d0 || bg != 0x101010 {
			t.Errorf("hl 0 should be the defaults: %x %x", fg, bg)
		}
	})
}

func TestUnsetDefaultColorsKeepUsableOnes(t *testing.T) {
	m := New(1, 1)
	m.Apply(ev("default_colors_set", []any{int64(-1), int64(-1), int64(-1), int64(0), int64(0)}))
	m.Read(func(s *State) {
		if s.DefFg != 0xffffff || s.DefBg != 0 {
			t.Errorf("defaults %x %x", s.DefFg, s.DefBg)
		}
	})
}

func TestModesCursorAndTitle(t *testing.T) {
	m := New(10, 3)
	m.Apply(ev("mode_info_set", []any{true, []any{
		map[string]any{"name": "normal", "cursor_shape": "block", "cell_percentage": int64(100)},
		map[string]any{"name": "insert", "cursor_shape": "vertical", "cell_percentage": int64(25), "attr_id": int64(4)},
		map[string]any{"name": "replace", "cursor_shape": "horizontal", "cell_percentage": int64(20)},
	}}))
	r := m.Apply(ev("mode_change", []any{"insert", int64(1)}), ev("grid_cursor_goto", []any{int64(1), int64(2), int64(7)}), ev("set_title", []any{"a.txt - NVIM"}))
	if !r.ModeChanged || !r.TitleChanged {
		t.Errorf("result %+v", r)
	}
	m.Read(func(s *State) {
		mi := s.ModeCursor()
		if s.Mode != "insert" || mi.Shape != CursorVertical || mi.CellPercentage != 25 || mi.AttrID != 4 {
			t.Errorf("mode %q cursor %+v", s.Mode, mi)
		}
		if s.CurRow != 2 || s.CurCol != 7 || s.Title != "a.txt - NVIM" {
			t.Errorf("cursor %d,%d title %q", s.CurRow, s.CurCol, s.Title)
		}
	})
	// The same title again is no change.
	if r := m.Apply(ev("set_title", []any{"a.txt - NVIM"})); r.TitleChanged {
		t.Error("unchanged title reported")
	}
	// With cursor styling off every mode is a block.
	m.Apply(ev("mode_info_set", []any{false, []any{map[string]any{"name": "insert", "cursor_shape": "vertical"}}}))
	m.Read(func(s *State) {
		if s.ModeCursor().Shape != CursorBlock {
			t.Error("cursor style disabled but shape is not a block")
		}
	})
}

func TestCursorColors(t *testing.T) {
	m := New(2, 1)
	m.Apply(
		ev("default_colors_set", []any{int64(0xeeeeee), int64(0x111111), int64(-1)}),
		ev("hl_attr_define", []any{int64(2), map[string]any{"foreground": int64(0xaa0000), "background": int64(0x00aa00)}, map[string]any{}, []any{}}),
		ev("hl_attr_define", []any{int64(6), map[string]any{"foreground": int64(0x000001), "background": int64(0xfefefe)}, map[string]any{}, []any{}}),
		ev("mode_info_set", []any{true, []any{
			map[string]any{"name": "normal", "cursor_shape": "block"},
			map[string]any{"name": "visual", "cursor_shape": "block", "attr_id": int64(6)},
		}}),
		ev("grid_line", line(0, 0, cell("a", int64(2)))),
		ev("grid_cursor_goto", []any{int64(1), int64(0), int64(0)}),
	)
	m.Read(func(s *State) {
		fg, bg := s.CursorColors() // attr_id 0: the cell reversed
		if fg != 0x00aa00 || bg != 0xaa0000 {
			t.Errorf("reversed cell cursor: fg %x bg %x", fg, bg)
		}
	})
	m.Apply(ev("mode_change", []any{"visual", int64(1)}))
	m.Read(func(s *State) {
		fg, bg := s.CursorColors()
		if fg != 0x000001 || bg != 0xfefefe {
			t.Errorf("attr_id cursor: fg %x bg %x", fg, bg)
		}
	})
}

func TestMalformedUpdatesDoNotPanic(t *testing.T) {
	m := New(2, 2)
	bad := [][]any{
		{}, {"grid_line"}, {"grid_line", "oops"}, {"grid_line", []any{}}, {"grid_line", []any{int64(1)}},
		{"grid_line", []any{int64(1), "x", nil, 5}}, {"grid_scroll", []any{int64(1)}},
		{"hl_attr_define", []any{int64(1), "notamap"}}, {"mode_info_set", []any{true, 3}},
		{"mode_change", []any{}}, {"default_colors_set", []any{nil, nil, nil}}, {"flush"}, {"unknown_event", []any{1}},
		{"grid_line", []any{int64(1), int64(0), int64(0), []any{[]any{}, 5, []any{1, 2, 3}}}},
	}
	for _, u := range bad {
		m.Apply(u)
	}
	if r := m.Apply([]any{"flush"}); !r.Flushed {
		t.Error("bare flush not seen")
	}
	if r := m.Apply(ev("flush", []any{})); !r.Flushed {
		t.Error("flush with empty args not seen")
	}
	m.Read(func(s *State) {
		if s.Flushes != 3 {
			t.Errorf("flushes %d, want 3", s.Flushes)
		}
	})
}

func BenchmarkFullScreenRedraw(b *testing.B) {
	m := New(200, 50)
	var cs []any
	for i := 0; i < 200; i++ {
		cs = append(cs, cell("x", int64(i%4)))
	}
	var batch [][]any
	for r := 0; r < 50; r++ {
		batch = append(batch, ev("grid_line", line(r, 0, cs...)))
	}
	batch = append(batch, ev("flush", []any{}))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Apply(batch...)
	}
}

func TestOptionSetGuifont(t *testing.T) {
	m := New(4, 2)
	guifont := func() (v string) { m.Read(func(s *State) { v = s.GuiFont }); return }

	// Neovim reports the option when the UI attaches, even when it is empty.
	res := m.Apply(ev("option_set", []any{"guifont", ""}))
	if res.GuiFontChanged || guifont() != "" {
		t.Errorf("an empty guifont at attach counted as a change: %+v, %q", res, guifont())
	}
	res = m.Apply(ev("option_set", []any{"guifont", "Fira_Code:h14"}))
	if !res.GuiFontChanged || guifont() != "Fira_Code:h14" {
		t.Errorf("setting guifont: changed=%v value=%q", res.GuiFontChanged, guifont())
	}
	res = m.Apply(ev("option_set", []any{"guifont", "Fira_Code:h14"}))
	if res.GuiFontChanged {
		t.Error("the same value again counted as a change")
	}
	// Several options in one event, and the ones that are not guifont.
	res = m.Apply(ev("option_set", []any{"linespace", int64(2)}, []any{"guifont", "Menlo"}, []any{"ambiwidth", "single"}))
	if !res.GuiFontChanged || guifont() != "Menlo" {
		t.Errorf("guifont among other options: changed=%v value=%q", res.GuiFontChanged, guifont())
	}
	res = m.Apply(ev("option_set", []any{"guifont", ""}))
	if !res.GuiFontChanged || guifont() != "" {
		t.Errorf("clearing guifont: changed=%v value=%q", res.GuiFontChanged, guifont())
	}
	res = m.Apply(ev("option_set", []any{"linespace", int64(1)}))
	if res.GuiFontChanged {
		t.Error("another option counted as a guifont change")
	}
}

func TestMalformedOptionSetIsIgnored(t *testing.T) {
	m := New(4, 2)
	m.Apply(ev("option_set", []any{"guifont", "Menlo"}))
	res := m.Apply(
		ev("option_set", []any{}),
		ev("option_set", []any{"guifont"}),
		ev("option_set", []any{int64(1), "x"}),
		ev("option_set", []any{"guifont", int64(7)}), // not a string: read as empty
	)
	var v string
	m.Read(func(s *State) { v = s.GuiFont })
	if !res.GuiFontChanged || v != "" {
		t.Errorf("a non-string guifont: changed=%v value=%q", res.GuiFontChanged, v)
	}
}
