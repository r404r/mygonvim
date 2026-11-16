package nvimproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryClipboard struct {
	mu            sync.Mutex
	text          string
	reads, writes int
}

func (m *memoryClipboard) read(context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	return m.text, nil
}
func (m *memoryClipboard) write(_ context.Context, s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes++
	m.text = s
	return nil
}
func (m *memoryClipboard) value() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text
}

func waitClipboard(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func clipLua(t *testing.T, s *Session, code string, result any, args ...any) {
	t.Helper()
	if err := s.Nvim().ExecLua(code, result, args...); err != nil {
		t.Fatal(err)
	}
}

func startClipboard(t *testing.T, init string, c *Clipboard) *Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "init.lua")
	if err := os.WriteFile(path, []byte(init), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Start(Options{
		Bin: testBin(t), Args: []string{"--noplugin", "-i", "NONE", "-u", path, "-c", "autocmd VimEnter * let g:clip_ready = 1"},
		Env: PortableEnv(os.Environ(), t.TempDir()), Cols: 60, Rows: 12, Clipboard: c, Logf: t.Logf, OnRedraw: func(...[]any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	waitClipboard(t, "VimEnter", func() bool {
		var ready int
		return s.Nvim().Eval("get(g:, 'clip_ready', 0)", &ready) == nil && ready == 1
	})
	clipLua(t, s, `vim.g.clip_sync = 0; vim.keymap.set({'n','i','x','s','c'}, '<F12>', '<Cmd>let g:clip_sync += 1<CR>')`, nil)
	return s
}

func clipSync(t *testing.T, s *Session) {
	t.Helper()
	var want int
	if err := s.Nvim().Eval("g:clip_sync + 1", &want); err != nil {
		t.Fatal(err)
	}
	s.Input("<F12>")
	waitClipboard(t, "clipboard input barrier", func() bool {
		var got int
		return s.Nvim().Eval("g:clip_sync", &got) == nil && got == want
	})
}

func clipLines(t *testing.T, s *Session) string {
	t.Helper()
	lines, err := s.Nvim().BufferLines(0, 0, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	var b []string
	for _, l := range lines {
		b = append(b, string(l))
	}
	return strings.Join(b, "\n")
}

func TestRealNvimPasteModes(t *testing.T) {
	for _, tc := range []struct{ name, setup, keys, text, want string }{
		{"insert", "abc", "i", "中文\n<CR>\t🙂", "中文\n<CR>\t🙂abc"},
		{"normal", "abc", "", "中文\n<CR>\t🙂", "a中文\n<CR>\t🙂bc"},
		{"visual", "abc", "vl", "中\n文", "中\n文c"},
		{"select", "abc", "vl<C-g>", "中\n文", "中\n文c"},
		{"crlf", "", "i", "中文\r\nabc\r\n🙂\r\n", "中文\nabc\n🙂\n"},
		{"cr", "", "i", "a\rb\rc", "a\nb\nc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startClipboard(t, "", nil)
			clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{...})`, nil, tc.setup)
			s.Input(tc.keys)
			s.Paste(tc.text)
			clipSync(t, s)
			if got := clipLines(t, s); got != tc.want {
				t.Fatalf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRealNvimPasteCommandLine(t *testing.T) {
	s := startClipboard(t, "", nil)
	s.Input(":")
	s.Paste("echo '中文🙂'\r\nSECOND")
	clipSync(t, s)
	var line string
	clipLua(t, s, `return vim.fn.getcmdline()`, &line)
	if line != "echo '中文🙂'" {
		t.Fatalf("cmdline = %q", line)
	}
	s.Input("<Esc>")
	clipSync(t, s)
	if got := clipLines(t, s); got != "" {
		t.Fatalf("cmdline paste changed buffer: %q", got)
	}
}

func TestRealNvimPasteLargeAndUndo(t *testing.T) {
	for _, size := range []int{1 << 20, 10 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			s := startClipboard(t, "", nil)
			unit := "中🙂0123456789\r\n"
			text := strings.Repeat(unit, size/len(unit)+1) + "END"
			want := strings.ReplaceAll(text, "\r\n", "\n")
			start := time.Now()
			s.Paste(text)
			clipSync(t, s)
			if got := clipLines(t, s); got != want {
				t.Fatalf("paste mismatch: bytes=%d want=%d", len(got), len(want))
			}
			t.Logf("%d input bytes: %s", len(text), time.Since(start))
			s.Input("u")
			clipSync(t, s)
			if got := clipLines(t, s); got != "" {
				t.Fatalf("one undo left %d bytes", len(got))
			}
			s.Input("<C-r>")
			clipSync(t, s)
			if got := clipLines(t, s); got != want {
				t.Fatalf("redo mismatch: bytes=%d", len(got))
			}
		})
	}
}

func TestRealNvimPasteSingleCallAndStreamAgree(t *testing.T) {
	s := startClipboard(t, "", nil)
	text := strings.Repeat("中文🙂\r\n", 15000) + "tail"
	s.Paste(text)
	clipSync(t, s)
	want := clipLines(t, s)
	clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{''}); vim.api.nvim_win_set_cursor(0,{1,0})`, nil)
	if ok, err := s.Nvim().Paste(text, true, -1); err != nil || !ok {
		t.Fatalf("single paste: %v %v", ok, err)
	}
	if got := clipLines(t, s); got != want {
		t.Fatalf("single/stream differ: %d/%d bytes", len(got), len(want))
	}
}

func TestRealNvimPasteStopsOnCancelAndError(t *testing.T) {
	for _, failure := range []string{"return false", "error('stop paste')"} {
		t.Run(failure, func(t *testing.T) {
			s := startClipboard(t, "", nil)
			clipLua(t, s, `_G.old_paste=vim.paste; vim.g.paste_calls=0; vim.paste=function() vim.g.paste_calls=vim.g.paste_calls+1; `+failure+` end`, nil)
			s.Paste(strings.Repeat("line\n", 30000))
			clipSync(t, s)
			var calls int
			clipLua(t, s, `return vim.g.paste_calls`, &calls)
			if calls != 1 || clipLines(t, s) != "" {
				t.Fatalf("cancel/error: calls=%d", calls)
			}
			clipLua(t, s, `vim.paste=_G.old_paste`, nil)
			s.Paste("recovered")
			clipSync(t, s)
			if got := clipLines(t, s); got != "recovered" {
				t.Fatalf("next paste = %q", got)
			}
		})
	}
}

func TestRealNvimClipboardRoundTrip(t *testing.T) {
	m := &memoryClipboard{}
	s := startClipboard(t, "", &Clipboard{Read: m.read, Write: m.write})
	clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{'中文🙂','second'}); vim.cmd('normal! gg"+yy')`, nil)
	want := "中文🙂\n"
	if runtime.GOOS == "windows" {
		want = "中文🙂\r\n"
	}
	if got := m.value(); got != want {
		t.Fatalf("yank clipboard = %q, want %q", got, want)
	}
	m.write(context.Background(), "from GUI\r\n中文🙂\r\n")
	clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{''}); vim.cmd('normal! "+p')`, nil)
	if got := clipLines(t, s); got != "\nfrom GUI\n中文🙂" {
		t.Fatalf("put = %q", got)
	}
	var regtype string
	clipLua(t, s, `return vim.fn.getregtype('+')`, &regtype)
	if regtype != "V" {
		t.Fatalf("register type = %q", regtype)
	}
	// A new external characterwise value must be read, not a cached yank.
	m.write(context.Background(), "fresh🙂")
	clipLua(t, s, `return vim.fn.getreg('+')`, &regtype)
	if regtype != "fresh🙂" {
		t.Fatalf("cached clipboard = %q", regtype)
	}
	clipLua(t, s, `return vim.fn.getregtype('+')`, &regtype)
	if regtype != "v" {
		t.Fatalf("character register type = %q", regtype)
	}
}

const userClipboard = `
vim.g.clipboard = {name='user', cache_enabled=false,
 copy={['+']=function() vim.g.user_copy=(vim.g.user_copy or 0)+1 end, ['*']=function() end},
 paste={['+']=function() vim.g.user_paste=(vim.g.user_paste or 0)+1; return {{'USER'},'v'} end, ['*']=function() return {{''},'v'} end}}
`

func TestRealNvimClipboardUserPriority(t *testing.T) {
	for _, tc := range []struct{ name, init, provider string }{
		{"empty", "", "gui"},
		{"user", userClipboard, "user"},
		{"hasThenUser", `vim.fn.has('clipboard'); ` + userClipboard, "builtin"},
		{"hasOnly", `vim.fn.has('clipboard')`, "gui"},
		{"false", `vim.g.clipboard=false`, "builtin"},
		{"unnamedplus", `vim.o.clipboard='unnamedplus'`, "gui"},
		{"initYank", `vim.api.nvim_buf_set_lines(0,0,-1,true,{'init text'}); vim.cmd('normal! "+yy')`, "gui"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &memoryClipboard{}
			s := startClipboard(t, tc.init, &Clipboard{Read: m.read, Write: m.write})
			if m.value() != "" {
				t.Fatal("GUI provider used before VimEnter")
			}
			clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{'yank text'}); vim.cmd('normal! gg"+yy')`, nil)
			m.write(context.Background(), "EXTERNAL")
			clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{''}); vim.cmd('normal! "+p')`, nil)
			var used []int
			clipLua(t, s, `return {vim.g.user_copy or 0, vim.g.user_paste or 0}`, &used)
			m.mu.Lock()
			reads, writes := m.reads, m.writes-1
			m.mu.Unlock()
			switch tc.provider {
			case "gui":
				if reads == 0 || writes == 0 || clipLines(t, s) != "EXTERNAL" {
					t.Fatalf("GUI roundtrip: reads=%d writes=%d buffer=%q", reads, writes, clipLines(t, s))
				}
			case "user":
				if used[0] == 0 || used[1] == 0 || clipLines(t, s) != "USER" || reads != 0 || writes != 0 {
					t.Fatalf("user priority: marks=%v gui=%d/%d buffer=%q", used, reads, writes, clipLines(t, s))
				}
			case "builtin":
				if used[0] != 0 || used[1] != 0 || reads != 0 || writes != 0 {
					t.Fatalf("built-in replaced: marks=%v gui=%d/%d", used, reads, writes)
				}
			}
			if tc.name == "false" {
				var preserved bool
				clipLua(t, s, `return vim.g.clipboard == false`, &preserved)
				if !preserved {
					t.Fatal("false overwritten")
				}
			}
		})
	}
}

