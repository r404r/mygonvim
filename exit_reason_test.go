package main

import (
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
)

func TestExitReason(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"exit status 0xc0000374", "heap corruption (0xC0000374)"},
		{"exit status 0xC0000005", "access violation (0xC0000005)"},
		{"exit status 0xc0000409", "stack buffer overrun (0xC0000409)"},
		{"exit status 0xc00000fd", "stack overflow (0xC00000FD)"},
		{"exit status 0xc000001d", "illegal instruction (0xC000001D)"},
		{"exit status 3221226356", "heap corruption (0xC0000374)"},
		{"exit status -1073740940", "heap corruption (0xC0000374)"},
		{"exit status 0xc0000374: plugin failed\r\nlast error", "heap corruption (0xC0000374): plugin failed  last error"},
		{"exit status 7: plugin failed\nlast error", "exit status 7: plugin failed last error"},
		{"exit status 0xdeadbeef", "exit status 0xdeadbeef"},
		{"exit status 1", "exit status 1"},
		{"signal: killed", "signal: killed"},
		{"exit status unknown", "exit status unknown"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := exitReason(tc.input); got != tc.want {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExitLogsBeforeQuit(t *testing.T) {
	for _, err := range []error{nil, errors.New("exit status 0xc0000374: stderr tail\nnext")} {
		t.Run(fmt.Sprint(err), func(t *testing.T) {
			var quitting atomic.Bool
			var events []string
			exitHandler(&quitting, func() {
				if !quitting.Load() {
					t.Error("quit without allowing the window to close")
				}
				events = append(events, "quit")
			}, func(format string, args ...any) {
				events = append(events, fmt.Sprintf(format, args...))
			})(err)
			want := "nvim exited cleanly"
			if err != nil {
				want = "nvim exited: heap corruption (0xC0000374): stderr tail next"
			}
			if !slices.Equal(events, []string{want, "quit"}) {
				t.Fatalf("events = %q", events)
			}
		})
	}
}
