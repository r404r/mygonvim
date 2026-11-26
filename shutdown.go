package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

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
			logf("nvim exited: %s", exitReason(err.Error()))
		} else {
			logf("nvim exited cleanly")
		}
		quit()
	}
}

// exitReason recognizes Windows status codes on every platform and keeps any
// stderr tail on the same log line. Unknown errors retain their original text.
func exitReason(message string) string {
	if rest, ok := strings.CutPrefix(message, "exit status "); ok {
		end := strings.IndexAny(rest, ": \t\r\n")
		if end < 0 {
			end = len(rest)
		}
		token := rest[:end]
		var code uint32
		if n, err := strconv.ParseUint(token, 0, 32); err == nil {
			code = uint32(n)
		} else if n, err := strconv.ParseInt(token, 0, 32); err == nil {
			code = uint32(n)
		}
		reason := map[uint32]string{
			0xC0000374: "heap corruption",
			0xC0000005: "access violation",
			0xC0000409: "stack buffer overrun",
			0xC00000FD: "stack overflow",
			0xC000001D: "illegal instruction",
		}[code]
		if reason != "" {
			message = fmt.Sprintf("%s (0x%08X)%s", reason, code, rest[end:])
		}
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
}