func TestRealNvimClipboardSelectionCommands(t *testing.T) {
	for _, keys := range []string{"vl", "V", "<C-v>j", "vl<C-g>", "V<C-g>", "<C-v>j<C-g>"} {
		for _, cut := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cut=%v", keys, cut), func(t *testing.T) {
				m := &memoryClipboard{}
				s := startClipboard(t, "", &Clipboard{Read: m.read, Write: m.write})
				clipLua(t, s, `vim.api.nvim_buf_set_lines(0,0,-1,true,{'abc','def'}); vim.keymap.set({'n','x'},'y','BAD'); vim.keymap.set({'n','x'},'d','BAD')`, nil)
				s.Input(keys)
				if cut {
					s.Cut()
				} else {
					s.Copy()
				}
				clipSync(t, s)
				want := "ab"
				buf := "abc\ndef"
				if strings.HasPrefix(keys, "V") {
					want = "abc\n"
					if cut {
						buf = "def"
					}
				} else if strings.HasPrefix(keys, "<C-v>") {
					want = "a\nd\n"
					if cut {
						buf = "bc\nef"
					}
				} else if cut {
					buf = "c\ndef"
				}
				if runtime.GOOS == "windows" {
					want = strings.ReplaceAll(want, "\n", "\r\n")
				}
				if got := m.value(); got != want {
					t.Fatalf("clipboard=%q want=%q", got, want)
				}
				if got := clipLines(t, s); got != buf {
					t.Fatalf("buffer=%q want=%q", got, buf)
				}
			})
		}
	}
	for _, keys := range []string{"", "i", ":"} {
		t.Run("noSelection/"+keys, func(t *testing.T) {
			m := &memoryClipboard{text: "keep"}
			s := startClipboard(t, "", &Clipboard{Read: m.read, Write: m.write})
			s.Input(keys)
			s.Copy()
			s.Cut()
			clipSync(t, s)
			if m.value() != "keep" || clipLines(t, s) != "" {
				t.Fatal("copy/cut without selection changed text")
			}
		})
	}
}

