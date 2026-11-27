package gui

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/appmenu"
	"github.com/r404r/mygonvim/internal/fluent"
	"github.com/r404r/mygonvim/internal/fontcfg"
)

func fluentEnv(t *testing.T) *env {
	t.Helper()
	e := &env{be: &fake{}}
	bar := &fluent.TitleBar{Items: appmenu.Template("windows", appmenu.Actions{})}
	e.ed = NewEditor(e.be, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24, Chrome: bar}, Hooks{})
	tt := ui.NewTester(e.ed.View, 800, 480)
	e.tt = tt
	tt.SetTitleBar(ui.TitleBar{Height: 32, Right: 138})
	tt.SetFocused(true)
	tt.Frame()
	return e
}

func TestFluentGridResizeOpaqueAndFullScreenLayout(t *testing.T) {
	e := fluentEnv(t)
	e.ed.OnRedraw(ev("default_colors_set", i64(0xffffff, 0x102030, 0xff0000)), ev("grid_cursor_goto", i64(1, 1000, 1000)), ev("flush"))
	check := func(w, h int, title float32) {
		t.Helper()
		e.tt.SetSize(w, h)
		e.tt.SetTitleBar(ui.TitleBar{Height: title, Right: 138})
		e.tt.SetVibrancy(true)
		e.tt.Frame()
		want := [2]int{max(1, int(float32(w)/e.ed.cw)), max(1, int((float32(h)-title)/e.ed.ch))}
		if got := e.be.lastResize(); got != want {
			t.Fatalf("resize=%v want=%v", got, want)
		}
		if got := e.tt.Image().RGBAAt(w-1, h-1); got != (color.RGBA{16, 32, 48, 255}) {
			t.Fatalf("editor background=%v", got)
		}
		if title > 0 && w > 300 {
			if got := e.tt.Image().RGBAAt(w-150, 16); got != (color.RGBA{}) {
				t.Fatalf("composited title background=%v want transparent", got)
			}
		}
		for _, label := range []string{"File", "Edit", "Help"} {
			if e.tt.HasText(label) != (title > 0) {
				t.Fatalf("title=%v %s visible=%v", title, label, e.tt.HasText(label))
			}
		}
	}
	check(800, 480, 32)
	check(640, 360, 32)
	check(800, 480, 0)
	check(800, 480, 32)
	check(2, 34, 32)
}

func TestFluentAltF10AndClipboard(t *testing.T) {
	e := fluentEnv(t)
	pastes := 0
	e.ed.hooks.PasteClipboard = func() { pastes++ }
	for _, code := range []ui.KeyCode{ui.CodeAltLeft, ui.CodeAltRight} {
		e.tt.CodeDown(ui.Alt, code)
		e.tt.CodeUp(0, code)
	}
	if len(e.be.sent()) != 0 || len(e.tt.Menu()) != 0 || !e.tt.Focused("Neovim") {
		t.Fatal("Alt changed input, menu or focus")
	}
	e.tt.Key(0, ui.KeyF10)
	e.tt.Key(ui.Alt, ui.KeyA)
	if !reflect.DeepEqual(e.be.sent(), []string{"<F10>", "<A-a>"}) || len(e.tt.Menu()) != 0 {
		t.Fatalf("sent=%v menu=%v", e.be.sent(), e.tt.Menu())
	}
	e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyC)
	e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyV)
	if e.be.copies != 1 || pastes != 1 || len(e.be.sent()) != 2 {
		t.Fatalf("copies=%d pastes=%d sent=%v", e.be.copies, pastes, e.be.sent())
	}
	e.tt.Key(ui.Ctrl, ui.KeyV)
	if got := e.be.sent(); got[len(got)-1] != "<C-v>" || pastes != 1 {
		t.Fatalf("Ctrl+V=%v pastes=%d", got, pastes)
	}
	// Additional modifiers stay in the existing Neovim path.
	e.tt.Key(ui.Ctrl|ui.Shift|ui.Alt, ui.KeyV)
	if pastes != 1 {
		t.Fatal("Alt+Ctrl+Shift+V pasted")
	}
}

func TestFluentMenuCloseRestoresEditorFocus(t *testing.T) {
	for _, method := range []string{"choice", "escape", "outside", "switch"} {
		t.Run(method, func(t *testing.T) {
			e := fluentEnv(t)
			pastes := 0
			e.ed.hooks.PasteClipboard = func() { pastes++ }
			if err := e.tt.Click("File"); err != nil {
				t.Fatal(err)
			}
			switch method {
			case "choice":
				if err := e.tt.ChooseMenuItem("Open…"); err != nil {
					t.Fatal(err)
				}
			case "escape":
				e.tt.Key(0, ui.KeyEscape)
			case "outside":
				e.tt.ClickAt(500, 300)
			case "switch":
				r, _ := e.tt.Find("Edit")
				e.tt.Move(r.X+r.W/2, r.Y+r.H/2)
				e.tt.Key(0, ui.KeyEscape)
			}
			e.tt.Frame()
			if !e.tt.Focused("Neovim") {
				t.Fatal("editor did not regain focus")
			}
			e.tt.TypeKey(0, ui.KeyI, "i")
			e.tt.Key(0, ui.KeyF10)
			for _, code := range []ui.KeyCode{ui.CodeAltLeft, ui.CodeAltRight} {
				e.tt.CodeDown(ui.Alt, code)
				e.tt.CodeUp(0, code)
			}
			e.tt.Key(ui.Alt, ui.KeyA)
			e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyC)
			e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyV)
			if got := e.be.sent(); !reflect.DeepEqual(got, []string{"i", "<F10>", "<A-a>"}) || len(e.tt.Menu()) != 0 || e.be.copies != 1 || pastes != 1 {
				t.Fatalf("after %s keys=%v menu=%v copy=%d paste=%d", method, got, e.tt.Menu(), e.be.copies, pastes)
			}
		})
	}
}

