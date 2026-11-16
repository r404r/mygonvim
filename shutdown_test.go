package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/r404r/mygonvim/internal/appmenu"
	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

// The application quit callback is a stand-in for MyGo's native window walk.
// Native App.Quit routing and OS gestures still require a desktop smoke test.
func TestCloseRoutesKeepWindowUntilExit(t *testing.T) {
	for _, route := range []string{"window", "file-exit-windows", "file-exit-linux", "app-quit"} {
		t.Run(route, func(t *testing.T) {
			var quitting atomic.Bool
			var commands []string
			closed := false
			handler := closeHandler(&quitting, func(cmd string) { commands = append(commands, cmd) })
			quit := func() {
				e := &mygo.CloseEvent{}
				handler(e)
				if !e.DefaultPrevented() {
					closed = true
				}
			}
			switch route {
			case "window", "app-quit":
				quit()
			default:
				goos := strings.TrimPrefix(route, "file-exit-")
				menu := mygo.NewMenu(appmenu.Template(goos, appmenu.Actions{Quit: quit}))
				item := menu.ItemByID(appmenu.IDExit)
				if item == nil || item.Click == nil {
					t.Fatal("missing Exit action")
				}
				item.Click(item, nil)
			}
			if closed || !slices.Equal(commands, []string{"confirm qa"}) {
				t.Fatalf("before exit: closed=%v commands=%q", closed, commands)
			}
			exitHandler(&quitting, quit, t.Logf)(nil)
			if !closed || !quitting.Load() || len(commands) != 1 {
				t.Fatalf("after exit: closed=%v quitting=%v commands=%q", closed, quitting.Load(), commands)
			}
		})
	}
}

func waitForShutdown(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestCloseConfirmationAgainstRealNvim(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	for _, modified := range []bool{false, true} {
		name := "clean"
		if modified {
			name = "unsaved"
		}
		t.Run(name, func(t *testing.T) {
			var quitting, closed atomic.Bool
			model := grid.New(80, 24)
			exited := make(chan error, 1)
			s, err := nvimproc.Start(nvimproc.Options{
				Bin: bin, Args: []string{"--clean"}, Cols: 80, Rows: 24,
				Env:      nvimproc.PortableEnv(os.Environ(), t.TempDir()),
				OnRedraw: func(updates ...[]any) { model.Apply(updates...) },
				OnExit: func(err error) {
					exitHandler(&quitting, func() {
						e := &mygo.CloseEvent{}
						closeHandler(&quitting, func(string) { t.Error("exit tried another quit command") })(e)
						closed.Store(!e.DefaultPrevented())
					}, t.Logf)(err)
					exited <- err
				},
				Logf: t.Logf,
			})
			if err != nil {
				t.Fatal(err)
			}
			handler := closeHandler(&quitting, s.Command)
			defer s.Close()
			if err := s.Nvim().SetBufferName(0, filepath.Join(t.TempDir(), "notes.txt")); err != nil {
				t.Fatal(err)
			}
			if modified {
				if err := s.Nvim().SetBufferLines(0, 0, -1, true, [][]byte{[]byte("unsaved text")}); err != nil {
					t.Fatal(err)
				}
			}
			e := &mygo.CloseEvent{}
			handler(e)
			if !e.DefaultPrevented() || closed.Load() {
				t.Fatal("window closed before Neovim exited")
			}
			if modified {
				screen := func() (text string) {
					model.Read(func(st *grid.State) {
						for row := 0; row < st.Rows; row++ {
							text += st.RowText(row) + "\n"
						}
					})
					return
				}
				waitForShutdown(t, "Save changes prompt", func() bool { return strings.Contains(screen(), "Save changes") })
				select {
				case <-s.Done():
					t.Fatal("Neovim exited with unanswered prompt")
				default:
				}
				if closed.Load() {
					t.Fatal("window closed during prompt")
				}
				s.Input("c")
				waitForShutdown(t, "cancel to dismiss prompt", func() bool { return !strings.Contains(screen(), "Save changes") })
				if closed.Load() {
					t.Fatal("cancel closed the window")
				}
				var result int
				if err := s.Nvim().Eval("6 * 7", &result); err != nil || result != 42 {
					t.Fatalf("session unusable after cancel: %d, %v", result, err)
				}
				s.Input("A continues<Esc>")
				waitForShutdown(t, "editing after cancel", func() bool { return strings.Contains(screen(), "unsaved text continues") })
				// Ask again, then choose No (discard).
				handler(&mygo.CloseEvent{})
				waitForShutdown(t, "second prompt", func() bool { return strings.Contains(screen(), "Save changes") })
				s.Input("n")
			}
			select {
			case err := <-exited:
				if err != nil {
					t.Fatalf("exit: %v", err)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("Neovim did not exit")
			}
			if !closed.Load() || !quitting.Load() {
				t.Fatal("Neovim exited without permitting window close")
			}
		})
	}
}
