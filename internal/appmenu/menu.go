// Package appmenu builds File, Edit and Font menus, with the standard app and
// Window menus on macOS.
package appmenu

import (
	"github.com/egoist/mygo"
)

// Menu item IDs, for Menu.ItemByID.
const (
	IDOpen  = "file.open"
	IDExit  = "file.exit"
	IDCopy  = "edit.copy"
	IDPaste = "edit.paste"
)

// Actions are what the menu items do. They run on the main thread, so they must
// return quickly: start a goroutine for anything that waits.
type Actions struct {
	// Open is the File > Open... item.
	Open func()
	// Quit is the File > Exit item (Windows and Linux).
	Quit func()
	// Font, when set, adds the Font menu.
	Font *FontMenu
	// Edit handles copy/paste commands on Windows and Linux. macOS uses the
	// native Edit menu, which dispatches InputCommand to the focused element.
	Edit func(command string)
}

// Template returns the menu bar for goos, as runtime.GOOS spells it.
//
// macOS keeps the app menu (About, Hide, Quit), Edit and Window, and gets Open...
// with Cmd+O next to Close. Elsewhere File has Open... and Exit without
// shortcuts, preserving Ctrl+O, Ctrl+Q and Ctrl+W for Neovim; Edit has Copy and
// Paste on Ctrl+Shift+C/V, preserving Ctrl+V too. With a.Font set there is a
// Font menu after Edit, without shortcuts.
func Template(goos string, a Actions) []*mygo.MenuItem {
	open := &mygo.MenuItem{ID: IDOpen, Label: "Open\u2026", Click: func(*mygo.MenuItem, *mygo.Window) {
		if a.Open != nil {
			a.Open()
		}
	}}
	var font []*mygo.MenuItem
	if a.Font != nil {
		font = []*mygo.MenuItem{fontItem(a.Font)}
	}
	if goos == "darwin" {
		open.Accelerator = "Cmd+O"
		bar := []*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "File", Submenu: []*mygo.MenuItem{open, mygo.Separator(), {Role: mygo.RoleClose}}},
			{Role: mygo.RoleEditMenu},
		}
		bar = append(bar, font...)
		return append(bar, &mygo.MenuItem{Role: mygo.RoleWindowMenu})
	}
	exit := &mygo.MenuItem{ID: IDExit, Label: "Exit", Click: func(*mygo.MenuItem, *mygo.Window) {
		if a.Quit != nil {
			a.Quit()
		}
	}}
	bar := []*mygo.MenuItem{{Label: "File", Submenu: []*mygo.MenuItem{open, mygo.Separator(), exit}}}
	edit := func(id, label, accelerator, command string) *mygo.MenuItem {
		return &mygo.MenuItem{ID: id, Label: label, Accelerator: accelerator, Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.Edit != nil {
				a.Edit(command)
			}
		}}
	}
	bar = append(bar, &mygo.MenuItem{Label: "Edit", Submenu: []*mygo.MenuItem{
		edit(IDCopy, "Copy", "Ctrl+Shift+C", "copy"),
		edit(IDPaste, "Paste", "Ctrl+Shift+V", "paste"),
	}})
	return append(bar, font...)
}

// ChooseAndOpen asks the user for files with pick and gives them to open. When
// the dialog is canceled (no paths) or fails, nothing is opened; a failure is
// logged. It blocks while the dialog is up, so call it from a goroutine.
func ChooseAndOpen(pick func() ([]string, error), open func(paths []string), logf func(format string, args ...any)) {
	paths, err := pick()
	if err != nil {
		if logf != nil {
			logf("open dialog: %v", err)
		}
		return
	}
	if len(paths) == 0 {
		return
	}
	open(paths)
}
