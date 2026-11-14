package appmenu

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Prefixes of the font items' IDs; the family name or the size follows.
const (
	IDFontFamily = "font.family:"
	IDFontSize   = "font.size:"
)

// DefaultSizes are the sizes (DIPs) the Font menu offers.
var DefaultSizes = []float32{10, 11, 12, 13, 14, 15, 16, 18, 20, 22, 24, 28}

// FontMenu describes the Font menu: one group of radio items for the family
// and one for the size.
type FontMenu struct {
	// Families are the names to offer, usually MonospacedFamilies.
	Families []string
	// Sizes are the sizes to offer, usually DefaultSizes.
	Sizes []float32
	// Family and Size are the font in use now, whose items start checked: the
	// family is matched by name against the first name of the list, ignoring
	// case.
	Family string
	Size   float32
	// SetFamily and SetSize are what choosing an item does. They run on the main
	// thread.
	SetFamily func(family string)
	SetSize   func(size float32)
}

// MonospacedFamilies are the names of the families that are fixed-width, in
// the order given, once each. Names starting with a dot are left out: they are
// the system's private fonts, not ones a user picks.
func MonospacedFamilies(all []ui.FontFamily) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range all {
		name := strings.TrimSpace(f.Name)
		if !f.Monospaced || name == "" || strings.HasPrefix(name, ".") || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func sizeLabel(s float32) string { return strconv.FormatFloat(float64(s), 'f', -1, 32) }

// fontItem is the top-level Font menu: Family and Size submenus of radio items.
func fontItem(fm *FontMenu) *mygo.MenuItem {
	var sub []*mygo.MenuItem
	if len(fm.Families) > 0 {
		var fams []*mygo.MenuItem
		for _, name := range fm.Families {
			fams = append(fams, &mygo.MenuItem{
				ID: IDFontFamily + name, Label: name, Type: mygo.MenuItemRadio,
				Checked: strings.EqualFold(name, fm.Family),
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if fm.SetFamily != nil {
						fm.SetFamily(name)
					}
				},
			})
		}
		sub = append(sub, &mygo.MenuItem{Label: "Family", Submenu: fams})
	}
	if len(fm.Sizes) > 0 {
		var sizes []*mygo.MenuItem
		for _, s := range fm.Sizes {
			sizes = append(sizes, &mygo.MenuItem{
				ID: IDFontSize + sizeLabel(s), Label: sizeLabel(s), Type: mygo.MenuItemRadio,
				Checked: s == fm.Size,
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if fm.SetSize != nil {
						fm.SetSize(s)
					}
				},
			})
		}
		sub = append(sub, &mygo.MenuItem{Label: "Size", Submenu: sizes})
	}
	return &mygo.MenuItem{Label: "Font", Submenu: sub}
}

// SyncFont checks the items of the font in use in a menu made from Template,
// and unchecks the rest, for when the font changed some other way than by the
// menu (Neovim's 'guifont'). A font that is not among the items leaves its
// group unchecked.
func SyncFont(m *mygo.Menu, fm *FontMenu, family string, size float32) {
	if m == nil || fm == nil {
		return
	}
	set := func(id string, on bool) {
		if it := m.ItemByID(id); it != nil && it.IsChecked() != on {
			it.SetChecked(on)
		}
	}
	for _, name := range fm.Families {
		set(IDFontFamily+name, strings.EqualFold(name, family))
	}
	for _, s := range fm.Sizes {
		set(IDFontSize+sizeLabel(s), s == size)
	}
}
