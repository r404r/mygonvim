// MyGoNVim is a Neovim GUI drawn with MyGo's native UI: it runs `nvim --embed`,
// keeps Neovim's cell grid in memory, and draws it with ui.Shape and
// Painter.Glyphs, cell by cell. No cgo, no web view.
//
//	go run . [file ...]
//	go tool mygo dev
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/gui"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

const appTitle = "MyGoNVim"

func main() {
	opts, err := parseOptions(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mygonvim:", err)
		os.Exit(2)
	}
	if opts.printVersion {
		writeVersion(os.Stdout)
		return
	}
	mygo.App.SetVersion(displayedVersion())
	var sess *nvimproc.Session
	mygo.App.WhenReady(func() { sess = start(opts) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if sess != nil {
		sess.Close()
	}
}

// lateBackend lets the editor exist before the session does: the editor needs a
// Backend, the session needs the editor's OnRedraw.
type lateBackend struct {
	be atomic.Pointer[nvimproc.Session]
}

func (l *lateBackend) Input(k string) {
	if s := l.be.Load(); s != nil {
		s.Input(k)
	}
}
func (l *lateBackend) Resize(c, r int) {
	if s := l.be.Load(); s != nil {
		s.Resize(c, r)
	}
}
func (l *lateBackend) Command(c string) {
	if s := l.be.Load(); s != nil {
		s.Command(c)
	}
}

// start finds nvim, runs it, and opens the window. It runs on the main thread
// once the app is ready.
func start(opts options) *nvimproc.Session {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	dirs := []string{exeDir}
	if res, err := mygo.App.Path(mygo.PathResources); err == nil && res != exeDir {
		dirs = append(dirs, res)
	}

	bin, err := nvimproc.Locate(opts.nvim, os.Getenv("MYGONVIM_NVIM"), dirs)
	if err != nil {
		return fail(err)
	}

	// Portable mode: a data/ folder next to the program keeps Neovim's config,
	// data, state and cache, and the app's own, beside it.
	env := os.Environ()
	if data, ok := nvimproc.PortableDir(exeDir); ok {
		if err := nvimproc.EnsurePortableDirs(data); err != nil {
			return fail(err)
		}
		env = nvimproc.PortableEnv(env, data)
		app := filepath.Join(data, "mygonvim")
		mygo.App.SetPath(mygo.PathUserData, app)
		mygo.App.SetPath(mygo.PathCache, filepath.Join(data, "cache", "mygonvim"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(app, "logs"))
	}

	cfg := gui.Config{FontFamily: opts.font, FontSize: float32(opts.fontSize), Cols: 100, Rows: 32}
	var win atomic.Pointer[mygo.Window]
	late := &lateBackend{}
	ed := gui.NewEditor(late, cfg, gui.Hooks{
		Invalidate: func() {
			if w := win.Load(); w != nil {
				w.Invalidate()
			}
		},
		SetTitle: func(t string) {
			if w := win.Load(); w != nil {
				w.SetTitle(t)
			}
		},
	})

	var quitting atomic.Bool
	sess, err := nvimproc.Start(nvimproc.Options{
		Bin: bin, Args: opts.files, Env: env, Cols: cfg.Cols, Rows: cfg.Rows,
		OnRedraw: ed.OnRedraw,
		OnExit: func(err error) {
			quitting.Store(true)
			if err != nil {
				log.Printf("nvim exited: %v", err)
			}
			mygo.App.Quit()
		},
		Logf: log.Printf,
	})
	if err != nil {
		return fail(err)
	}
	late.be.Store(sess)

	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     appTitle,
		Width:     opts.width,
		Height:    opts.height,
		MinWidth:  320,
		MinHeight: 200,
		Content:   ui.View(ed.View),
	})
	// Closing the window asks Neovim to quit, which may ask about unsaved
	// changes on its command line; the window closes when nvim has exited.
	w.OnClose(func(e *mygo.CloseEvent) {
		if quitting.Load() {
			return
		}
		e.PreventDefault()
		sess.Command("confirm qa")
	})
	win.Store(w)
	var title string
	ed.Model.Read(func(s *grid.State) { title = s.Title })
	if title != "" {
		w.SetTitle(title)
	}
	w.Invalidate()
	return sess
}

// fail shows why the editor cannot start: a GUI program started from a menu has
// no console to print to.
func fail(err error) *nvimproc.Session {
	log.Println(err)
	mygo.NewWindow(mygo.WindowOptions{
		Title: appTitle, Width: 560, Height: 220,
		Content: ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Center().Gap(12).Padding(24).Children(func() {
				ui.Text(c, "MyGoNVim cannot start Neovim").FontSize(18).Bold()
				ui.Text(c, err.Error()).TextColor(c.Theme().TextMuted)
			})
		}),
	})
	return nil
}
