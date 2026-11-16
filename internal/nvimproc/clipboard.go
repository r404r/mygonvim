package nvimproc

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"time"
)

// Clipboard connects Neovim's registers to a plain-text clipboard. Callbacks
// run off the UI thread and must honor cancellation before changing contents.
type Clipboard struct {
	Read  func(context.Context) (string, error)
	Write func(context.Context, string) error
}

type clipboardProvider struct {
	Clipboard
	busy    chan struct{}
	timeout time.Duration
	logf    func(string, ...any)
}

func newClipboardProvider(c Clipboard, logf func(string, ...any)) *clipboardProvider {
	return &clipboardProvider{Clipboard: c, busy: make(chan struct{}, 1), timeout: 2 * time.Second, logf: logf}
}

func (c *clipboardProvider) register(register func(string, any) error) error {
	if err := register("mygonvim.clipboard_get", c.get); err != nil {
		return err
	}
	return register("mygonvim.clipboard_set", c.set)
}

// One outstanding native operation at a time bounds resource use even if a
// native clipboard owner never replies. RPC callers still return on timeout.
func (c *clipboardProvider) call(fn func(context.Context) (string, error)) (string, error) {
	select {
	case c.busy <- struct{}{}:
	default:
		return "", errors.New("clipboard operation still pending")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := fn(ctx)
		<-c.busy
		done <- result{text, err}
	}()
	select {
	case r := <-done:
		return r.text, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *clipboardProvider) get(register string) ([]any, error) {
	text, err := c.call(func(ctx context.Context) (string, error) { return c.Read(ctx) })
	if err != nil {
		c.logf("clipboard read: %v", err)
		return []any{[]string{""}, "v"}, nil
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	regtype := "v"
	if strings.HasSuffix(text, "\n") {
		regtype = "V"
		text = strings.TrimSuffix(text, "\n")
	}
	return []any{strings.Split(text, "\n"), regtype}, nil
}

func (c *clipboardProvider) set(lines []string, regtype, register string) (int, error) {
	// Neovim includes a trailing empty line for linewise and blockwise yanks.
	// Joining the supplied list already preserves the text's final newline.
	text := strings.Join(lines, "\n")
	if runtime.GOOS == "windows" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	_, err := c.call(func(ctx context.Context) (string, error) { return "", c.Write(ctx, text) })
	if err != nil {
		c.logf("clipboard write: %v", err)
	}
	return 0, nil
}

const clipboardSetup = `
local channel = ...
vim.api.nvim_create_autocmd('VimEnter', {once = true, callback = function()
  if vim.g.clipboard ~= nil then return end
  local copy = function(register)
    return function(lines, regtype)
      return vim.rpcrequest(channel, 'mygonvim.clipboard_set', lines, regtype, register)
    end
  end
  local paste = function(register)
    return function() return vim.rpcrequest(channel, 'mygonvim.clipboard_get', register) end
  end
  vim.g.clipboard = {
    name = 'mygonvim', cache_enabled = false,
    copy = {['+'] = copy('+'), ['*'] = copy('*')},
    paste = {['+'] = paste('+'), ['*'] = paste('*')},
  }
  vim.g.loaded_clipboard_provider = nil
  vim.cmd('runtime autoload/provider/clipboard.vim')
end})
`

const copySelection = `
local cut = ...
local mode = vim.fn.mode(1):sub(1, 1)
if mode == 's' or mode == 'S' or mode == '\019' then
  vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes('<C-g>', true, false, true), 'nx', false)
elseif mode ~= 'v' and mode ~= 'V' and mode ~= '\022' then
  return
end
vim.api.nvim_feedkeys('"+' .. (cut and 'd' or 'y'), 'nx', false)
`

// The target is 64 KiB, but never split a line (or a CRLF pair). An individual
// long line may exceed it. The entire stream remains one queue operation.
const pasteChunkSize = 64 << 10

func pasteText(text string, paste func(string, bool, int) (bool, error)) error {
	if text == "" {
		return nil
	}
	phase := -1
	for text != "" {
		n := len(text)
		if n > pasteChunkSize {
			if end := strings.LastIndexByte(text[:pasteChunkSize], '\n'); end >= 0 {
				n = end + 1
			} else if end := strings.IndexByte(text[pasteChunkSize:], '\n'); end >= 0 {
				n = pasteChunkSize + end + 1
			}
		}
		if phase == -1 && n < len(text) {
			phase = 1
		} else if phase != -1 {
			phase = 2
			if n == len(text) {
				phase = 3
			}
		}
		ok, err := paste(text[:n], true, phase)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("paste cancelled by Neovim")
		}
		text = text[n:]
	}
	return nil
}
