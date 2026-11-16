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
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/appmenu"
	"github.com/r404r/mygonvim/internal/fontcfg"
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
	var logs io.Closer
	mygo.App.WhenReady(func() { sess, logs = start(opts) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if sess != nil {
		sess.Close()
	}
	if logs != nil {
		log.SetOutput(os.Stderr)
		logs.Close()
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
func (l *lateBackend) Mouse(button, action, modifier string, row, col int) {
	if s := l.be.Load(); s != nil {
		s.Mouse(button, action, modifier, row, col)
	}
}
func (l *lateBackend) Command(c string) {
	if s := l.be.Load(); s != nil {
		s.Command(c)
	}
}

// start finds nvim, runs it, and opens the window. It runs on the main thread
// once the app is ready.
func start(opts options) (*nvimproc.Session, io.Closer) {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	dirs := []string{exeDir}
	if res, err := mygo.App.Path(mygo.PathResources); err == nil && res != exeDir {
		dirs = append(dirs, res)
	}

	// Portable mode: a data/ folder next to the program keeps Neovim's config,
	// data, state and cache, and the app's own, beside it.
	env := os.Environ()
	if data, ok := nvimproc.PortableDir(exeDir); ok {
		if err := nvimproc.EnsurePortableDirs(data); err != nil {
			return fail(err), nil
		}
		env = nvimproc.PortableEnv(env, data)
		app := filepath.Join(data, "mygonvim")
		mygo.App.SetPath(mygo.PathUserData, app)
		mygo.App.SetPath(mygo.PathCache, filepath.Join(data, "cache", "mygonvim"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(app, "logs"))
	}
	logs := startLogging(os.Stderr)
	bin, err := nvimproc.Locate(opts.nvim, os.Getenv("MYGONVIM_NVIM"), dirs)
	if err != nil {
		return fail(err), logs
	}

	// The font chosen in the menu last time, in settings.json of the user data
	// folder (data/mygonvim/ in portable mode). A missing file is normal; a
	// damaged one is reported and ignored, and the next choice replaces it.
	settingsPath := ""
	var saved fontcfg.Choice
	if dir, err := mygo.App.Path(mygo.PathUserData); err != nil {
		log.Printf("settings: no user data folder, the font choice will not be saved: %v", err)
	} else {
		settingsPath = filepath.Join(dir, fontcfg.File)
		if saved, err = fontcfg.Load(settingsPath); err != nil {
			log.Printf("settings: ignoring %v", err)
		}
	}

	cfg := gui.Config{FontFamily: opts.font, FontSize: float32(opts.fontSize), SavedFont: saved, Cols: 100, Rows: 32}
	var win atomic.Pointer[mygo.Window]
	var menu *mygo.Menu
	var fontMenu *appmenu.FontMenu
	late := &lateBackend{}
	ed := gui.NewEditor(late, cfg, gui.Hooks{
		FontChanged: func(c fontcfg.Choice) {
			// Whoever changed the font (the menu, 'guifont'), the checks follow.
			appmenu.SyncFont(menu, fontMenu, fontcfg.FirstFamily(c.Family), c.Size)
		},
		SaveFont: func(c fontcfg.Choice) {
			if settingsPath == "" {
				return
			}
			if err := fontcfg.Save(settingsPath, c); err != nil {
				log.Printf("settings: cannot save the font: %v", err)
			}
		},
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
		InitCmds: initCmds(saved),
		Version:  displayedVersion(),
		OnRedraw: ed.OnRedraw,
		OnExit:   exitHandler(&quitting, mygo.App.Quit, log.Printf),
		Logf:     log.Printf,
	})
	if err != nil {
		return fail(err), logs
	}
	late.be.Store(sess)
	// Neovim 0.12 gives 'guifont' a default font list. It is not the user's
	// choice: the editor treats a 'guifont' equal to it as empty.
	go func() {
		def, err := sess.OptionDefault("guifont")
		if err != nil {
			log.Printf("default guifont: %v", err)
		}
		ed.SetDefaultGuiFont(def)
	}()

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
	w.OnClose(closeHandler(&quitting, sess.Command))
	cur := ed.Font()
	fontMenu = &appmenu.FontMenu{
		Families: appmenu.MonospacedFamilies(ui.FontFamilies()), Sizes: appmenu.DefaultSizes,
		Family: fontcfg.FirstFamily(cur.Family), Size: cur.Size,
		SetFamily: ed.ChooseFamily, SetSize: ed.ChooseSize,
	}
	menu = mygo.NewMenu(appmenu.Template(runtime.GOOS, appmenu.Actions{
		Font: fontMenu,
		// The dialog blocks until it is dismissed, and menu items run on the
		// main thread: wait for it on a goroutine of its own.
		Open: func() {
			go appmenu.ChooseAndOpen(func() ([]string, error) {
				return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: "Open", Multiple: true})
			}, ed.OpenFiles, log.Printf)
		},
		Quit: mygo.App.Quit,
	}))
	mygo.App.SetMenu(menu)
	// Files dropped from Finder or Explorer open in Neovim.
	w.OnFileDrop(func(e *mygo.FileDropEvent) { ed.OpenFiles(e.Paths) })
	win.Store(w)
	var title string
	ed.Model.Read(func(s *grid.State) { title = s.Title })
	if title != "" {
		w.SetTitle(title)
	}
	w.Invalidate()
	return sess, logs
}

// initCmds presets Neovim's 'guifont' with the font saved from the menu, ahead
// of the user's init file.
func initCmds(saved fontcfg.Choice) []string {
	if cmd := fontcfg.StartupCommand(saved); cmd != "" {
		return []string{cmd}
	}
	return nil
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
