package about

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAboutButtonReliefMatchesSpecification(t *testing.T) {
	for _, label := range []string{"GitHub", "License", "Copy", "Show in folder", "Open directory"} {
		for _, enabled := range []bool{false, true} {
			m := &viewModel{fluent: enabled}
			render := func(reference bool) *ui.Tester {
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Box(c).Fill().Background(ui.Color{R: 100, G: 100, B: 100, A: 255}).Padding(16).Children(func() {
						if !reference {
							m.button(c, label)
							return
						}
						b := ui.Button(c, label)
						if enabled {
							b.Radius(4).Border(0, ui.Color{}).Background(ui.Color{R: 255, G: 255, B: 255, A: 184}).TextColor(ui.Color{A: 230}).NoShadow().
								Shadow(0, 1, 2, 0, ui.Color{A: 36}).InsetShadow(0, 1, 0, 0, ui.Color{R: 255, G: 255, B: 255, A: 180})
						}
					})
				}, 240, 80)
				tt.Frame()
				return tt
			}
			got, want := render(false), render(true)
			if !bytes.Equal(got.Image().Pix, want.Image().Pix) {
				t.Fatalf("%s fluent=%v radius, fill or shadows differ", label, enabled)
			}
		}
	}
}

func TestAboutFluentViewHasNoEditorTitles(t *testing.T) {
	m := &viewModel{fluent: true, nvim: "test", paths: []Path{{Label: "Settings", Value: "sample"}}}
	tt := ui.NewTester(m.view, 720, 780)
	tt.SetTitleBar(ui.TitleBar{Height: 32, Right: 138})
	tt.Frame()
	for _, title := range []string{"File", "Edit", "Font", "Help"} {
		if _, ok := tt.Find(title); ok {
			t.Fatal("editor title in About: " + title)
		}
	}
	for _, label := range []string{"GitHub", "License", "Copy", "Show in folder", "Open directory"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatal("missing button " + label)
		}
	}
}
