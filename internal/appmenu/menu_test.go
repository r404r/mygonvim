package appmenu

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// walk calls f for every item of the template, submenus included.
func walk(items []*mygo.MenuItem, f func(*mygo.MenuItem)) {
	for _, it := range items {
		f(it)
		walk(it.Submenu, f)
	}
}

func find(items []*mygo.MenuItem, id string) *mygo.MenuItem {
	var found *mygo.MenuItem
	walk(items, func(it *mygo.MenuItem) {
		if it.ID == id {
			found = it
		}
	})
	return found
}

func TestMacMenuKeepsStandardMenusAndAddsOpen(t *testing.T) {
	tpl := Template("darwin", Actions{})
	var tops []string
	for _, it := range tpl {
		if it.Role != "" {
			tops = append(tops, string(it.Role))
		} else {
			tops = append(tops, it.Label)
		}
	}
	if want := []string{"MyGoNVim", "File", "editMenu", "windowMenu"}; !reflect.DeepEqual(tops, want) {
		t.Fatalf("menus = %q, want %q", tops, want)
	}
	file := tpl[1].Submenu
	if len(file) != 3 {
		t.Fatalf("File has %d items, want Open, a separator, Close", len(file))
	}
	if file[0].ID != IDOpen || file[0].Label != "Open\u2026" || file[0].Accelerator != "Cmd+O" {
		t.Errorf("first File item = %+v, want Open... with Cmd+O", file[0])
	}
	if file[1].Type != mygo.MenuItemSeparator {
		t.Errorf("second File item = %+v, want a separator", file[1])
	}
	if file[2].Role != mygo.RoleClose {
		t.Errorf("last File item = %+v, want the Close role", file[2])
	}
}

func TestOtherSystemsGetFileMenuWithoutShortcuts(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			tpl := Template(goos, Actions{})
			if len(tpl) != 3 || tpl[0].Label != "File" || tpl[1].Label != "Edit" {
				t.Fatalf("menu bar = %+v, want File and Edit", tpl)
			}
			file := tpl[0].Submenu
			if len(file) != 3 || file[0].ID != IDOpen || file[1].Type != mygo.MenuItemSeparator || file[2].ID != IDExit {
				t.Fatalf("File = %+v, want Open..., a separator, Exit", file)
			}
			if file[0].Label != "Open\u2026" || file[2].Label != "Exit" {
				t.Errorf("labels = %q and %q, want \"Open\u2026\" and \"Exit\"", file[0].Label, file[2].Label)
			}
			walk(tpl[:1], func(it *mygo.MenuItem) {
				if it.Accelerator != "" {
					t.Errorf("item %q has shortcut %q: it would take the key from Neovim", it.Label, it.Accelerator)
				}
				if it.Role != "" {
					t.Errorf("item %q has role %q: roles bring their own shortcuts", it.Label, it.Role)
				}
			})
		})
	}
}

func TestNoControlShortcutsAnywhere(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		walk(Template(goos, Actions{}), func(it *mygo.MenuItem) {
			a := strings.ToLower(it.Accelerator)
			if a == "ctrl+o" || a == "cmdorctrl+o" {
				t.Errorf("%s: %q uses %q, which is Ctrl+O on Windows", goos, it.Label, it.Accelerator)
			}
		})
	}
}

func TestClickingItemsRunsActions(t *testing.T) {
	var opened, quit int
	for _, goos := range []string{"darwin", "windows"} {
		tpl := Template(goos, Actions{Open: func() { opened++ }, Quit: func() { quit++ }})
		item := find(tpl, IDOpen)
		item.Click(item, nil)
		if opened != 1 {
			t.Errorf("%s: Open ran %d times after one click, want 1", goos, opened)
		}
		opened = 0
	}
	exit := find(Template("windows", Actions{Quit: func() { quit++ }}), IDExit)
	exit.Click(exit, nil)
	if quit != 1 {
		t.Errorf("Exit ran Quit %d times, want 1", quit)
	}
	// Items with no action behind them do nothing instead of panicking.
	for _, id := range []string{IDOpen, IDExit} {
		it := find(Template("windows", Actions{}), id)
		it.Click(it, nil)
	}
}

