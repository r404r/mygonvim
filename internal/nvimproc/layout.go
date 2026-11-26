package nvimproc

import "github.com/r404r/mygonvim/internal/winlayout"

// All API calls run together on Neovim's main loop. winlayout supplies the
// same ancestor walk as stl_connected in statusline.c, including T junctions.
const windowLayout = `
local connected = {}
local function walk(node, right)
  if node[1] == 'leaf' then
    connected[node[2]] = right
    return
  end
  local children = node[2]
  for i, child in ipairs(children) do
    local next_right = right
    if i < #children then next_right = node[1] == 'row' end
    walk(child, next_right)
  end
end
walk(vim.fn.winlayout(), false)
local result = {Cols=vim.o.columns, Rows=vim.o.lines, CmdHeight=vim.o.cmdheight,
                GlobalStatus=vim.o.laststatus == 3, Windows={}}
for _, id in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
  local cfg = vim.api.nvim_win_get_config(id)
  if not cfg.external and not cfg.hide then
    local info = vim.fn.getwininfo(id)[1]
    local w = {Row=info.winrow-1, Col=info.wincol-1,
               Width=info.width, Height=info.height, Winbar=info.winbar,
               Floating=cfg.relative ~= '', StatusConnected=connected[id] or false}
    if w.Floating then
      local b = cfg.border or {}
      local function edge(i)
        local v = b[i]
        if type(v) == 'table' then v = v[1] end
        return v and v ~= '' and 1 or 0
      end
      w.Width = w.Width + edge(4) + edge(8)
      w.Height = w.Height + w.Winbar + edge(2) + edge(6)
      w.Winbar = 0
    end
    table.insert(result.Windows, w)
  end
end
return result
`

func (s *Session) requestLayout() {
	select {
	case s.layoutRefresh <- struct{}{}:
	default:
	}
}

// refreshLayouts has at most one RPC in flight and one coalesced refresh.
// Keeping it separate from work lets recovery keys pass a pending Lua call.
func (s *Session) refreshLayouts(publish func(*winlayout.Snapshot)) {
	for {
		select {
		case <-s.done:
			return
		case <-s.layoutRefresh:
			select {
			case <-s.done:
				return
			default:
			}
			var snapshot winlayout.Snapshot
			err := s.nv.ExecLua(windowLayout, &snapshot)
			select {
			case <-s.done:
				return
			default:
			}
			if err != nil {
				publish(nil)
				continue
			}
			publish(&snapshot)
		}
	}
}
