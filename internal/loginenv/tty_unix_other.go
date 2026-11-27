//go:build unix && !darwin

package loginenv

// The desktop heuristic is macOS-only; other Unix systems require the switch.
func StdoutTTY() bool { return false }
