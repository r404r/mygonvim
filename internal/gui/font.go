package gui

import (
	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/fontcfg"
)

// fontStyles builds the four faces of a font by bold | italic<<1.
func fontStyles(c fontcfg.Choice) [4]ui.Font {
	return [4]ui.Font{
		{Family: c.Family, Size: c.Size},
		{Family: c.Family, Size: c.Size, Weight: 700},
		{Family: c.Family, Size: c.Size, Italic: true},
		{Family: c.Family, Size: c.Size, Weight: 700, Italic: true},
	}
}

// Font is the font the editor draws with now. Main thread only.
func (e *Editor) Font() fontcfg.Choice { return e.cur }

// SetFont draws with another font family list and size (DIPs) from the next
// frame on: it rebuilds the faces, forgets the cell size, which is measured
// again whatever the scale, and drops the shaped text. When the cells change
// size the next frame asks Neovim for the grid that now fits. Main thread
// only: it is what the frame uses.
//
// Most callers want to set one of the font sources instead (OnRedraw does so
// for 'guifont', SetSavedFont for the menu's choice); they decide which source
// wins.
func (e *Editor) SetFont(family string, size float32) {
	e.setFont(fontcfg.Choice{Family: family, Size: size})
	if e.hooks.Invalidate != nil {
		e.hooks.Invalidate()
	}
}

func (e *Editor) setFont(c fontcfg.Choice) {
	e.cur = c
	e.fonts = fontStyles(c)
	e.metricsFor = 0 // measure again, also at the same scale
	clear(e.shaped)
	if e.hooks.FontChanged != nil {
		e.hooks.FontChanged(c)
	}
}

// SetSavedFont sets the font chosen in the menu, the source below 'guifont' and
// above the -font flag, and applies the result. Main thread only.
func (e *Editor) SetSavedFont(c fontcfg.Choice) {
	e.saved = c
	e.refreshFont()
	if e.hooks.Invalidate != nil {
		e.hooks.Invalidate()
	}
}

// refreshFont makes the font the sources add up to the current one, if it is
// not already. It sends nothing to Neovim, so a font that came from 'guifont'
// or that was echoed back to it cannot start a loop.
func (e *Editor) refreshFont() {
	if want := e.wantedFont(); want != e.cur {
		e.setFont(want)
	}
}

// refreshFontQuietly is refreshFont for the constructor: no hook is called.
func (e *Editor) refreshFontQuietly() {
	e.cur = e.wantedFont()
	e.fonts = fontStyles(e.cur)
}

// wantedFont adds the sources up.
func (e *Editor) wantedFont() fontcfg.Choice {
	e.fontMu.Lock()
	gui := e.guiFont
	e.fontMu.Unlock()
	// The menu's family is a single name: give it the generic fallback that
	// 'guifont' values get, so that this font and Neovim's echo of it, once the
	// menu has set guifont, are the same font.
	saved := e.saved
	if saved.Family != "" {
		saved.Family = fontcfg.WithFallback(saved.Family)
	}
	return fontcfg.Resolve(gui, saved, e.flag, e.def)
}

// applyPendingFont is called at the start of each frame, on the main thread,
// where a 'guifont' that arrived on Neovim's goroutine takes effect.
func (e *Editor) applyPendingFont() {
	e.fontMu.Lock()
	dirty := e.fontDirty
	e.fontDirty = false
	e.fontMu.Unlock()
	if dirty {
		e.refreshFont()
	}
}

// setGuiFont records a new value of 'guifont' (any goroutine); the next frame
// applies it.
func (e *Editor) setGuiFont(value string) {
	e.fontMu.Lock()
	e.guiRaw = value
	e.parseGuiFontLocked()
	e.fontMu.Unlock()
}

// SetDefaultGuiFont tells the editor what Neovim's 'guifont' is before anything
// configured it: since Neovim 0.12 that is a font list for the platform, not
// "". A 'guifont' with that value is no choice of the user's, and must not
// outrank -font or the font saved from the menu, so it is treated as empty.
// Until it has been called, 'guifont' is not used at all, so that the default
// does not show for a moment at startup. Any goroutine may call it, in any order
// with option_set events; ask Neovim for the value with
// nvimproc.Session.OptionDefault, and call this with "" if that fails.
func (e *Editor) SetDefaultGuiFont(value string) {
	e.fontMu.Lock()
	e.guiDefault = value
	e.guiKnown = true
	e.parseGuiFontLocked()
	e.fontMu.Unlock()
	if e.hooks.Invalidate != nil {
		e.hooks.Invalidate()
	}
}

func (e *Editor) parseGuiFontLocked() {
	e.guiFont = fontcfg.Choice{}
	if e.guiKnown && (e.guiDefault == "" || e.guiRaw != e.guiDefault) {
		e.guiFont = fontcfg.Parse(e.guiRaw).Choice()
	}
	e.fontDirty = true
}

// ChooseFamily is a family picked in the Font menu. Main thread only.
//
// The choice is saved, shown at once unless a 'guifont' that Neovim has set
// still outranks it, and given to Neovim as ":let &guifont = ..." so that
// Neovim and the menu agree. Nothing here answers the option_set event that
// follows: the font it describes is the one already in use.
func (e *Editor) ChooseFamily(name string) {
	e.choose(fontcfg.Choice{Family: name, Size: e.saved.Size}, []string{name}, e.cur.Size)
}

// ChooseSize is a size picked in the Font menu, in DIPs: see ChooseFamily. The
// font list in use stays as it is, with the new size on its first font.
func (e *Editor) ChooseSize(size float32) {
	e.choose(fontcfg.Choice{Family: e.saved.Family, Size: size}, fontcfg.Families(fontcfg.WithFallback(e.cur.Family)), size)
}

func (e *Editor) choose(saved fontcfg.Choice, families []string, size float32) {
	e.SetSavedFont(saved)
	if e.hooks.SaveFont != nil {
		e.hooks.SaveFont(saved)
	}
	e.be.Command(fontcfg.SetCommand(families, size))
}
