package grid

import "testing"

func TestBackgroundOptionIsDistinctFromHighlight(t *testing.T) {
	m := New(80, 24)
	option := func(name string, value any) Result { return m.Apply([]any{"option_set", []any{name, value}}) }
	if !option("background", "dark").BackgroundChanged {
		t.Fatal("missing change")
	}
	if option("background", "dark").BackgroundChanged || option("guifont", "mono:h14").BackgroundChanged || option("background", 42).BackgroundChanged {
		t.Fatal("unrelated change")
	}
	m.Apply([]any{"hl_attr_define", []any{int64(1), map[string]any{"background": int64(0x123456)}}})
	m.Read(func(s *State) {
		if s.Background != "dark" || s.BackgroundVersion != 1 || s.AttrOf(1).Bg != 0x123456 {
			t.Fatalf("option=%q version=%d highlight=%v", s.Background, s.BackgroundVersion, s.AttrOf(1))
		}
	})
	if !option("background", "light").BackgroundChanged {
		t.Fatal("light ignored")
	}
}
