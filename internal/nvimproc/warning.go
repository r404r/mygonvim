package nvimproc

// warn never waits on Neovim's main loop or holds up queued recovery input.
// In particular, provider callbacks may be serving a nested rpcrequest.
func (s *Session) warn(message string) {
	go func() {
		if err := s.nv.ExecLua(clipboardWarning, nil, "MyGoNVim: "+message); err != nil {
			s.logf("clipboard warning failed: %v", err)
		}
	}()
}

const clipboardWarning = `
local message = ...
vim.schedule(function()
  -- Control characters would wrap the message or create a hit-enter prompt.
  message = message:gsub('%c', ' ')
  local space = math.max(0, vim.v.echospace)
  message = vim.fn.strcharpart(message, 0, space)
  while vim.fn.strdisplaywidth(message) > space do
    message = vim.fn.strcharpart(message, 0, vim.fn.strchars(message) - 1)
  end
  if message ~= '' then
    vim.api.nvim_echo({{message, 'WarningMsg'}}, true, {})
  end
end)
`
