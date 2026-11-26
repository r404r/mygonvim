// Package winlayout classifies single-grid separator cells from window geometry.
package winlayout

// Cursor describes the system pointer wanted at a grid cell.
type Cursor uint8

const (
	Default Cursor = iota
	Column
	Row
)

// Window uses zero-based screen coordinates. Height excludes the winbar.
// Floating windows describe their entire outer rectangle, including borders.
type Window struct {
	Row, Col, Width, Height, Winbar int
	Floating, StatusConnected       bool
}

// Snapshot is immutable after publication. Its zero value has no geometry.
type Snapshot struct {
	Cols, Rows, CmdHeight int
	GlobalStatus          bool
	Windows               []Window
}

// At follows mouse.c's on_sep_line / on_status_line cells and the
// stl_connected corner rule. It never examines rendered characters.
func (s *Snapshot) At(row, col int) Cursor {
	if s == nil || row < 0 || col < 0 || col >= s.Cols || row >= s.Rows-s.CmdHeight {
		return Default
	}
	if s.GlobalStatus && row >= s.Rows-s.CmdHeight-1 {
		return Default
	}
	for _, w := range s.Windows {
		if w.Floating && row >= w.Row && row < w.Row+w.Height && col >= w.Col && col < w.Col+w.Width {
			return Default
		}
	}
	for _, w := range s.Windows {
		if w.Floating || row < w.Row+w.Winbar || col < w.Col {
			continue
		}
		bottom, right := w.Row+w.Winbar+w.Height, w.Col+w.Width
		if row > bottom || col > right {
			continue
		}
		status, sep := row == bottom, col == right && right < s.Cols
		if status && sep {
			if w.StatusConnected {
				sep = false
			} else {
				status = false
			}
		}
		if status {
			return Row
		}
		if sep {
			return Column
		}
	}
	return Default
}
