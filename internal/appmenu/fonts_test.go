package appmenu

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMonospacedFamiliesFiltersFixtureData(t *testing.T) {
	all := []ui.FontFamily{
		{Name: "Registered Mono", Monospaced: true, Registered: true},
		{Name: "Arial", Monospaced: false},
		{Name: "Menlo", Monospaced: true},
		{Name: "Helvetica Neue"},
		{Name: ".SF NS Mono", Monospaced: true}, // private system font
		{Name: "  Fira Code  ", Monospaced: true},
		{Name: "Menlo", Monospaced: true}, // listed twice
		{Name: "", Monospaced: true},
		{Name: "   ", Monospaced: true},
		{Name: "思源等宽", Monospaced: true},
		{Name: "Courier New", Monospaced: true},
	}
	want := []string{"Registered Mono", "Menlo", "Fira Code", "思源等宽", "Courier New"}
	if got := MonospacedFamilies(all); !reflect.DeepEqual(got, want) {
		t.Errorf("MonospacedFamilies = %q, want %q", got, want)
	}
	if got := MonospacedFamilies(nil); got != nil {
		t.Errorf("MonospacedFamilies(nil) = %q, want none", got)
	}
	if got := MonospacedFamilies([]ui.FontFamily{{Name: "Arial"}}); got != nil {
		t.Errorf("no monospaced fonts gave %q, want none", got)
	}
}

func fontGroup(t *testing.T, bar []*mygo.MenuItem, label string) []*mygo.MenuItem {
	t.Helper()
	for _, top := range bar {
		if top.Label == "Font" {
			for _, g := range top.Submenu {
				if g.Label == label {
					return g.Submenu
				}
			}
		}
	}
	t.Fatalf("no Font > %s submenu in the template", label)
	return nil
}

func labels(items []*mygo.MenuItem) (out []string) {
	for _, it := range items {
		out = append(out, it.Label)
	}
	return
}

func checked(items []*mygo.MenuItem) (out []string) {
	for _, it := range items {
		if it.Checked {
			out = append(out, it.Label)
		}
	}
	return
}

func testFontMenu() *FontMenu {
	return &FontMenu{
		Families: []string{"Fira Code", "Menlo", "思源等宽"},
		Sizes:    []float32{12, 13.5, 14, 16},
		Family:   "menlo", // matched ignoring case
		Size:     14,
	}
}

func TestFontMenuPlacesAndChecksItems(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			bar := Template(goos, Actions{Font: testFontMenu()})
			var tops []string
			for _, it := range bar {
				if it.Role != "" {
					tops = append(tops, string(it.Role))
				} else {
					tops = append(tops, it.Label)
				}
			}
			want := []string{"File", "Font"}
			if goos == "darwin" {
				want = []string{"appMenu", "File", "editMenu", "Font", "windowMenu"}
			}
			if !reflect.DeepEqual(tops, want) {
				t.Fatalf("menus = %q, want %q", tops, want)
			}
			fam := fontGroup(t, bar, "Family")
			if got := labels(fam); !reflect.DeepEqual(got, []string{"Fira Code", "Menlo", "思源等宽"}) {
				t.Errorf("family items = %q", got)
			}
			if got := checked(fam); !reflect.DeepEqual(got, []string{"Menlo"}) {
				t.Errorf("checked families = %q, want Menlo", got)
			}
			size := fontGroup(t, bar, "Size")
			if got := labels(size); !reflect.DeepEqual(got, []string{"12", "13.5", "14", "16"}) {
				t.Errorf("size items = %q", got)
			}
			if got := checked(size); !reflect.DeepEqual(got, []string{"14"}) {
				t.Errorf("checked sizes = %q, want 14", got)
			}
			for _, it := range append(fam, size...) {
				if it.Type != mygo.MenuItemRadio {
					t.Errorf("item %q is %q, want a radio item", it.Label, it.Type)
				}
			}
			walk(bar, func(it *mygo.MenuItem) {
				if goos != "darwin" && it.Accelerator != "" {
					t.Errorf("item %q has shortcut %q", it.Label, it.Accelerator)
				}
			})
		})
	}
}

