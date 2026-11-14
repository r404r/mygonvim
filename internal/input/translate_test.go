package input

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func kd(mods ui.Modifiers, k ui.Key) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputKeyDown, Key: k, Mods: mods}
}
func ku(mods ui.Modifiers, k ui.Key) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputKeyUp, Key: k, Mods: mods}
}
func txt(s string) ui.InputEvent { return ui.InputEvent{Kind: ui.InputText, Text: s} }

// press plays what a platform sends for one key: down, the text if any, up.
func press(t *Translator, mods ui.Modifiers, k ui.Key, text string) (sent string) {
	keys, _ := t.Handle(kd(mods, k))
	sent += keys
	if text != "" {
		keys, _ = t.Handle(txt(text))
		sent += keys
	}
	t.Handle(ku(mods, k))
	return sent
}

func TestSpecialKeys(t *testing.T) {
	cases := []struct {
		mods ui.Modifiers
		key  ui.Key
		text string
		want string
	}{
		{0, ui.KeyEscape, "", "<Esc>"},
		{0, ui.KeyEnter, "\r", "<CR>"}, // the text of Enter is dropped
		{0, ui.KeyBackspace, "\x7f", "<BS>"},
		{0, ui.KeyTab, "\t", "<Tab>"},
		{ui.Shift, ui.KeyTab, "", "<S-Tab>"},
		{0, ui.KeyDelete, "", "<Del>"},
		{0, ui.KeyLeft, "", "<Left>"},
		{ui.Shift, ui.KeyRight, "", "<S-Right>"},
		{ui.Ctrl | ui.Shift, ui.KeyUp, "", "<C-S-Up>"},
		{ui.Alt, ui.KeyDown, "", "<A-Down>"},
		{0, ui.KeyPageDown, "", "<PageDown>"},
		{0, ui.KeyHome, "", "<Home>"},
		{0, ui.KeyF5, "", "<F5>"},
		{ui.Ctrl, ui.KeyF12, "", "<C-F12>"},
	}
	for _, c := range cases {
		tr := &Translator{GOOS: "linux"}
		if got := press(tr, c.mods, c.key, c.text); got != c.want {
			t.Errorf("mods %v key %v: sent %q, want %q", c.mods, c.key, got, c.want)
		}
	}
}

func TestPlainKeysSendTheirText(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		tr := &Translator{GOOS: goos}
		if got := press(tr, 0, ui.KeyJ, "j"); got != "j" {
			t.Errorf("%s: j sent %q", goos, got)
		}
		if got := press(tr, ui.Shift, ui.KeyA, "A"); got != "A" {
			t.Errorf("%s: shift-a sent %q", goos, got)
		}
		if got := press(tr, 0, ui.KeySpace, " "); got != " " {
			t.Errorf("%s: space sent %q", goos, got)
		}
		if got := press(tr, ui.Shift, ui.KeyComma, "<"); got != "<lt>" {
			t.Errorf("%s: < sent %q", goos, got)
		}
	}
	// Linux sends the text alone, with no key event.
	tr := &Translator{GOOS: "linux"}
	keys, taken := tr.Handle(txt("x"))
	if keys != "x" || !taken {
		t.Errorf("text alone: %q %v", keys, taken)
	}
}

func TestCtrlAndAltKeys(t *testing.T) {
	cases := []struct {
		goos string
		mods ui.Modifiers
		key  ui.Key
		text string
		want string
	}{
		{"linux", ui.Ctrl, ui.KeyW, "", "<C-w>"},
		{"windows", ui.Ctrl, ui.KeyA, "\x01", "<C-a>"}, // the control character is dropped
		{"darwin", ui.Ctrl, ui.KeyR, "", "<C-r>"},
		{"linux", ui.Ctrl | ui.Shift, ui.KeyA, "", "<C-A>"},
		{"linux", ui.Ctrl, ui.KeySpace, "", "<C-Space>"},
		{"linux", ui.Ctrl, ui.KeyBackslash, "", "<C-Bslash>"},
		{"linux", ui.Ctrl, ui.KeyBracketLeft, "", "<C-[>"},
		{"linux", ui.Alt, ui.KeyX, "", "<A-x>"},
		{"windows", ui.Alt, ui.KeyJ, "", "<A-j>"},
		{"linux", ui.Ctrl | ui.Alt, ui.KeyK, "", "<C-A-k>"},
		{"linux", ui.Ctrl | ui.Shift, ui.Key1, "", "<C-S-1>"},
	}
	for _, c := range cases {
		tr := &Translator{GOOS: c.goos}
		if got := press(tr, c.mods, c.key, c.text); got != c.want {
			t.Errorf("%s mods %v key %v: sent %q, want %q", c.goos, c.mods, c.key, got, c.want)
		}
	}
}