func TestTemplateBuildsAMyGoMenu(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		m := mygo.NewMenu(Template(goos, Actions{}))
		if it := m.ItemByID(IDOpen); it == nil || it.Label != "Open\u2026" {
			t.Errorf("%s: menu has no Open... item: %+v", goos, it)
		}
	}
}

func TestClipboardMenuPreservesVimKeys(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		var commands []string
		tpl := Template(goos, Actions{Edit: func(s string) { commands = append(commands, s) }})
		for _, tc := range []struct{ id, key string }{{IDCopy, "Ctrl+Shift+C"}, {IDPaste, "Ctrl+Shift+V"}} {
			item := find(tpl, tc.id)
			if item == nil || item.Role != "" || item.Accelerator != tc.key {
				t.Fatalf("%s: %s = %+v", goos, tc.id, item)
			}
			item.Click(item, nil)
		}
		if !reflect.DeepEqual(commands, []string{"copy", "paste"}) {
			t.Fatalf("commands=%q", commands)
		}
		walk(tpl, func(it *mygo.MenuItem) {
			key := strings.ToLower(it.Accelerator)
			if key == "ctrl+v" || key == "cmdorctrl+v" || it.Role == mygo.RolePaste {
				t.Fatalf("Ctrl+V captured by %+v", it)
			}
		})
	}
}

func TestChooseAndOpen(t *testing.T) {
	var logged []string
	logf := func(f string, a ...any) { logged = append(logged, f) }
	var got [][]string
	open := func(p []string) { got = append(got, p) }

	ChooseAndOpen(func() ([]string, error) { return []string{"/a/b", "/a/c d"}, nil }, open, logf)
	if want := [][]string{{"/a/b", "/a/c d"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("opened %q, want %q", got, want)
	}

	got = nil
	ChooseAndOpen(func() ([]string, error) { return nil, nil }, open, logf) // canceled
	ChooseAndOpen(func() ([]string, error) { return []string{}, nil }, open, logf)
	if got != nil {
		t.Errorf("a canceled dialog opened %q, want nothing", got)
	}
	if logged != nil {
		t.Errorf("a canceled dialog logged %q, want nothing", logged)
	}

	ChooseAndOpen(func() ([]string, error) { return []string{"/x"}, errors.New("boom") }, open, logf)
	if got != nil {
		t.Errorf("a failed dialog opened %q, want nothing", got)
	}
	if len(logged) != 1 {
		t.Errorf("a failed dialog logged %d times, want once", len(logged))
	}
	ChooseAndOpen(func() ([]string, error) { return nil, errors.New("boom") }, open, nil) // nil logf is fine
}

func TestAboutMenuPlacementAndAction(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			calls := 0
			tpl := Template(goos, Actions{Font: testFontMenu(), About: func() { calls++ }})
			count := 0
			roles := map[mygo.MenuRole]bool{}
			walk(tpl, func(it *mygo.MenuItem) {
				roles[it.Role] = true
				if it.ID == IDAbout {
					count++
					if it.Label != "About MyGoNVim" || it.Accelerator != "" || it.Role != "" {
						t.Fatalf("About: %+v", it)
					}
					it.Click(it, nil)
				}
				if it.Role == mygo.RoleAppMenu || it.Role == mygo.RoleAbout {
					t.Fatalf("system About could duplicate the window: %+v", it)
				}
			})
			if count != 1 || calls != 1 {
				t.Fatalf("About count=%d calls=%d", count, calls)
			}
			if goos == "darwin" {
				if tpl[0].Submenu[0].ID != IDAbout {
					t.Fatal("About is not in application menu")
				}
				for _, role := range []mygo.MenuRole{mygo.RoleServices, mygo.RoleHide, mygo.RoleHideOthers, mygo.RoleUnhide, mygo.RoleQuit} {
					if !roles[role] {
						t.Errorf("missing %s", role)
					}
				}
			} else {
				help := tpl[len(tpl)-1]
				if tpl[len(tpl)-2].Label != "Font" || help.Label != "Help" || help.Accelerator != "" || len(help.Submenu) != 1 || help.Submenu[0].ID != IDAbout {
					t.Fatalf("Help placement: %+v", tpl)
				}
			}
			it := find(Template(goos, Actions{}), IDAbout)
			it.Click(it, nil)
		})
	}
}
