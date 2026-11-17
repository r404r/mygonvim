package nvimproc

import (
	"context"
	"errors"
	"fmt"
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
	warn    func(string)
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
		c.failed("read", err)
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
		c.failed("write", err)
	}
	return 0, nil
}

func (c *clipboardProvider) failed(operation string, err error) {
	message := fmt.Sprintf("clipboard %s failed: %v", operation, err)
	c.logf("%s", message)
	if c.warn != nil {
		c.warn(message)
	}
}

const clipboardSetup = `
local channel = ...
vim.api.nvim_create_autocmd('VimEnter', {once = true, callback = function()
  -- Even false is an explicit user choice; only an unset value uses our provider.
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
    -- Read external clipboard changes instead of returning a cached yank.
    name = 'mygonvim', cache_enabled = false,
    copy = {['+'] = copy('+'), ['*'] = copy('*')},
    paste = {['+'] = paste('+'), ['*'] = paste('*')},
  }
  -- Init may already have triggered built-in detection. Reload it with our provider.
  vim.g.loaded_clipboard_provider = nil
  vim.cmd('runtime autoload/provider/clipboard.vim')
end})
`

const copySelection = `
local cut = ...
local mode = vim.fn.mode(1):sub(1, 1)
-- Select uses the same register commands after switching to Visual without mappings.
if mode == 's' or mode == 'S' or mode == '\019' then
  vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes('<C-g>', true, false, true), 'nx', false)
elseif mode ~= 'v' and mode ~= 'V' and mode ~= '\022' then
  -- Copy/Cut never act on a whole line merely because no selection is active.
  return
end
-- Non-remapped execution preserves the selection and ignores custom y/d mappings.
vim.api.nvim_feedkeys('"+' .. (cut and 'd' or 'y'), 'nx', false)
`

// The target is 64 KiB, but never split a line (or a CRLF pair). An individual
// long line may exceed it. The entire stream remains one queue operation.
const pasteChunkSize = 64 << 10

// Bound each request's wait, including a mode change after fast preflight.
// A stream has no total duration limit while its requests keep replying.
const clipboardRequestTimeout = 2 * time.Second

func trackClipboardRequest(ctx context.Context, requests chan<- (<-chan struct{}), call func() (bool, error)) (bool, error) {
	done := make(chan struct{})
	select {
	case requests <- done:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer close(done)
	return call()
}

func waitClipboardRequests(requests <-chan (<-chan struct{}), result <-chan error) (error, bool) {
	for {
		select {
		case err := <-result:
			return err, false
		case done := <-requests:
			// Start a fresh timer for every RPC, not for the whole paste.
			timer := time.NewTimer(clipboardRequestTimeout)
			select {
			case <-done:
				timer.Stop()
			case <-timer.C:
				// Prefer a reply already received at the deadline boundary.
				select {
				case <-done:
				default:
					return context.DeadlineExceeded, true
				}
			}
		}
	}
}

func pasteText(text string, paste func(string, bool, int) (bool, error)) error {
	return pasteTextContext(context.Background(), text, paste)
}

func pasteTextContext(ctx context.Context, text string, paste func(string, bool, int) (bool, error)) (result error) {
	if text == "" {
		return nil
	}
	phase := -1
	streaming := false
	defer func() {
		// A chunk already sent may run after the deadline. Close its stream
		// after it replies, before allowing another clipboard operation.
		if streaming && ctx.Err() != nil {
			_, err := paste("", true, 3)
			result = errors.Join(result, err)
		}
	}()
	for text != "" {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		streaming = phase == 1 || phase == 2
		if err := ctx.Err(); err != nil {
			return err
		}
		text = text[n:]
	}
	return nil
}
