// Package input turns MyGo's keyboard and input-method events into the key
// notation of Neovim's nvim_input ("<Esc>", "<C-w>", "<S-Left>", plain text),
// and tracks the input method's composition.
//
// It depends on MyGo's ui package for the event types only; nothing here draws.
//
// How a key reaches it (docs of ui.InputKind): on macOS and Windows a key that
// types text comes as InputKeyDown followed by InputText; on Linux, only the
// text comes. So a plain printable key sends nothing on its own and its text
// is what goes to Neovim; a key with Ctrl (or Alt) is sent at once as <C-x>
// and the text the system may still type for it is dropped.
package input

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Translator is the state of the keyboard between events. Main thread only.
type Translator struct {
	// GOOS is the system whose keys are read: runtime.GOOS, set by tests.
	GOOS string

	// pending is a printable key down whose text has not come yet.
	pending ui.Key
	// dropText: the key down was sent as a combination or a special key, so
	// the text the system still types for it (Option-E's dead key, "\r" for
	// Enter, a control character for Ctrl+letter on Windows) is dropped.
	dropText bool

	// Preedit is the input method's composition, "" when there is none, and
	// PreeditCaret the rune its caret is at.
	Preedit      string
	PreeditCaret int
}

// New returns a Translator for this system.
func New() *Translator { return &Translator{GOOS: runtime.GOOS} }

// Handle reads one input event. keys is what to send to nvim_input, "" for
// nothing; taken reports whether the event is the editor's, so that MyGo does
// not use it for shortcuts or focus. Pointer and scroll events are not taken
// yet (MVP step 2).
func (t *Translator) Handle(ev ui.InputEvent) (keys string, taken bool) {
	switch ev.Kind {
	case ui.InputKeyDown:
		return t.keyDown(ev)
	case ui.InputKeyUp:
		return "", t.keyUp(ev)
	case ui.InputText:
		return t.text(ev.Text)
	case ui.InputCompose:
		t.Preedit, t.PreeditCaret = ev.Text, ev.Caret
		t.pending = ui.KeyUnknown
		return "", true
	}
	return "", false
}

// specialNames are the keys that never type text.
var specialNames = map[ui.Key]string{
	ui.KeyEnter: "CR", ui.KeyEscape: "Esc", ui.KeyBackspace: "BS", ui.KeyTab: "Tab",
	ui.KeyDelete: "Del", ui.KeyInsert: "Insert", ui.KeyHome: "Home", ui.KeyEnd: "End",
	ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
	ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	ui.KeyF1: "F1", ui.KeyF2: "F2", ui.KeyF3: "F3", ui.KeyF4: "F4", ui.KeyF5: "F5", ui.KeyF6: "F6",
	ui.KeyF7: "F7", ui.KeyF8: "F8", ui.KeyF9: "F9", ui.KeyF10: "F10", ui.KeyF11: "F11", ui.KeyF12: "F12",
}

// printable maps the keys that type a character to it, unshifted, on the
// layout's own terms (ui.Key follows the layout: KeyA types an a).
var printable = map[ui.Key]rune{
	ui.KeySpace: ' ', ui.KeyMinus: '-', ui.KeyEqual: '=', ui.KeyComma: ',', ui.KeyPeriod: '.',
	ui.KeySlash: '/', ui.KeySemicolon: ';', ui.KeyQuote: '\'', ui.KeyBracketLeft: '[',
	ui.KeyBracketRight: ']', ui.KeyBackslash: '\\', ui.KeyBackquote: '`',
	ui.Key0: '0', ui.Key1: '1', ui.Key2: '2', ui.Key3: '3', ui.Key4: '4',
	ui.Key5: '5', ui.Key6: '6', ui.Key7: '7', ui.Key8: '8', ui.Key9: '9',
}

func init() {
	for k := ui.KeyA; k <= ui.KeyZ; k++ {
		printable[k] = 'a' + rune(k-ui.KeyA)
	}
}