func TestRealNvimClipboardTimeout(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(fmt.Sprint(write), func(t *testing.T) {
			blocked := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
			s := startClipboard(t, "", &Clipboard{Read: func(ctx context.Context) (string, error) { return "", blocked(ctx) }, Write: func(ctx context.Context, _ string) error { return blocked(ctx) }})
			start := time.Now()
			if write {
				clipLua(t, s, `vim.cmd('normal! "+yy')`, nil)
			} else {
				clipLua(t, s, `return vim.fn.getreg('+')`, new(string))
			}
			if elapsed := time.Since(start); elapsed < time.Second || elapsed > 5*time.Second {
				t.Fatalf("timeout took %s", elapsed)
			}
			s.Input("iALIVE<Esc>")
			clipSync(t, s)
			if clipLines(t, s) != "ALIVE" {
				t.Fatal("Neovim blocked after clipboard timeout")
			}
		})
	}
}

func TestPasteChunkBoundariesAndCancel(t *testing.T) {
	text := strings.Repeat("中🙂\r\n", 20000) + "end"
	var phases []int
	var joined strings.Builder
	err := pasteText(text, func(part string, crlf bool, phase int) (bool, error) {
		if !crlf || strings.HasSuffix(part, "\r") {
			t.Fatal("CRLF boundary broken")
		}
		joined.WriteString(part)
		phases = append(phases, phase)
		return true, nil
	})
	if err != nil || joined.String() != text || !reflect.DeepEqual(phases, []int{1, 2, 3}) {
		t.Fatalf("stream err=%v phases=%v", err, phases)
	}
	for _, cancel := range []bool{false, true} {
		calls := 0
		err = pasteText(text, func(string, bool, int) (bool, error) {
			calls++
			if cancel {
				return false, nil
			}
			return false, errors.New("failed")
		})
		if err == nil || calls != 1 {
			t.Fatalf("failure ignored: %v calls=%d", err, calls)
		}
	}
	calls := 0
	if err := pasteText("", func(string, bool, int) (bool, error) { calls++; return true, nil }); err != nil || calls != 0 {
		t.Fatal("empty paste sent")
	}
}

