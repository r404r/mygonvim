package fluent

import (
	"fmt"
	"image/color"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestPlatformWindowAndMenuPolicy(t *testing.T) {
	for _, tc := range []struct {
		goos    string
		build   uint32
		enabled bool
	}{
		{"windows", 19045, false}, {"windows", 22000, true}, {"windows", 22621, true}, {"windows", 26100, true},
		{"linux", 26100, false}, {"darwin", 26100, false}, {"windows", 0, false},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.goos, tc.build), func(t *testing.T) {
			on := UseChrome(tc.goos, tc.build)
			if on != tc.enabled {
				t.Fatalf("enabled=%v", on)
			}
			base := mygo.WindowOptions{Title: "editor", Width: 800, Height: 480, MinWidth: 320, MinHeight: 200}
			opts := ConfigureWindow(base, on)
			if on {
				if opts.TitleBarStyle != mygo.TitleBarHidden || opts.TitleBarHeight != 32 || opts.Vibrancy != mygo.VibrancyMica {
					t.Fatalf("chrome options: %+v", opts)
				}
			} else if !reflect.DeepEqual(opts, base) {
				t.Fatalf("standard options changed: %+v", opts)
			}
			menu := mygo.NewMenu(nil)
			calls := 0
			InstallMenu(on, menu, func(m *mygo.Menu) {
				calls++
				if m != menu {
					t.Fatal("different menu")
				}
			})
			if calls != map[bool]int{true: 0, false: 1}[on] {
				t.Fatalf("SetMenu calls=%d", calls)
			}
		})
	}
}

func TestTitleBarFillPixels(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, vibrancy := range []bool{false, true} {
			for _, reduce := range []bool{false, true} {
				bar := &TitleBar{}
				tt := ui.NewTester(func(c *ui.Context) { bar.View(c) }, 400, 100)
				tt.SetTitleBar(ui.TitleBar{Height: 32, Right: 138})
				tt.SetDark(dark)
				tt.SetVibrancy(vibrancy)
				tt.SetPreferences(ui.Preferences{ReduceTransparency: reduce})
				tt.Frame()
				paint, fill := titleBarFill(vibrancy, reduce, dark)
				want := color.RGBA{}
				if paint {
					want = color.RGBA{243, 243, 243, 255}
					if dark {
						want = color.RGBA{32, 32, 32, 255}
					}
				}
				if paint && fill != (ui.Color{R: want.R, G: want.G, B: want.B, A: want.A}) {
					t.Fatalf("fill=%v", fill)
				}
				if got := tt.Image().RGBAAt(230, 16); got != want {
					t.Errorf("dark=%v vibrancy=%v reduce=%v: pixel=%v want=%v", dark, vibrancy, reduce, got, want)
				}
			}
		}
	}
}

func TestAppearanceFromAndApply(t *testing.T) {
	for _, tc := range []struct {
		background string
		source     mygo.ThemeSource
	}{{"dark", mygo.ThemeDark}, {"light", mygo.ThemeLight}, {"", mygo.ThemeSystem}, {"unknown", mygo.ThemeSystem}} {
		var app, buttons []mygo.ThemeSource
		ApplyAppearance(tc.background, func(s mygo.ThemeSource) { app = append(app, s) }, func(s mygo.ThemeSource) { buttons = append(buttons, s) })
		if !reflect.DeepEqual(app, []mygo.ThemeSource{tc.source}) || !reflect.DeepEqual(buttons, app) {
			t.Fatalf("%q app=%v buttons=%v", tc.background, app, buttons)
		}
	}
}

func TestButtonFacesAndStatePriority(t *testing.T) {
	for _, dark := range []bool{false, true} {
		f, _ := buttonStyle(dark, true, false, false, false)
		if f.outer.blur != 2 || f.outer.color.A == 0 || f.inner.color.A == 0 {
			t.Fatal("normal relief absent")
		}
		f, _ = buttonStyle(dark, false, false, false, false)
		wantText := ui.Color{A: 140}
		if dark {
			wantText = ui.Color{R: 255, G: 255, B: 255, A: 128}
		}
		if f.text != wantText || f.outer.blur != 1 || f.inner.color.A != 0 {
			t.Fatalf("inactive=%+v", f)
		}
		f, _ = buttonStyle(dark, false, true, false, false)
		if f.text != wantText || f.outer.blur != 3 {
			t.Fatal("hover did not override inactive shadow")
		}
		f, _ = buttonStyle(dark, false, true, true, false)
		if f.outer.color.A != 0 || f.inner.blur != 2 {
			t.Fatal("press did not override hover")
		}
		f, _ = buttonStyle(dark, false, true, true, true)
		if f.text.A != 97 || f.outer.color.A != 0 || f.inner.color.A != 0 {
			t.Fatal("disabled did not override press")
		}
	}
}
