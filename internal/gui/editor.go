// Package gui is the editor's window content: a MyGo native view that draws
// the Neovim cell grid and feeds keyboard and input-method events back.
//
// The grid model (package grid) and the key translation (package input) are
// kept apart from this package, which only draws and wires them.
package gui

import (
	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/input"
)

// Backend is what the editor needs of Neovim. Its methods must not block: the
// UI thread calls them (nvimproc.Session queues the calls).
type Backend interface {
	Input(keys string)
	Resize(cols, rows int)
	Command(cmd string)
}

// Config is the editor's look.
type Config struct {
	// FontFamily is a family list as ui.Font takes it, e.g. "Menlo, monospace".
	FontFamily string
	// FontSize is in DIPs.
	FontSize float32
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
}

// Editor is the view of one Neovim. Create it with NewEditor, give View to the
// window, and let OnRedraw receive the session's redraw notifications.
type Editor struct {
	Model *grid.Model
	be    Backend
	tr    *input.Translator
	cfg   Config
	hooks Hooks

	// Main thread only, below.
	fonts      [4]ui.Font // by bold | italic<<1
	metricsFor float32    // the scale cw, ch and base were measured at; 0 before the first
	cw, ch     float32    // a cell, in DIPs, a whole number of pixels each way
	base       float32    // the baseline's distance below a cell's top
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
	if cfg.FontSize <= 0 {
		cfg.FontSize = 14
	}
	if cfg.FontFamily == "" {
		cfg.FontFamily = DefaultFontFamily
	}
	if cfg.Cols < 1 || cfg.Rows < 1 {
		cfg.Cols, cfg.Rows = 80, 24
	}
	e := &Editor{
		Model: grid.New(cfg.Cols, cfg.Rows), be: be, tr: input.New(), cfg: cfg, hooks: hooks,
		reqCols: cfg.Cols, reqRows: cfg.Rows,
		shaped: map[shapeKey][]ui.Glyph{},
	}
	e.fonts = [4]ui.Font{
		{Family: cfg.FontFamily, Size: cfg.FontSize},
		{Family: cfg.FontFamily, Size: cfg.FontSize, Weight: 700},
		{Family: cfg.FontFamily, Size: cfg.FontSize, Italic: true},
		{Family: cfg.FontFamily, Size: cfg.FontSize, Weight: 700, Italic: true},
	}
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
	if res.Flushed && e.hooks.Invalidate != nil {
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
		cw, ch = e.cfg.FontSize*0.6, e.cfg.FontSize*1.3
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
	keys, taken := e.tr.Handle(ev)
	if keys != "" {
		e.be.Input(keys)
	}
	return taken
}
