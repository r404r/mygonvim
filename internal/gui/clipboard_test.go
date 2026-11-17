package gui

import (
	"context"
	"github.com/r404r/mygonvim/internal/nvimproc"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditCommandsAndControlV(t *testing.T) {
	e := newEnv(t, 640, 400)
	e.tt.SetFocused(true)
	e.ed.hooks.PasteClipboard = func() { e.be.Paste("中文\r\n<CR>🙂") }
	e.tt.Command("paste")
	e.tt.Command("copy")
	e.tt.Command("cut")
	if !reflect.DeepEqual(e.be.pastes, []string{"中文\r\n<CR>🙂"}) || e.be.copies != 1 || e.be.cuts != 1 {
		t.Fatalf("paste=%q copy=%d cut=%d", e.be.pastes, e.be.copies, e.be.cuts)
	}
	e.ed.hooks.PasteClipboard = func() {}
	e.tt.Command("paste")
	e.ed.hooks.PasteClipboard = nil
	e.tt.Command("paste")
	if len(e.be.pastes) != 1 {
		t.Fatal("empty clipboard sent a paste")
	}
	if e.ed.handle(ui.InputEvent{Kind: ui.InputCommand, Text: "unknown"}) {
		t.Fatal("unknown command consumed")
	}
	e.tt.Command("undo")
	e.tt.Command("redo")
	e.tt.Command("selectAll")
	e.tt.Key(ui.Ctrl, ui.KeyV)
	if got := e.be.sent(); !reflect.DeepEqual(got, []string{"<Cmd>undo<CR>", "<Cmd>redo<CR>", "<C-\\><C-n>ggVG", "<C-v>"}) {
		t.Fatalf("Ctrl+V sent %q", got)
	}
}

func TestRealNvimEditCommandPasteAndControlV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clipboard.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0600); err != nil {
		t.Fatal(err)
	}
	r := startRealWheelWithClipboard(t, path, &nvimproc.Clipboard{Read: func(context.Context) (string, error) { return "中文\r\n🙂", nil }})
	if err := r.sess.Nvim().Command("let g:pointer_sync=0 | noremap <F12> <Cmd>let g:pointer_sync += 1<CR>"); err != nil {
		t.Fatal(err)
	}
	r.tt.SetFocused(true)
	r.ed.hooks.PasteClipboard = r.sess.PasteClipboard
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

func TestRealNvimEditCommandUndoRedoSelectAll(t *testing.T) {
	r := startRealPointer(t)
	if err := r.sess.Nvim().Command("inoremap <F12> <Cmd>let g:pointer_sync += 1<CR>"); err != nil {
		t.Fatal(err)
	}
	r.tt.SetFocused(true)
	r.sess.Input("iEDIT<Esc>")
	pointerSync(r)
	r.tt.Command("undo")
	pointerSync(r)
	if r.eval("getline(1) ==# 'alpha beta gamma'") != 1 {
		t.Fatal("Undo did not restore original text")
	}
	r.tt.Command("redo")
	pointerSync(r)
	if r.eval("getline(1) ==# 'EDITalpha beta gamma'") != 1 {
		t.Fatal("Redo did not restore edit")
	}
	r.sess.Input("i")
	pointerSync(r)
	r.tt.Command("undo")
	pointerSync(r)
	if r.eval("mode() ==# 'i' && getline(1) ==# 'alpha beta gamma'") != 1 {
		t.Fatal("Insert Undo did not preserve Insert mode")
	}
	r.tt.Command("redo")
	pointerSync(r)
	if r.eval("mode() ==# 'i' && getline(1) ==# 'EDITalpha beta gamma'") != 1 {
		t.Fatal("Insert Redo did not preserve Insert mode")
	}
	r.tt.Command("selectAll")
	pointerSync(r)
	if r.eval("mode() ==# 'V' && line('v') == 1 && line('.') == line('$')") != 1 {
		t.Fatal("Select All did not select the full buffer")
	}
}