func TestStandardClipboardKeysStillUseExistingPath(t *testing.T) {
	e := newEnv(t, 800, 480)
	e.tt.SetFocused(true)
	e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyC)
	e.tt.Key(ui.Ctrl|ui.Shift, ui.KeyV)
	if e.be.copies != 0 || !reflect.DeepEqual(e.be.sent(), []string{"<C-C>", "<C-V>"}) {
		t.Fatalf("standard path changed: %v", e.be.sent())
	}
}

func TestBackgroundAppearanceAppliedOnceOnUIFrame(t *testing.T) {
	e := fluentEnv(t)
	var apps, buttons []mygo.ThemeSource
	e.ed.hooks.ApplyAppearance = func(s string) {
		fluent.ApplyAppearance(s, func(v mygo.ThemeSource) { apps = append(apps, v) }, func(v mygo.ThemeSource) { buttons = append(buttons, v) })
	}
	for _, tc := range []struct {
		value string
		want  mygo.ThemeSource
	}{{"dark", mygo.ThemeDark}, {"light", mygo.ThemeLight}, {"", mygo.ThemeSystem}} {
		n := len(apps)
		e.ed.OnRedraw(ev("option_set", []any{"background", tc.value}))
		if len(apps) != n {
			t.Fatal("appearance called from redraw thread")
		}
		e.tt.Frame()
		e.tt.Frame()
		e.ed.OnRedraw(ev("option_set", []any{"background", tc.value}), ev("hl_attr_define", []any{int64(1), map[string]any{"background": int64(0xffffff)}, map[string]any{}, []any{}}))
		e.tt.Frame()
		if len(apps) != n+1 || apps[n] != tc.want || !reflect.DeepEqual(apps, buttons) {
			t.Fatalf("appearance apps=%v buttons=%v", apps, buttons)
		}
	}
}

func TestFluentCompositedMaterialAndFallback(t *testing.T) {
	e := fluentEnv(t)
	e.ed.OnRedraw(ev("default_colors_set", i64(0xffffff, 0x102030, 0xff0000)), ev("flush"))
	for _, dark := range []bool{false, true} {
		for _, vibrancy := range []bool{true, false} {
			for _, reduce := range []bool{false, true} {
				e.tt.SetDark(dark)
				e.tt.SetVibrancy(vibrancy)
				e.tt.SetPreferences(ui.Preferences{ReduceTransparency: reduce})
				e.tt.Frame()
				want := color.RGBA{}
				if !vibrancy || reduce {
					want = color.RGBA{243, 243, 243, 255}
					if dark {
						want = color.RGBA{32, 32, 32, 255}
					}
				}
				if got := e.tt.Image().RGBAAt(500, 16); got != want {
					t.Fatalf("dark=%v vibrancy=%v reduce=%v final title=%v want=%v", dark, vibrancy, reduce, got, want)
				}
				if got := e.tt.Image().RGBAAt(500, 400); got != (color.RGBA{16, 32, 48, 255}) {
					t.Fatalf("final editor=%v", got)
				}
			}
		}
	}
}

func TestFluentGuifontKeepsSharedMenuChecks(t *testing.T) {
	e := fluentEnv(t)
	fm := &appmenu.FontMenu{Families: []string{"monospace", "serif"}, Sizes: []float32{12, 18}, Family: "monospace", Size: 12}
	menu := mygo.NewMenu(appmenu.Template("windows", appmenu.Actions{Font: fm}))
	e.ed.cfg.Chrome.Items = menu.Items()
	e.ed.hooks.FontChanged = func(c fontcfg.Choice) { appmenu.SyncFont(menu, fm, fontcfg.FirstFamily(c.Family), c.Size) }
	e.ed.SetDefaultGuiFont("")
	setGuiFont(e, "serif:h18")
	e.tt.Frame()
	if err := e.tt.Click("Font"); err != nil {
		t.Fatal(err)
	}
	if !menu.ItemByID(appmenu.IDFontFamily+"serif").IsChecked() || menu.ItemByID(appmenu.IDFontFamily+"monospace").IsChecked() || !menu.ItemByID(appmenu.IDFontSize+"18").IsChecked() {
		t.Fatal("guifont did not update shared checks")
	}
	if !reflect.DeepEqual(e.tt.Menu(), []string{"Family", "Size"}) {
		t.Fatalf("Font=%v", e.tt.Menu())
	}
}

func TestFluentPointerWheelAndCaretUseEditorOrigin(t *testing.T) {
	e := fluentEnv(t)
	x, y := float32(2.5)*e.ed.cw, 32+float32(3.5)*e.ed.ch
	e.tt.ClickAt(x, y)
	e.tt.Wheel(x, y, 0, -1)
	got := e.mouse()
	if len(got) != 3 || got[0] != (mouseCall{"left", "press", "", 3, 2}) || got[1] != (mouseCall{"left", "release", "", 3, 2}) || got[2].button != "wheel" || got[2].row != 3 || got[2].col != 2 {
		t.Fatalf("editor coordinates=%v", got)
	}
	e.ed.OnRedraw(ev("grid_cursor_goto", i64(1, 3, 2)), ev("mode_change", []any{"insert", int64(1)}), ev("flush"))
	e.tt.Frame()
	r, on := e.tt.TextCaret()
	if !on || r.X != 2*e.ed.cw || r.Y != 32+3*e.ed.ch {
		t.Fatalf("screen caret=%v active=%v", r, on)
	}
}