func TestNoFontMenuWithoutFontActions(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		walk(Template(goos, Actions{}), func(it *mygo.MenuItem) {
			if it.Label == "Font" {
				t.Errorf("%s: a Font menu without font actions", goos)
			}
		})
	}
}

func TestFontMenuWithoutFamiliesOrSizes(t *testing.T) {
	bar := Template("windows", Actions{Font: &FontMenu{Sizes: []float32{14}}})
	if len(bar) != 2 || len(bar[1].Submenu) != 1 || bar[1].Submenu[0].Label != "Size" {
		t.Errorf("without monospaced families the Font menu should have only Size: %+v", bar[1].Submenu)
	}
	bar = Template("windows", Actions{Font: &FontMenu{Families: []string{"Menlo"}}})
	if len(bar[1].Submenu) != 1 || bar[1].Submenu[0].Label != "Family" {
		t.Errorf("without sizes the Font menu should have only Family: %+v", bar[1].Submenu)
	}
}

func TestChoosingFontItemsCallsBack(t *testing.T) {
	var fams []string
	var sizes []float32
	fm := testFontMenu()
	fm.SetFamily = func(f string) { fams = append(fams, f) }
	fm.SetSize = func(s float32) { sizes = append(sizes, s) }
	bar := Template("darwin", Actions{Font: fm})
	click := func(items []*mygo.MenuItem, label string) {
		for _, it := range items {
			if it.Label == label {
				it.Click(it, nil)
				return
			}
		}
		t.Fatalf("no item %q", label)
	}
	click(fontGroup(t, bar, "Family"), "思源等宽")
	click(fontGroup(t, bar, "Family"), "Fira Code")
	click(fontGroup(t, bar, "Size"), "13.5")
	if !reflect.DeepEqual(fams, []string{"思源等宽", "Fira Code"}) {
		t.Errorf("families chosen: %q", fams)
	}
	if !reflect.DeepEqual(sizes, []float32{13.5}) {
		t.Errorf("sizes chosen: %v", sizes)
	}
	// Items with no callback behind them do nothing instead of panicking.
	bar = Template("darwin", Actions{Font: testFontMenu()})
	click(fontGroup(t, bar, "Family"), "Menlo")
	click(fontGroup(t, bar, "Size"), "16")
}

func TestSyncFontMovesTheChecks(t *testing.T) {
	fm := testFontMenu()
	m := mygo.NewMenu(Template("darwin", Actions{Font: fm}))
	isOn := func(id string) bool { return m.ItemByID(id).IsChecked() }

	SyncFont(m, fm, "Fira Code", 16)
	if !isOn(IDFontFamily+"Fira Code") || isOn(IDFontFamily+"Menlo") || !isOn(IDFontSize+"16") || isOn(IDFontSize+"14") {
		t.Errorf("after syncing Fira Code 16 the checks are wrong")
	}
	// A font that is not in the list leaves the group unchecked.
	SyncFont(m, fm, "Some Other Font", 17)
	for _, id := range []string{IDFontFamily + "Fira Code", IDFontFamily + "Menlo", IDFontFamily + "思源等宽", IDFontSize + "12", IDFontSize + "13.5", IDFontSize + "14", IDFontSize + "16"} {
		if isOn(id) {
			t.Errorf("%s is still checked for a font that is not in the menu", id)
		}
	}
	SyncFont(m, fm, "MENLO", 13.5)
	if !isOn(IDFontFamily+"Menlo") || !isOn(IDFontSize+"13.5") {
		t.Error("the family is matched ignoring case, and fractional sizes work")
	}
	// Nothing to sync is not an error.
	SyncFont(nil, fm, "Menlo", 14)
	SyncFont(m, nil, "Menlo", 14)
}
