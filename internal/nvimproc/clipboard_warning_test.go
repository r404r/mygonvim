package nvimproc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func clipboardMessages(t *testing.T, s *Session) string {
	t.Helper()
	var messages string
	clipLua(t, s, `return vim.fn.execute('messages')`, &messages)
	return messages
}

func waitClipboardWarning(t *testing.T, s *Session, message string) {
	t.Helper()
	waitClipboard(t, message, func() bool { return strings.Contains(clipboardMessages(t, s), message) })
	waitClipboardBlocking(t, s, false)
}

func TestRealNvimPasteTimeoutWarnsAndUndoesOnce(t *testing.T) {
	s := startClipboard(t, `
 local original = vim.paste
 local chunks = 0
 vim.paste = function(lines, phase)
   chunks = chunks + 1
   if chunks == 3 then vim.wait(2500) end
   if phase == 3 then vim.g.stream_closed = true end
   return original(lines, phase)
 end
 local echo = vim.api.nvim_echo
 vim.api.nvim_echo = function(chunks, history, opts)
   if chunks[1][1]:find('paste incomplete', 1, true) then
     vim.g.warning_after_close = vim.g.stream_closed == true
   end
   return echo(chunks, history, opts)
 end
 `, nil)
	text := strings.Repeat(strings.Repeat("x", 1023)+"\n", 320)
	s.Paste(text)
	waitClipboardWarning(t, s, "MyGoNVim: paste incomplete")
	var afterClose bool
	clipLua(t, s, `return vim.g.warning_after_close`, &afterClose)
	if !afterClose {
		t.Fatal("warning preceded phase 3 cleanup")
	}
	got := clipLines(t, s)
	if got == "" || got == text || !strings.HasPrefix(text, got) {
		t.Fatalf("paste is not an incomplete prefix: %d/%d bytes", len(got), len(text))
	}
	s.Input("u")
	clipSync(t, s)
	if got := clipLines(t, s); got != "" {
		t.Fatalf("one undo left %d bytes", len(got))
	}
}

func TestRealNvimClipboardSkipWarns(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%v", pending), func(t *testing.T) {
			skipped := make(chan struct{}, 3)
			deadline := make(chan struct{}, 1)
			s := startClipboard(t, "", nil, func(format string, args ...any) {
				message := fmt.Sprintf(format, args...)
				if strings.Contains(message, "operation skipped") {
					skipped <- struct{}{}
				}
				if strings.Contains(message, "request failed: context deadline exceeded") {
					deadline <- struct{}{}
				}
			})
			if err := s.Nvim().TryResizeUI(120, 12); err != nil {
				t.Fatal(err)
			}
			if pending {
				entered := make(chan struct{}, 1)
				if err := s.Nvim().RegisterHandler("warning_test_wait", func() { entered <- struct{}{} }); err != nil {
					t.Fatal(err)
				}
				clipLua(t, s, `local channel=...; vim.paste=function() vim.rpcnotify(channel,'warning_test_wait'); vim.fn.getchar(); return true end`, nil, s.Nvim().ChannelID())
				s.Paste("pending")
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("paste never entered")
				}
				select {
				case <-deadline:
				case <-time.After(5 * time.Second):
					t.Fatal("request did not time out")
				}
			} else {
				s.Input(`"`)
				waitClipboardBlocking(t, s, true)
			}
			s.Paste("WRONG")
			s.Copy()
			s.Cut()
			for i := 0; i < 3; i++ {
				select {
				case <-skipped:
				case <-time.After(5 * time.Second):
					t.Fatal("operation not skipped")
				}
			}
			s.Input("<Esc>")
			waitClipboardBlocking(t, s, false)
			want := "Neovim is waiting for input"
			if pending {
				want = "previous request still pending"
			}
			waitClipboard(t, "three skip warnings", func() bool {
				return strings.Count(clipboardMessages(t, s), "MyGoNVim: clipboard operation skipped: "+want) == 3
			})
			waitClipboardBlocking(t, s, false)
			s.Input("iALIVE<Esc>")
			clipSync(t, s)
			if !strings.Contains(clipLines(t, s), "ALIVE") {
				t.Fatal("warning stalled queued input")
			}
		})
	}
}

func TestRealNvimClipboardProviderFailureWarns(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(fmt.Sprintf("write=%v", write), func(t *testing.T) {
			failure := errors.New(strings.Repeat("错误🙂\n", 100))
			s := startClipboard(t, "", &Clipboard{
				Read:  func(context.Context) (string, error) { return "", failure },
				Write: func(context.Context, string) error { return failure },
			})
			clipLua(t, s, `vim.o.ruler=false; vim.o.showcmd=false; vim.o.laststatus=0`, nil)
			if err := s.Nvim().TryResizeUI(40, 12); err != nil {
				t.Fatal(err)
			}
			var text string
			if write {
				clipLua(t, s, `vim.cmd('normal! "+yy')`, nil)
			} else {
				clipLua(t, s, `return vim.fn.getreg('+')`, &text)
				if text != "" {
					t.Fatalf("read failure returned %q", text)
				}
			}
			kind := "read"
			if write {
				kind = "write"
			}
			waitClipboardWarning(t, s, "MyGoNVim: clipboard "+kind+" failed:")
			var widthOK bool
			clipLua(t, s, `local m=vim.fn.execute('messages'):match('[^\n]+$'); return vim.fn.strdisplaywidth(m) <= vim.v.echospace`, &widthOK)
			if !widthOK {
				t.Fatal("warning exceeds echospace")
			}
			s.Input("iALIVE<Esc>")
			clipSync(t, s)
			if clipLines(t, s) != "ALIVE" {
				t.Fatal("warning blocked editing")
			}
		})
	}
}

func TestRealNvimGUIClipboardReadFailureWarns(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			s := startClipboard(t, "", &Clipboard{Read: func(ctx context.Context) (string, error) {
				if timeout {
					<-ctx.Done()
					return "", ctx.Err()
				}
				return "", errors.New("native read denied")
			}})
			s.PasteClipboard()
			waitClipboardWarning(t, s, "MyGoNVim: clipboard read failed:")
			if got := clipLines(t, s); got != "" {
				t.Fatalf("failed read inserted %q", got)
			}
			s.Input("iALIVE<Esc>")
			clipSync(t, s)
			if clipLines(t, s) != "ALIVE" {
				t.Fatal("native read stalled queued input")
			}
		})
	}
}
