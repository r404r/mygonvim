package grid

// Result tells what a batch of updates changed beyond cells.
type Result struct {
	// Flushed: the batch contained "flush", so the grid is a whole frame.
	Flushed      bool
	TitleChanged bool
	ModeChanged  bool
	Resized      bool
	// Bell: Neovim rang the (visual) bell.
	Bell bool
}

// Apply applies the updates of one "redraw" notification: each update is
// [name, args1, args2, ...] as the msgpack-rpc client decodes it. It does not
// panic on malformed data; what it cannot read it skips.
//
// Only grid 1 is drawn (ext_multigrid is off); events for other grids are
// ignored.
func (m *Model) Apply(updates ...[]any) Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res Result
	s := &m.s
	for _, u := range updates {
		if len(u) == 0 {
			continue
		}
		name, _ := u[0].(string)
		for _, a := range u[1:] {
			args, _ := a.([]any)
			s.event(name, args, &res)
		}
		// Events without arguments (flush, busy_start, ...) arrive as a
		// bare [name] or [name, []].
		if len(u) == 1 {
			s.event(name, nil, &res)
		}
	}
	return res
}

func (s *State) event(name string, args []any, res *Result) {
	switch name {
	case "grid_resize":
		if len(args) < 3 || toInt(args[0], 1) != 1 {
			return
		}
		s.resize(toInt(args[1], 1), toInt(args[2], 1))
		res.Resized = true
	case "grid_clear":
		if len(args) < 1 || toInt(args[0], 1) != 1 {
			return
		}
		s.clear()
	case "grid_cursor_goto":
		if len(args) < 3 || toInt(args[0], 1) != 1 {
			return
		}
		s.CurRow, s.CurCol = toInt(args[1], 0), toInt(args[2], 0)
	case "grid_line":
		s.gridLine(args)
	case "grid_scroll":
		s.gridScroll(args)
	case "default_colors_set":
		if len(args) < 3 {
			return
		}
		// -1 means "not set"; keep a usable color then.
		if v := toInt(args[0], -1); v >= 0 {
			s.DefFg = v
		}
		if v := toInt(args[1], -1); v >= 0 {
			s.DefBg = v
		}
		if v := toInt(args[2], -1); v >= 0 {
			s.DefSp = v
		}
	case "hl_attr_define":
		if len(args) < 2 {
			return
		}
		s.Attrs[toInt(args[0], 0)] = parseAttr(args[1])
	case "mode_info_set":
		s.modeInfoSet(args)
	case "mode_change":
		if len(args) < 2 {
			return
		}
		mode, _ := args[0].(string)
		s.Mode, s.ModeIdx = mode, toInt(args[1], 0)
		res.ModeChanged = true
	case "set_title":
		if len(args) < 1 {
			return
		}
		t, _ := args[0].(string)
		if t != s.Title {
			s.Title = t
			res.TitleChanged = true
		}
	case "busy_start":
		s.Busy = true
	case "busy_stop":
		s.Busy = false
	case "bell", "visual_bell":
		res.Bell = true
	case "flush":
		s.Flushes++
		res.Flushed = true
	}
}

func (s *State) gridLine(args []any) {
	if len(args) < 4 || toInt(args[0], 1) != 1 {
		return
	}
	row, col := toInt(args[1], -1), toInt(args[2], -1)
	cells, _ := args[3].([]any)
	if row < 0 || row >= s.Rows || col < 0 {
		return
	}
	hl := 0
	for _, c := range cells {
		cc, _ := c.([]any)
		if len(cc) == 0 {
			continue
		}
		text, _ := cc[0].(string)
		if len(cc) > 1 && cc[1] != nil {
			hl = toInt(cc[1], hl)
		}
		rep := 1
		if len(cc) > 2 {
			rep = toInt(cc[2], 1)
		}
		for i := 0; i < rep; i++ {
			if col >= s.Cols {
				return
			}
			s.cells[row*s.Cols+col] = Cell{Text: text, Hl: hl}
			col++
		}
	}
}

// gridScroll moves the region [top,bot) x [left,right) by rows: positive rows
// scroll the content up (line top+rows moves to top), negative down. The rows
// that scrolled in are left as they are: Neovim redraws them with grid_line.
func (s *State) gridScroll(args []any) {
	if len(args) < 6 || toInt(args[0], 1) != 1 {
		return
	}
	top, bot := toInt(args[1], 0), toInt(args[2], 0)
	left, right := toInt(args[3], 0), toInt(args[4], 0)
	rows := toInt(args[5], 0)
	top, left = max(top, 0), max(left, 0)
	bot, right = min(bot, s.Rows), min(right, s.Cols)
	if rows == 0 || top >= bot || left >= right {
		return
	}
	row := func(r int) []Cell { return s.cells[r*s.Cols+left : r*s.Cols+right] }
	if rows > 0 {
		for r := top; r < bot-rows; r++ {
			copy(row(r), row(r+rows))
		}
	} else {
		for r := bot - 1; r >= top-rows; r-- {
			copy(row(r), row(r+rows))
		}
	}
}

func (s *State) modeInfoSet(args []any) {
	if len(args) < 2 {
		return
	}
	s.CursorStyleEnabled, _ = args[0].(bool)
	list, _ := args[1].([]any)
	modes := make([]ModeInfo, 0, len(list))
	for _, e := range list {
		d, _ := e.(map[string]any)
		mi := ModeInfo{CellPercentage: 100}
		mi.Name, _ = d["name"].(string)
		switch d["cursor_shape"] {
		case "vertical":
			mi.Shape = CursorVertical
		case "horizontal":
			mi.Shape = CursorHorizontal
		}
		if v, ok := d["cell_percentage"]; ok {
			if p := toInt(v, 100); p > 0 {
				mi.CellPercentage = min(p, 100)
			}
		}
		mi.AttrID = toInt(d["attr_id"], 0)
		modes = append(modes, mi)
	}
	s.Modes = modes
}

var flagKeys = map[string]AttrFlags{
	"reverse": Reverse, "bold": Bold, "italic": Italic, "strikethrough": Strikethrough,
	"underline": Underline, "undercurl": Undercurl, "underdouble": Underdouble,
	"underdotted": Underdotted, "underdashed": Underdashed,
}

func parseAttr(v any) Attr {
	a := Attr{Fg: Unset, Bg: Unset, Sp: Unset}
	d, _ := v.(map[string]any)
	for k, val := range d {
		switch k {
		case "foreground":
			a.Fg = toInt(val, Unset)
		case "background":
			a.Bg = toInt(val, Unset)
		case "special":
			a.Sp = toInt(val, Unset)
		case "blend":
			a.Blend = toInt(val, 0)
		default:
			if f, ok := flagKeys[k]; ok {
				if b, isBool := val.(bool); !isBool || b {
					a.Flags |= f
				}
			}
		}
	}
	return a
}

// toInt reads a number msgpack decoded as any integer or float type.
func toInt(v any, def int) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}
