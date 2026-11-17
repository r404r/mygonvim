package gui

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditCommandsAndControlV(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	e.ed.hooks.ReadClipboard = func() string { return "中文\r\n<CR>🙂" }
	e.tt.Command("paste")
	e.tt.Command("copy")
	e.tt.Command("cut")
	if !reflect.DeepEqual(e.be.pastes, []string{"中文\r\n<CR>🙂"}) || e.be.copies != 1 || e.be.cuts != 1 {
		t.Fatalf("paste=%q copy=%d cut=%d", e.be.pastes, e.be.copies, e.be.cuts)
	}
	e.ed.hooks.ReadClipboard = func() string { return "" }
	e.tt.Command("paste")
	e.ed.hooks.ReadClipboard = nil
	e.tt.Command("paste")
	if len(e.be.pastes) != 1 {
		t.Fatal("empty clipboard sent a paste")
	}
	if e.ed.handle(ui.InputEvent{Kind: ui.InputCommand, Text: "selectAll"}) {
		t.Fatal("unknown command consumed")
	}
	e.tt.Key(ui.Ctrl, ui.KeyV)
	if got := e.be.sent(); !reflect.DeepEqual(got, []string{"<C-v>"}) {
		t.Fatalf("Ctrl+V sent %q", got)
	}
}

func TestRealNvimEditCommandPasteAndControlV(t *testing.T) {
	r := startRealPointer(t)
	r.tt.SetFocused(true)
	r.ed.hooks.ReadClipboard = func() string { return "中文\r\n🙂" }
	r.tt.Command("paste")
	pointerSync(r)
	lines, err := r.sess.Nvim().BufferLines(0, 0, 2, true)
	if err != nil || len(lines) != 2 || string(lines[0]) != "a中文" || string(lines[1]) != "🙂lpha beta gamma" {
		t.Fatalf("paste buffer=%q err=%v", lines, err)
	}
	r.tt.Key(ui.Ctrl, ui.KeyV)
	pointerSync(r)
	if r.eval("mode() ==# nr2char(22)") != 1 {
		t.Fatal("Ctrl+V did not enter block Visual")
	}
}
