// Package openfiles turns file paths, as a file dialog or a drag and drop
// delivers them, into the Ex command that makes Neovim open them.
package openfiles

import "strings"

// Command returns the Ex command that opens paths in Neovim, or "" when there
// is nothing to open. goos is the operating system the paths come from, as
// runtime.GOOS spells it.
//
// The command is `confirm drop {file} ...`: :drop switches to a window that
// already shows the first file and otherwise edits it in the current window,
// and "confirm" makes Neovim ask about unsaved changes instead of failing with
// an error. With several paths the argument list becomes all of them and the
// first one is shown.
//
// Paths are escaped as Neovim's fnameescape() does, so that spaces, '%', '#',
// '|' and the like reach :drop as part of a file name. Paths should be
// absolute: a name starting with '~' would still be expanded by Neovim.
func Command(goos string, paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		if p == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("confirm drop")
		}
		b.WriteByte(' ')
		b.WriteString(Escape(goos, p))
	}
	return b.String()
}

// Characters that fnameescape() puts a backslash before. On Windows the
// backslash is a path separator, and '$' is left alone, as Neovim does.
// Windows defaults include '[', '{' and '!' in 'isfname', so they are not
// escaped. These static sets do not follow user changes to 'isfname'.
const (
	escUnix    = " \t\n*?[{`$\\%#'\"|!<"
	escWindows = " \t\n*?`%#'\"|<"
)

// Escape is Neovim's fnameescape(): it escapes name for use as a file name
// argument of an Ex command.
func Escape(goos, name string) string {
	set := escUnix
	if goos == "windows" {
		set = escWindows
	}
	var b strings.Builder
	b.Grow(len(name) + 2)
	for i := 0; i < len(name); i++ {
		c := name[i]
		// '+' and '>' at the start would be taken as a command option or
		// as a redirect, and a lone '-' as "the previous directory".
		lead := i == 0 && (c == '+' || c == '>' || (c == '-' && len(name) == 1))
		if lead || strings.IndexByte(set, c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}
