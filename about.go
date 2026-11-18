package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/about"
)

//go:embed resources/icon.png
var aboutIcon []byte

func newAbout(parent *mygo.Window, process about.Process, client about.LuaClient) *about.Controller {
	icon, err := ui.DecodeImage(aboutIcon)
	if err != nil {
		log.Printf("about icon: %v", err)
	}
	query := &about.Query{Client: client}
	return &about.Controller{
		Process: process, Icon: icon, Query: query.Load, Dispatch: mygo.RunOnMain,
		Services: about.Services{
			Copy:   func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) },
			Reveal: mygo.Shell.ShowItemInFolder, OpenDirectory: mygo.Shell.OpenPath,
			OpenExternal: mygo.Shell.OpenExternal, Stat: os.Stat,
		},
		NewWindow: aboutWindowFactory(parent, func(opts mygo.WindowOptions) aboutNativeWindow {
			return mygo.NewWindow(opts)
		}),
	}
}

type aboutNativeWindow interface {
	about.Window
	SetMenu(*mygo.Menu)
	OnClosed(func()) func()
}

func aboutWindowFactory(parent *mygo.Window, create func(mygo.WindowOptions) aboutNativeWindow) about.Factory {
	return func(view func(*ui.Context), closed func()) about.Window {
		w := create(mygo.WindowOptions{
			Title: "About MyGoNVim", Parent: parent, Width: 720, Height: 780,
			MinWidth: 520, MinHeight: 420, Content: ui.View(view),
		})
		// Override the inherited window menu; macOS keeps its application menu.
		w.SetMenu(mygo.NewMenu(nil))
		w.OnClosed(closed)
		return w
	}
}
