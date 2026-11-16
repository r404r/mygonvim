package main

import (
	"sync/atomic"

	"github.com/egoist/mygo"
)

// closeHandler keeps the window available for Neovim's unsaved-buffer prompt.
func closeHandler(quitting *atomic.Bool, command func(string)) func(*mygo.CloseEvent) {
	return func(e *mygo.CloseEvent) {
		if quitting.Load() {
			return
		}
		e.PreventDefault()
		command("confirm qa")
	}
}

func exitHandler(quitting *atomic.Bool, quit func(), logf func(string, ...any)) func(error) {
	return func(err error) {
		quitting.Store(true)
		if err != nil {
			logf("nvim exited: %v", err)
		}
		quit()
	}
}
