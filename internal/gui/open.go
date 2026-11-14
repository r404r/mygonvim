package gui

import (
	"runtime"

	"github.com/r404r/mygonvim/internal/openfiles"
)

// OpenFiles asks Neovim to open files, as dropped on the window or chosen in a
// file dialog: the first is shown, and when there are several they all go to
// the argument list. Unsaved changes in the current buffer make Neovim ask on
// its command line. It does nothing for an empty list. Any goroutine may call
// it: the backend only queues the command.
func (e *Editor) OpenFiles(paths []string) {
	if cmd := openfiles.Command(runtime.GOOS, paths); cmd != "" {
		e.be.Command(cmd)
	}
}
