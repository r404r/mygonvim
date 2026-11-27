// MyGoNVim is a Neovim GUI drawn with MyGo's native UI: it runs `nvim --embed`,
// keeps Neovim's cell grid in memory, and draws it with ui.Shape and
// Painter.Glyphs, cell by cell. No cgo, no web view.
//
//	go run . [file ...]
//	go tool mygo dev
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/about"
	"github.com/r404r/mygonvim/internal/appmenu"
	"github.com/r404r/mygonvim/internal/fontcfg"
	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/gui"
	"github.com/r404r/mygonvim/internal/loginenv"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

const appTitle = "MyGoNVim"

func main() {
	if len(os.Args) == 2 && os.Args[1] == clipboardReadArgument {
		os.Exit(runClipboardReader())
	}
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
	var running *startup
	var logs io.Closer
	mygo.App.WhenReady(func() { running, logs = start(opts) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if running != nil {
		running.Close()
	}
	if logs != nil {
		log.SetOutput(os.Stderr)
		logs.Close()
	}
}

// lateBackend lets the editor exist before the session does: the editor needs a
// Backend, the session needs the editor's OnRedraw.
type lateBackend struct {
	be         atomic.Pointer[nvimproc.Session]
	mu         sync.Mutex
	cols, rows int
	pending    []pendingInput
	bytes      int
	failed     bool
	overflow   bool
}

const maxPendingBytes = 1 << 20
const maxPendingInputs = 1024

type pendingInput struct {
	kind byte
	text string
}

func (o pendingInput) send(s *nvimproc.Session) {
	switch o.kind {
	case 'i':
		s.Input(o.text)
	case 'p':
		s.Paste(o.text)
	case 'c':
		s.Command(o.text)
	}
}

func (l *lateBackend) attach(s *nvimproc.Session) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed {
		return false
	}
	l.be.Store(s)
	if l.cols > 0 && l.rows > 0 {
		s.Resize(l.cols, l.rows)
	}
	for _, input := range l.pending {
		input.send(s)
	}
	l.pending, l.bytes = nil, 0
	return true
}

func (l *lateBackend) send(kind byte, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed || text == "" {
		return
	}
	input := pendingInput{kind, text}
	if s := l.be.Load(); s != nil {
		input.send(s)
		return
	}
	if len(text) > maxPendingBytes-l.bytes || len(l.pending) >= maxPendingInputs {
		if !l.overflow {
			log.Printf("startup: pending input limit reached; ignoring excess input")
			l.overflow = true
		}
		return
	}
	l.pending = append(l.pending, input)
	l.bytes += len(text)
}

func (l *lateBackend) discard() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failed = true
	l.pending, l.bytes = nil, 0
}

func (l *lateBackend) Input(k string) { l.send('i', k) }
func (l *lateBackend) Resize(c, r int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cols, l.rows = c, r
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
	l.send('c', c)
}

func (l *lateBackend) Paste(text string) {
	l.send('p', text)
}
func (l *lateBackend) Copy() {
	if s := l.be.Load(); s != nil {
		s.Copy()
	}
}
func (l *lateBackend) Cut() {
	if s := l.be.Load(); s != nil {
		s.Cut()
	}
}

