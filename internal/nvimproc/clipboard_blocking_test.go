package nvimproc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Bound the probe too: a regression must fail rather than hang the test runner.
func waitClipboardBlocking(t *testing.T, s *Session, blocking bool) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			mode, err := s.Nvim().Mode()
			if err != nil {
				result <- err
				return
			}
			if mode.Blocking == blocking {
				result <- nil
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		result <- fmt.Errorf("Neovim never reached blocking=%v", blocking)
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		s.Close()
		<-result
		t.Fatal("mode probe stalled")
	}
}

func TestRealNvimClipboardSkipsBlocking(t *testing.T) {
	for _, state := range []struct{ name, before, after string }{
		{"register", `"`, "<Esc>"},
		{"insert-register", "i<C-r>", "<Esc><Esc>"},
		{"visual-find", "vf", "d<Esc>"},
		{"hit-enter", ":echo \"a\\nb\"<CR>", "<CR>"},
	} {
		for _, action := range []string{"paste", "copy", "cut"} {
			t.Run(state.name+"/"+action, func(t *testing.T) {
				m := &memoryClipboard{text: "keep"}
				skipped := make(chan struct{}, 1)
				s := startClipboard(t, "", &Clipboard{Read: m.read, Write: m.write}, func(format string, args ...any) {
					t.Logf(format, args...)
					if strings.Contains(fmt.Sprintf(format, args...), "Neovim is waiting for input") {
						skipped <- struct{}{}
					}
				})
				clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{'abcdef'})`, nil)
				s.Input(state.before)
				waitClipboardBlocking(t, s, true)
				switch action {
				case "paste":
					s.Paste("WRONG")
				case "copy":
					s.Copy()
				case "cut":
					s.Cut()
				}
				select {
				case <-skipped:
				case <-time.After(5 * time.Second):
					t.Fatal("clipboard operation was not skipped")
				}
				s.Input(state.after)
				waitClipboardBlocking(t, s, false)
				clipSync(t, s)
				if got := clipLines(t, s); got != "abcdef" || m.value() != "keep" {
					t.Fatalf("skipped operation changed buffer=%q clipboard=%q", got, m.value())
				}
				s.Input("iALIVE<Esc>")
				clipSync(t, s)
				if got := clipLines(t, s); !strings.Contains(got, "ALIVE") {
					t.Fatalf("queued keys lost: %q", got)
				}
			})
		}
	}
}

func TestRealNvimClipboardDeadlineAfterPreflight(t *testing.T) {
	for _, action := range []string{"paste", "stream-first", "stream-middle", "copy", "cut"} {
		t.Run(action, func(t *testing.T) {
			m := &memoryClipboard{text: "keep"}
			entered := make(chan struct{}, 1)
			timedOut := make(chan struct{}, 1)
			pending := make(chan struct{}, 1)
			drained := make(chan struct{}, 1)
			s := startClipboard(t, "", &Clipboard{Read: m.read, Write: m.write}, func(format string, args ...any) {
				t.Logf(format, args...)
				message := fmt.Sprintf(format, args...)
				if strings.Contains(message, "request failed: context deadline exceeded") {
					timedOut <- struct{}{}
				}
				if strings.Contains(message, "completed after deadline") {
					drained <- struct{}{}
				}
				if strings.Contains(message, "previous request still pending") {
					pending <- struct{}{}
				}
			})
			if err := s.Nvim().RegisterHandler("clipboard_test_wait", func() { entered <- struct{}{} }); err != nil {
				t.Fatal(err)
			}
			// These hooks run inside the non-fast request, after the worker's
			// preflight. getchar makes the race deterministic without a timer.
			clipLua(t, s, `
local channel, block_phase = ...
vim.g.wait_keys = ''
_G.clipboard_test_getchar = function()
  vim.rpcnotify(channel, 'clipboard_test_wait')
  local keys = ''
  for _ = 1, 3 do keys = keys .. string.char(vim.fn.getchar()) end
  vim.g.wait_keys = keys
end
vim.g.paste_phases = {}
local original_paste = vim.paste
vim.paste = function(lines, phase)
  local phases = vim.g.paste_phases
  table.insert(phases, phase)
  vim.g.paste_phases = phases
  if phase == vim.g.block_phase then
    _G.clipboard_test_getchar()
  end
  return original_paste(lines, phase)
end
vim.g.block_phase = block_phase
`, nil, s.Nvim().ChannelID(), map[string]int{"paste": -1, "stream-first": 1, "stream-middle": 2}[action])
			if action == "copy" || action == "cut" {
				clipLua(t, s, `
local original_mode = vim.fn.mode
vim.fn.mode = function(...)
  _G.clipboard_test_getchar()
  vim.fn.mode = original_mode
  return original_mode(...)
end
`, nil)
				if action == "copy" {
					s.Copy()
				} else {
					s.Cut()
				}
			} else if action == "paste" {
				s.Paste("FIRST")
			} else {
				s.Paste(strings.Repeat("a\n", pasteChunkSize*2))
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request never entered getchar after preflight")
			}
			// Unlike the four prefix states, getchar processes main-loop
			// events and nvim_get_mode reports blocking=false. Probe it after
			// the notification barrier instead of waiting a fixed interval.
			waitClipboardBlocking(t, s, false)
			select {
			case <-timedOut:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not time out")
			}
			// A second clipboard operation must not start while the old RPC
			// or its stream cleanup can still arrive. Keyboard input may pass.
			s.Paste("WRONG")
			select {
			case <-pending:
			case <-time.After(5 * time.Second):
				t.Fatal("pending clipboard operation was not bounded")
			}
			s.Input("z")
			s.Input("A")
			s.Input("B")
			waitClipboardBlocking(t, s, false)
			select {
			case <-drained:
			case <-time.After(5 * time.Second):
				t.Fatal("late clipboard request did not complete")
			}
			clipSync(t, s)
			var keys string
			clipLua(t, s, `return vim.g.wait_keys`, &keys)
			if keys != "zAB" {
				t.Fatalf("getchar received %q, want zAB", keys)
			}
			var phases []int
			clipLua(t, s, `return vim.g.paste_phases`, &phases)
			want := map[string][]int{"paste": {-1}, "stream-first": {1, 3}, "stream-middle": {1, 2, 3}}[action]
			if !reflect.DeepEqual(phases, want) && !(len(phases) == 0 && len(want) == 0) {
				t.Fatalf("paste phases=%v, want %v", phases, want)
			}
			if m.value() != "keep" || strings.Contains(clipLines(t, s), "WRONG") {
				t.Fatal("timed out request interfered with another operation")
			}
			clipLua(t, s, `vim.g.block_phase = -99`, nil)
			// An RPC barrier after the final input proves ordered keys still
			// run and the completed request cannot answer this new paste.
			s.Input("gg0iORDERED<Esc>")
			s.Paste("NEXT")
			clipSync(t, s)
			if got := clipLines(t, s); !strings.HasPrefix(got, "ORDEREDNEXT") {
				t.Fatalf("recovered input/paste order: %q", got)
			}
		})
	}
}

func TestPasteDeadlineClosesStream(t *testing.T) {
	for _, block := range []int{1, 2} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var phases []int
			err := pasteTextContext(ctx, strings.Repeat("a\n", pasteChunkSize*2), func(text string, crlf bool, phase int) (bool, error) {
				phases = append(phases, phase)
				if phase == block {
					cancel()
				}
				if phase == 3 && text != "" {
					t.Fatalf("deadline sent more text: %q", text)
				}
				return true, nil
			})
			want := []int{1, 3}
			if block == 2 {
				want = []int{1, 2, 3}
			}
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(phases, want) {
				t.Fatalf("phases=%v error=%v", phases, err)
			}
		})
	}
}
