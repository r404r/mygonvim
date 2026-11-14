package gui

import "runtime"

// DefaultFontFamily is the font list the editor draws with unless told
// otherwise: the platform's usual monospace font, then the generic family. Chinese,
// Japanese and emoji fall back to what the system's text engine finds.
var DefaultFontFamily = defaultFont(runtime.GOOS)

func defaultFont(goos string) string {
	switch goos {
	case "windows":
		return "Cascadia Mono, Consolas, monospace"
	case "darwin":
		return "SF Mono, Menlo, monospace"
	}
	return "monospace"
}