func TestRealNvimPasteQueueOrderAndLongLine(t *testing.T) {
	s := startClipboard(t, "", nil)
	text := strings.Repeat("中🙂<CR>", 120000)
	s.Input("iPREFIX")
	s.Paste(text)
	s.Input("SUFFIX<Esc>")
	clipSync(t, s)
	if got := clipLines(t, s); got != "PREFIX"+text+"SUFFIX" {
		t.Fatalf("paste/key order mismatch: %d bytes", len(got))
	}
}

func TestClipboardProviderFailureAndOutstandingLimit(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	c := newClipboardProvider(Clipboard{}, func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	c.Read = func(context.Context) (string, error) { return "", errors.New("read failed") }
	c.Write = func(context.Context, string) error { return errors.New("write failed") }
	got, err := c.get("+")
	if err != nil || !reflect.DeepEqual(got, []any{[]string{""}, "v"}) {
		t.Fatalf("error fallback=%v err=%v", got, err)
	}
	c.set([]string{"text"}, "v", "+")
	if len(logs) != 2 || !strings.Contains(logs[0], "read failed") || !strings.Contains(logs[1], "write failed") {
		t.Fatalf("logs=%q", logs)
	}
	started, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release); <-returned })
	c.timeout = 20 * time.Millisecond
	c.Read = func(context.Context) (string, error) { close(started); <-release; close(returned); return "late", nil }
	got, err = c.get("+")
	<-started
	if err != nil || !reflect.DeepEqual(got, []any{[]string{""}, "v"}) {
		t.Fatal("timeout did not return empty")
	}
	got, err = c.get("+")
	if err != nil || !reflect.DeepEqual(got, []any{[]string{""}, "v"}) {
		t.Fatal("pending native operation was not bounded")
	}
	if !strings.Contains(strings.Join(logs, "\n"), "still pending") {
		t.Fatalf("missing busy diagnostic: %q", logs)
	}
}
