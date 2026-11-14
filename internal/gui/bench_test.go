package gui

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// BenchmarkFullScreenFrame is one frame (build, layout, draw, software raster)
// of a full 200x50 grid of varied text with several highlights, at 2x scale:
// what a scroll costs once the glyphs are shaped and cached.
func BenchmarkFullScreenFrame(b *testing.B) {
	be := &fake{}
	ed := NewEditor(be, Config{FontFamily: "monospace", FontSize: 14, Cols: 200, Rows: 50}, Hooks{})
	tt := ui.NewTester(ed.View, 1700, 900)
	tt.SetScale(2)
	tt.Frame()
	updates := [][]any{
		ev("default_colors_set", i64(0xd0d0d0, 0x181818, 0xff0000)),
		ev("hl_attr_define",
			[]any{int64(1), map[string]any{"foreground": int64(0xff8800)}, map[string]any{}, []any{}},
			[]any{int64(2), map[string]any{"foreground": int64(0x55cc77), "italic": true}, map[string]any{}, []any{}},
			[]any{int64(3), map[string]any{"background": int64(0x303a50), "bold": true}, map[string]any{}, []any{}},
		),
	}
	for r := 0; r < ed.reqRows; r++ {
		var cells []any
		for c := 0; c < 190; c += 10 {
			cells = append(cells, []any{fmt.Sprintf("wd%03d-r%02d", c, r), int64((c / 10) % 4)}, []any{" ", int64(0)})
		}
		updates = append(updates, ev("grid_line", []any{int64(1), int64(r), int64(0), cells, false}))
	}
	ed.OnRedraw(updates...)
	tt.Frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}
