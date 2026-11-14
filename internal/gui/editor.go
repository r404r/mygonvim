// Package gui is the editor's window content: a MyGo native view that draws
// the Neovim cell grid and feeds keyboard and input-method events back.
//
// The grid model (package grid) and the key translation (package input) are
// kept apart from this package, which only draws and wires them.
package gui

import (
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/fontcfg"
	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/input"
)

// Backend is what the editor needs of Neovim. Its methods must not block: the
// UI thread calls them (nvimproc.Session queues the calls).
type Backend interface {
	Input(keys string)
	Resize(cols, rows int)
	Command(cmd string)
	// Mouse is nvim_input_mouse: see nvimproc.Session.Mouse.
	Mouse(button, action, modifier string, row, col int)
}

// Config is the editor's look.
type Config struct {
	// FontFamily is a family list as ui.Font takes it, e.g. "Menlo, monospace".
	// It is what the -font flag gave, "" for none. Neovim's guifont option and
	// the font saved from the menu come before it (see fontcfg.Resolve).
	FontFamily string
	// FontSize is in DIPs, 0 for the default.
	FontSize float32
	// SavedFont is the font chosen in the menu earlier, if any.
	SavedFont fontcfg.Choice
	// Cols and Rows are the grid Neovim attached with, before the first frame
	// measures the window.
	Cols, Rows int
}

// Hooks are called from the redraw goroutine as updates arrive; any may be nil.
type Hooks struct {
	// Invalidate asks the window for a new frame: Window.Invalidate.
	Invalidate func()
	// SetTitle sets the window title: Window.SetTitle.
	SetTitle func(title string)
	// FontChanged is called on the main thread when the editor starts to
	// draw with another font, whoever asked for it.
	FontChanged func(fontcfg.Choice)
	// SaveFont stores the font chosen in the menu, to start with next time. A
	// failure is the hook's to report.
	SaveFont func(fontcfg.Choice)
}

// Editor is the view of one Neovim. Create it with NewEditor, give View to the
// window, and let OnRedraw receive the session's redraw notifications.
type Editor struct {
	Model *grid.Model
	be    Backend
	tr    *input.Translator
	wheel *input.Wheel
	cfg   Config
	hooks Hooks

	// The font sources: the -font flag and the defaults never change; the
	// 'guifont' option arrives on Neovim's goroutine, under fontMu. Neovim's
	// own default for 'guifont' is not a choice (see SetDefaultGuiFont).
	flag, def  fontcfg.Choice
	fontMu     sync.Mutex
	guiRaw     string         // 'guifont' as Neovim last said
	guiDefault string         // 'guifont' as Neovim has it before any config
	guiKnown   bool           // guiDefault was set: until then 'guifont' is not used
	guiFont    fontcfg.Choice // guiRaw, parsed; zero when it is the default
	fontDirty  bool

	// Main thread only, below.
	saved      fontcfg.Choice // the menu's choice
	cur        fontcfg.Choice // what fonts were built from
	fonts      [4]ui.Font     // by bold | italic<<1
	metricsFor float32        // the scale cw, ch and base were measured at; 0 before the first
	cw, ch     float32        // a cell, in DIPs, a whole number of pixels each way
	base       float32        // the baseline's distance below a cell's top
	asc, desc  float32
	reqCols    int // the grid last asked of Neovim
	reqRows    int
	focused    bool
	shaped     map[shapeKey][]ui.Glyph
	scratch    []ui.Glyph
	txt        []byte
	cols       []int
}

type shapeKey struct {
	text  string
	style uint8 // bold | italic<<1
}

// NewEditor makes an editor whose model is a blank cfg.Cols x cfg.Rows grid.
func NewEditor(be Backend, cfg Config, hooks Hooks) *Editor {
	if cfg.Cols < 1 || cfg.Rows < 1 {
		cfg.Cols, cfg.Rows = 80, 24
	}
	e := &Editor{
		Model: grid.New(cfg.Cols, cfg.Rows), be: be, tr: input.New(), wheel: input.NewWheel(), cfg: cfg, hooks: hooks,
		reqCols: cfg.Cols, reqRows: cfg.Rows,
		shaped: map[shapeKey][]ui.Glyph{},
		flag:   fontcfg.Choice{Family: cfg.FontFamily, Size: max(cfg.FontSize, 0)},
		def:    fontcfg.Choice{Family: DefaultFontFamily, Size: 14},
		saved:  cfg.SavedFont,
	}
	e.refreshFontQuietly()
	return e
}

// OnRedraw applies a "redraw" notification to the model and asks for a frame
// when it ended a batch (flush). It runs on the Neovim client's goroutine.
func (e *Editor) OnRedraw(updates ...[]any) {
	res := e.Model.Apply(updates...)
	if res.TitleChanged && e.hooks.SetTitle != nil {
		var t string
		e.Model.Read(func(s *grid.State) { t = s.Title })
		e.hooks.SetTitle(t)
	}
	if res.GuiFontChanged {
		// The font changes on the main thread, at the start of a frame.
		var v string
		e.Model.Read(func(s *grid.State) { v = s.GuiFont })
		e.setGuiFont(v)
	}
	if (res.Flushed || res.GuiFontChanged) && e.hooks.Invalidate != nil {
		e.hooks.Invalidate()
	}
}

// View builds the editor's element: one focusable box that draws the grid, takes
// every key, and tells the system where the cursor is for input methods.
func (e *Editor) View(c *ui.Context) {
	var mode string
	var curRow, curCol int
	e.Model.Read(func(s *grid.State) { mode, curRow, curCol = s.Mode, s.CurRow, s.CurCol })

	el := ui.Box(c).Fill().Focusable().FocusRing(false).Clip().AutoFocus().Label("Neovim")
	el.HandleInput(e.handle)
	// The composition shows at the cursor, past the preedit's caret.
	cw, ch := e.cw, e.ch
	if cw == 0 {
		cw, ch = e.cur.Size*0.6, e.cur.Size*1.3
	}
	x := float32(curCol) * cw
	if e.tr.Preedit != "" {
		x += float32(cellsBefore(e.tr.Preedit, e.tr.PreeditCaret)) * cw
	}
	el.TextCaret(ui.Rect{X: x, Y: float32(curRow) * ch, W: 1, H: ch})
	el.InputMethod(input.IMEWanted(mode))
	el.Draw(e.draw)
	e.focused = el.Focused()
}

// handle is the box's HandleInput.
func (e *Editor) handle(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputScroll {
		return e.scroll(ev)
	}
	keys, taken := e.tr.Handle(ev)
	if keys != "" {
		e.be.Input(keys)
	}
	return taken
}

// scroll sends the wheel steps of a scroll event to the grid cell under the
// pointer, where Neovim picks the window to scroll. While Neovim's 'mouse'
// option is off for the mode it is in (the mouse_off event) the steps are
// dropped, as a terminal Neovim would. It takes the event, so that
// MyGo does not look for a scroll container around the editor; before the
// first frame has measured a cell it takes nothing.
func (e *Editor) scroll(ev ui.InputEvent) bool {
	if e.cw <= 0 || e.ch <= 0 {
		return false
	}
	steps := e.wheel.Steps(ev, e.cw, e.ch)
	var off bool
	e.Model.Read(func(s *grid.State) { off = s.MouseOff })
	if len(steps) > 0 && !off {
		cols, rows := e.Model.Size()
		row, col := input.CellAt(ev.X, ev.Y, e.cw, e.ch, cols, rows)
		for _, s := range steps {
			e.be.Mouse("wheel", s.Action, s.Modifier, row, col)
		}
	}
	return true
}
