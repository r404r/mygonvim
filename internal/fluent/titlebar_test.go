package fluent

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/appmenu"
)

func titleTester(items []*mygo.MenuItem, w, h int) *ui.Tester {
	b := &TitleBar{Items: items}
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { b.View(c); ui.Box(c).Fill() }) }, w, h)
	tt.SetTitleBar(ui.TitleBar{Height: 32, Right: 138})
	tt.Frame()
	return tt
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

func TestTitleMenusUseTemplateActionsAndState(t *testing.T) {
	var calls []string
	var menu *mygo.Menu
	fm := &appmenu.FontMenu{Families: []string{"First", "Last"}, Sizes: []float32{12, 14}, Family: "First", Size: 12}
	fm.SetFamily = func(s string) { calls = append(calls, s); appmenu.SyncFont(menu, fm, s, fm.Size) }
	fm.SetSize = func(s float32) { calls = append(calls, fmt.Sprint(s)); appmenu.SyncFont(menu, fm, "Last", s) }
	menu = mygo.NewMenu(appmenu.Template("windows", appmenu.Actions{
		Font: fm, Open: func() { calls = append(calls, "open") }, Quit: func() { calls = append(calls, "exit") },
		Edit: func(s string) { calls = append(calls, s) }, About: func() { calls = append(calls, "about") },
	}))
	tt := titleTester(menu.Items(), 800, 480)
	for _, label := range []string{"File", "Edit", "Font", "Help"} {
		if !tt.HasText(label) {
			t.Fatal("missing " + label)
		}
	}
	for _, tc := range []struct {
		title string
		path  []string
		call  string
	}{
		{"File", []string{"Open…"}, "open"}, {"File", []string{"Exit"}, "exit"},
		{"Edit", []string{"Copy"}, "copy"}, {"Edit", []string{"Paste"}, "paste"},
		{"Help", []string{"About MyGoNVim"}, "about"}, {"Font", []string{"Family", "Last"}, "Last"}, {"Font", []string{"Size", "14"}, "14"},
	} {
		click(t, tt, tc.title)
		if tc.title == "File" && !reflect.DeepEqual(tt.Menu(), []string{"Open…", "-", "Exit"}) {
			t.Fatalf("File=%v", tt.Menu())
		}
		if err := tt.ChooseMenuItem(tc.path...); err != nil {
			t.Fatal(err)
		}
		if calls[len(calls)-1] != tc.call {
			t.Fatalf("actions=%v", calls)
		}
	}
	if !menu.ItemByID(appmenu.IDFontFamily+"Last").IsChecked() || menu.ItemByID(appmenu.IDFontFamily+"First").IsChecked() || !menu.ItemByID(appmenu.IDFontSize+"14").IsChecked() {
		t.Fatal("font checks not shared")
	}
	menu.ItemByID(appmenu.IDOpen).SetEnabled(false)
	click(t, tt, "File")
	if err := tt.ChooseMenuItem("Open…"); err == nil {
		t.Fatal("disabled Open chosen")
	}
	tt.CloseMenu()
	menu.ItemByID(appmenu.IDOpen).SetVisible(false)
	click(t, tt, "File")
	if !reflect.DeepEqual(tt.Menu(), []string{"-", "Exit"}) {
		t.Fatalf("hidden Open shown: %v", tt.Menu())
	}
}

func TestHoverSwitchesOnlyWhileMenuOpen(t *testing.T) {
	tt := titleTester(appmenu.Template("windows", appmenu.Actions{}), 800, 480)
	move := func(label string) {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatal(label)
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
	}
	move("Edit")
	if len(tt.Menu()) != 0 {
		t.Fatal("hover opened closed menu")
	}
	click(t, tt, "File")
	move("Edit")
	if !reflect.DeepEqual(tt.Menu(), []string{"Copy", "Paste"}) {
		t.Fatalf("Edit not shown: %v", tt.Menu())
	}
	move("File")
	if !reflect.DeepEqual(tt.Menu(), []string{"Open…", "-", "Exit"}) {
		t.Fatalf("File not shown: %v", tt.Menu())
	}
	tt.CloseMenu()
	move("Help")
	if len(tt.Menu()) != 0 {
		t.Fatal("hover reopened menu")
	}
}

func TestLongFontMenuCanChooseLastFamily(t *testing.T) {
	families := make([]string, 100)
	for i := range families {
		families[i] = fmt.Sprintf("Family %03d", i)
	}
	chosen := ""
	tt := titleTester(appmenu.Template("windows", appmenu.Actions{Font: &appmenu.FontMenu{Families: families, SetFamily: func(s string) { chosen = s }}}), 500, 160)
	click(t, tt, "Font")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	r, ok := tt.Find(families[99])
	if !ok || r.Y < 0 || r.Y+r.H > 160 {
		t.Fatalf("last family is not visible after End: %+v found=%v", r, ok)
	}
	tt.Key(0, ui.KeyEnter)
	if chosen != families[99] {
		t.Fatalf("chosen=%q", chosen)
	}
}

func TestTitleButtonsAvoidSystemButtons(t *testing.T) {
	for _, width := range []int{800, 400, 320} {
		tt := titleTester(appmenu.Template("windows", appmenu.Actions{Font: &appmenu.FontMenu{Sizes: []float32{14}}}), width, 480)
		r, ok := tt.Find("Help")
		if !ok || r.X+r.W > float32(width)-138 {
			t.Fatalf("width=%d Help=%+v reserve starts at %d", width, r, width-138)
		}
	}
}

func TestInactiveTitleTextPixels(t *testing.T) {
	for _, dark := range []bool{false, true} {
		tt := titleTester(appmenu.Template("windows", appmenu.Actions{}), 800, 480)
		tt.SetDark(dark)
		tt.Frame()
		r, _ := tt.Find("File")
		ink := func() uint8 {
			v := uint8(255)
			if dark {
				v = 0
			}
			for y := int(r.Y) + 3; y < int(r.Y+r.H)-3; y++ {
				for x := int(r.X) + 10; x < int(r.X+r.W)-10; x++ {
					p := tt.Image().RGBAAt(x, y).R
					if dark {
						v = max(v, p)
					} else {
						v = min(v, p)
					}
				}
			}
			return v
		}
		active := ink()
		original := append([]byte(nil), tt.Image().Pix...)
		tt.SetWindowActive(false)
		tt.Frame()
		inactive := ink()
		if dark && !(active > 200 && inactive > 100 && inactive < 190) || !dark && !(active < 60 && inactive > 80 && inactive < 180) {
			t.Fatalf("dark=%v text active=%d inactive=%d", dark, active, inactive)
		}
		tt.SetWindowActive(true)
		tt.Frame()
		if !bytes.Equal(original, tt.Image().Pix) {
			t.Fatal("active appearance was not restored")
		}
	}
}