func (t *Translator) keyDown(ev ui.InputEvent) (string, bool) {
	key := ev.Key
	if key == ui.KeyUnknown && ev.Mods&ui.Ctrl != 0 {
		// A layout with no Latin letters (Cyrillic): Ctrl+S is still <C-s>.
		key = ev.PhysicalKey
	}
	if key == ui.KeyUnknown {
		return "", false // a modifier on its own, or a key of no name
	}
	if ev.Mods&ui.Super != 0 {
		return "", false // app shortcuts; native Edit sends InputCommand
	}
	t.dropText, t.pending = false, ui.KeyUnknown
	if name, ok := specialNames[key]; ok {
		t.dropText = true
		return "<" + modPrefix(ev.Mods, true) + name + ">", true
	}
	r, ok := printable[key]
	if !ok {
		return "", false
	}
	ctrl, alt := ev.Mods&ui.Ctrl != 0, ev.Mods&ui.Alt != 0
	altGr := t.GOOS == "windows" && ctrl && alt  // AltGr types text on Windows
	option := t.GOOS == "darwin" && alt && !ctrl // Option composes characters on macOS
	if altGr || option || (!ctrl && !alt) {
		t.pending = key // the text comes next
		return "", true
	}
	t.dropText = true
	mods := ev.Mods
	if mods&ui.Shift != 0 && r >= 'a' && r <= 'z' {
		r -= 'a' - 'A' // <C-S-a> is <C-A>
		mods &^= ui.Shift
	}
	return "<" + modPrefix(mods, true) + runeName(r) + ">", true
}

func (t *Translator) keyUp(ev ui.InputEvent) bool {
	key := ev.Key
	if key == ui.KeyUnknown && ev.Mods&ui.Ctrl != 0 {
		key = ev.PhysicalKey
	}
	if key == ui.KeyUnknown || ev.Mods&ui.Super != 0 {
		return false
	}
	_, special := specialNames[key]
	_, text := printable[key]
	if !special && !text {
		return false
	}
	t.dropText, t.pending = false, ui.KeyUnknown
	return true
}

// text reads typed or committed text.
func (t *Translator) text(s string) (string, bool) {
	t.Preedit, t.PreeditCaret = "", 0
	t.pending = ui.KeyUnknown
	if t.dropText {
		t.dropText = false
		return "", true
	}
	return EscapeText(s), true
}

// EscapeText makes literal text safe for nvim_input: "<" would start a key
// name, a newline is Enter, a tab is Tab, other control characters are dropped.
func EscapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			b.WriteString("<lt>")
		case r == '\r' || r == '\n':
			b.WriteString("<CR>")
		case r == '\t':
			b.WriteString("<Tab>")
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// modPrefix is the "C-S-A-" part of a key name. shift says whether Shift
// belongs in it (it does for named keys: <S-Left>).
func modPrefix(m ui.Modifiers, shift bool) string {
	var b strings.Builder
	if m&ui.Ctrl != 0 {
		b.WriteString("C-")
	}
	if shift && m&ui.Shift != 0 {
		b.WriteString("S-")
	}
	if m&ui.Alt != 0 {
		b.WriteString("A-")
	}
	return b.String()
}

// runeName is how a character is written inside <...>.
func runeName(r rune) string {
	switch r {
	case ' ':
		return "Space"
	case '<':
		return "lt"
	case '\\':
		return "Bslash"
	case '|':
		return "Bar"
	}
	return string(r)
}

// IMEWanted reports whether the input method should compose in a Neovim mode
// (a name from mode_change): where text is typed, yes; in Normal and Visual
// mode, where letters are commands, no, as in Neovide.
func IMEWanted(mode string) bool {
	switch mode {
	case "insert", "replace", "cmdline_normal", "cmdline_insert", "cmdline_replace", "terminal":
		return true
	}
	return false
}

// Quote shows a key notation for logs.
func Quote(keys string) string { return strconv.Quote(keys) }
