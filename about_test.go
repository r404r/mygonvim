package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAboutIconIsEmbeddedRepositoryImage(t *testing.T) {
	want, err := os.ReadFile("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(aboutIcon, want) {
		t.Fatal("embedded icon differs from repository image")
	}
	src, err := ui.DecodeImage(aboutIcon)
	if err != nil {
		t.Fatal(err)
	}
	w, h := ui.ImageSize(src)
	if w <= 0 || h <= 0 {
		t.Fatal("empty icon")
	}
}

type aboutMenuWindow struct {
	menu   *mygo.Menu
	closed func()
}

func (*aboutMenuWindow) Show()                     {}
func (*aboutMenuWindow) Focus()                    {}
func (*aboutMenuWindow) Restore()                  {}
func (*aboutMenuWindow) Invalidate()               {}
func (w *aboutMenuWindow) Close()                  { w.closed() }
func (w *aboutMenuWindow) SetMenu(menu *mygo.Menu) { w.menu = menu }
func (w *aboutMenuWindow) OnClosed(closed func()) func() {
	w.closed = closed
	return func() {}
}

func TestAboutWindowOverridesInheritedMenu(t *testing.T) {
	parent := &mygo.Window{}
	w := &aboutMenuWindow{menu: mygo.NewMenu([]*mygo.MenuItem{{Label: "File"}})}
	factory := aboutWindowFactory(parent, func() mygo.Point { return mygo.Point{} }, func(opts mygo.WindowOptions) aboutNativeWindow {
		if opts.Parent != parent || opts.Content == nil {
			t.Fatal("About window lost its parent or content")
		}
		if opts.AutoHideMenuBar {
			t.Fatal("About menu must be removed, not hidden")
		}
		return w
	})
	closed := false
	if got := factory(func(*ui.Context) {}, func() { closed = true }); got != w {
		t.Fatal("factory returned a different window")
	}
	if w.menu == nil || len(w.menu.Items()) != 0 {
		t.Fatal("About must override the inherited menu with a non-nil empty menu")
	}
	if w.closed == nil {
		t.Fatal("About close callback was not registered")
	}
	w.Close()
	if !closed {
		t.Fatal("About close callback was not called")
	}
}