// start opens the editor on the main thread before collecting the login
// environment. The worker then locates and starts Neovim.
func start(opts options) (*startup, io.Closer) {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	dirs := []string{exeDir}
	if res, err := mygo.App.Path(mygo.PathResources); err == nil && res != exeDir {
		dirs = append(dirs, res)
	}

	// Portable mode: a data/ folder next to the program keeps Neovim's config,
	// data, state and cache, and the app's own, beside it.
	env := os.Environ()
	portable := false
	portableDir := ""
	if data, ok := nvimproc.PortableDir(exeDir); ok {
		portable = true
		portableDir = data
		if err := nvimproc.EnsurePortableDirs(data); err != nil {
			fail(err)
			return nil, nil
		}
		app := filepath.Join(data, "mygonvim")
		mygo.App.SetPath(mygo.PathUserData, app)
		mygo.App.SetPath(mygo.PathCache, filepath.Join(data, "cache", "mygonvim"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(app, "logs"))
	}
	logs := startLogging(os.Stderr)
	startCrashLogging()
	logPath := ""
	if f, ok := logs.(interface{ Path() string }); ok {
		logPath = f.Path()
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
		PasteClipboard: func() {
			if s := late.be.Load(); s != nil {
				s.PasteClipboard()
			}
		},
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
	cwd, cwdErr := os.Getwd()
	home, homeErr := os.UserHomeDir()
	ctx, cancel := context.WithCancel(context.Background())
	running := &startup{cancel: cancel, done: make(chan struct{}), late: late}

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
		if sess := late.be.Load(); sess != nil {
			closeHandler(&quitting, sess.Command)(e)
		} else {
			cancel()
		}
	})
	w.OnClosed(func() { late.discard(); cancel() })
	failed := func(err error) {
		startupFailure(late, err, func(err error) {
			mygo.App.SetMenu(mygo.NewMenu(nil))
			fail(err)
		}, func() { win.Store(nil); w.Destroy() })
	}
	cur := ed.Font()
	fontMenu = &appmenu.FontMenu{
		Families: appmenu.MonospacedFamilies(ui.FontFamilies()), Sizes: appmenu.DefaultSizes,
		Family: fontcfg.FirstFamily(cur.Family), Size: cur.Size,
		SetFamily: ed.ChooseFamily, SetSize: ed.ChooseSize,
	}
	process := about.Process{Version: displayedVersion(), Portable: portable, Settings: settingsPath, Log: logPath, LoginEnv: "Pending"}
	aboutWindow := newAbout(w, process, lateLuaClient{late})
	menu = mygo.NewMenu(appmenu.Template(runtime.GOOS, appmenu.Actions{
		Font:  fontMenu,
		About: aboutWindow.Open,
		// The dialog blocks until it is dismissed, and menu items run on the
		// main thread: wait for it on a goroutine of its own.
		Open: func() {
			go appmenu.ChooseAndOpen(func() ([]string, error) {
				return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: "Open", Multiple: true})
			}, ed.OpenFiles, log.Printf)
		},
		Quit: mygo.App.Quit,
		Edit: func(command string) { ed.EditCommand(command) },
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
	clip := systemClipboard()
	go func() {
		defer close(running.done)
		username := ""
		if current, err := user.Current(); err == nil {
			username = current.Username
		}
		result := prepareEnvironment(ctx, loginenv.Options{
			Inputs: loginenv.Inputs{GOOS: runtime.GOOS, Parent: os.Getppid(), StdoutTTY: loginenv.StdoutTTY(), Env: env, SettingsOff: loginenv.SettingsOff(settingsPath)},
			Home:   home, Username: username, Logf: log.Printf,
		}, portableDir)
		if ctx.Err() != nil {
			return
		}
		bin, err := nvimproc.LocateIn(opts.nvim, os.Getenv("MYGONVIM_NVIM"), dirs, result.Env)
		logStartup(displayedVersion(), portable, bin, logPath)
		if ctx.Err() != nil {
			return
		}
		mygo.RunOnMain(func() {
			if ctx.Err() != nil {
				return
			}
			process.Nvim, process.LoginEnv = bin, result.Status
			aboutWindow.UpdateProcess(process)
			if err != nil {
				failed(err)
			}
		})
		if ctx.Err() != nil || err != nil {
			return
		}
		sess, err := nvimproc.StartContext(ctx, nvimproc.Options{
			Bin: bin, Args: opts.files, Env: result.Env, Cols: cfg.Cols, Rows: cfg.Rows,
			Dir: nvimStartDir(runtime.GOOS, cwd, cwdErr, home, homeErr), InitCmds: initCmds(saved),
			Version: displayedVersion(), Clipboard: clip, OnRedraw: ed.OnRedraw,
			OnWindowLayout: ed.OnWindowLayout, OnExit: func(err error) {
				// A failed handshake must leave its error window available.
				<-running.done
				if late.be.Load() != nil {
					exitHandler(&quitting, mygo.App.Quit, log.Printf)(err)
				}
			}, Logf: log.Printf,
		})
		if sess != nil {
			if ctx.Err() != nil || !late.attach(sess) {
				sess.Close()
				return
			}
			// Ignore Neovim's platform default font until the user chooses one.
			go func() {
				def, err := sess.OptionDefault("guifont")
				if err != nil {
					log.Printf("default guifont: %v", err)
				}
				ed.SetDefaultGuiFont(def)
			}()
		}
		mygo.RunOnMain(func() {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				failed(err)
			}
		})
	}()
	return running, logs
}

// nvimStartDir uses the home directory for macOS launches from /, as in Finder.
// An empty result leaves Neovim inheriting this process's working directory.
func nvimStartDir(goos, cwd string, cwdErr error, home string, homeErr error) string {
	if goos == "darwin" && cwdErr == nil && cwd == "/" && homeErr == nil && home != "" {
		return home
	}
	return ""
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
func fail(err error) {
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
}