func TestOptionOnMacAndAltGrOnWindowsTypeText(t *testing.T) {
	mac := &Translator{GOOS: "darwin"}
	if got := press(mac, ui.Alt, ui.Key3, "£"); got != "£" {
		t.Errorf("Option-3 on macOS sent %q, want the character", got)
	}
	win := &Translator{GOOS: "windows"}
	if got := press(win, ui.Ctrl|ui.Alt, ui.KeyQ, "@"); got != "@" {
		t.Errorf("AltGr-q on Windows sent %q, want the character", got)
	}
	// On Linux Alt is a modifier.
	lin := &Translator{GOOS: "linux"}
	if got := press(lin, ui.Alt, ui.KeyQ, ""); got != "<A-q>" {
		t.Errorf("Alt-q on Linux sent %q", got)
	}
}

func TestSuperIsNotTaken(t *testing.T) {
	tr := &Translator{GOOS: "darwin"}
	if keys, taken := tr.Handle(kd(ui.Super, ui.KeyC)); keys != "" || taken {
		t.Errorf("Cmd+C: %q %v", keys, taken)
	}
	if _, taken := tr.Handle(ku(ui.Super, ui.KeyC)); taken {
		t.Error("Cmd+C release taken")
	}
	if _, taken := tr.Handle(kd(0, ui.KeyUnknown)); taken {
		t.Error("a modifier on its own taken")
	}
}

func TestCyrillicCtrlUsesThePhysicalKey(t *testing.T) {
	tr := &Translator{GOOS: "linux"}
	ev := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUnknown, PhysicalKey: ui.KeyS, Mods: ui.Ctrl}
	if keys, _ := tr.Handle(ev); keys != "<C-s>" {
		t.Errorf("got %q", keys)
	}
}

func TestDroppedTextDoesNotSwallowTheNextKey(t *testing.T) {
	tr := &Translator{GOOS: "windows"}
	press(tr, ui.Ctrl, ui.KeyA, "") // no text came, released
	if got := press(tr, 0, ui.KeyJ, "j"); got != "j" {
		t.Errorf("next key sent %q", got)
	}
	press(tr, 0, ui.KeyEscape, "")
	if got := press(tr, 0, ui.KeyK, "k"); got != "k" {
		t.Errorf("key after Esc sent %q", got)
	}
}

func TestComposition(t *testing.T) {
	tr := &Translator{GOOS: "darwin"}
	if _, taken := tr.Handle(ui.InputEvent{Kind: ui.InputCompose, Text: "ni", Caret: 2}); !taken {
		t.Fatal("composition not taken")
	}
	if tr.Preedit != "ni" || tr.PreeditCaret != 2 {
		t.Errorf("preedit %q caret %d", tr.Preedit, tr.PreeditCaret)
	}
	keys, _ := tr.Handle(txt("你"))
	if keys != "你" || tr.Preedit != "" {
		t.Errorf("commit sent %q, preedit left %q", keys, tr.Preedit)
	}
}

func TestEscapeText(t *testing.T) {
	cases := map[string]string{
		"abc":    "abc",
		"a<b":    "a<lt>b",
		"<Esc>":  "<lt>Esc>",
		"x\ny":   "x<CR>y",
		"\tq":    "<Tab>q",
		"a\x01b": "ab",
		"中文😀":    "中文😀",
		"a\\b":   "a\\b",
	}
	for in, want := range cases {
		if got := EscapeText(in); got != want {
			t.Errorf("EscapeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIMEWanted(t *testing.T) {
	for mode, want := range map[string]bool{
		"normal": false, "visual": false, "operator": false, "visual_select": false,
		"insert": true, "replace": true, "cmdline_normal": true, "terminal": true,
	} {
		if IMEWanted(mode) != want {
			t.Errorf("IMEWanted(%q) = %v", mode, !want)
		}
	}
}
