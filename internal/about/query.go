package about

import (
	"context"
	"errors"
	"sync/atomic"
)

// Each field is isolated so one unsupported stdpath does not discard the rest.
const queryLua = `
local out = {version = '', paths = vim.empty_dict(), errors = vim.empty_dict()}
local ok, v = pcall(vim.version)
if ok then
 out.version = string.format('%d.%d.%d', v.major, v.minor, v.patch)
 if v.prerelease then out.version = out.version .. '-dev' end
end
for _, name in ipairs({'config', 'data', 'state', 'cache'}) do
 local success, value = pcall(vim.fn.stdpath, name)
 if success and type(value) == 'string' then out.paths[name] = value
 else out.errors[name] = tostring(value) end
end
return out
`

type LuaClient interface {
	ExecLua(string, interface{}, ...interface{}) error
}

// Query keeps at most one request outstanding, even across timeouts/reopens.
// The RPC itself finishes when Neovim responds or its connection closes.
type Query struct {
	Client LuaClient
	busy   atomic.Bool
}

func (q *Query) Load(ctx context.Context) (Info, error) {
	if !q.busy.CompareAndSwap(false, true) {
		return Info{}, errors.New("a Neovim information request is still pending")
	}
	type result struct {
		info Info
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var info Info
		err := q.Client.ExecLua(queryLua, &info)
		q.busy.Store(false)
		done <- result{info, err}
	}()
	select {
	case r := <-done:
		return r.info, r.err
	case <-ctx.Done():
		return Info{}, ctx.Err()
	}
}
