package about

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type testWindow struct {
	shown, focused, restored, invalidated int
	closed                                int
	close                                 func()
}

func (w *testWindow) Show()       { w.shown++ }
func (w *testWindow) Focus()      { w.focused++ }
func (w *testWindow) Restore()    { w.restored++ }
func (w *testWindow) Invalidate() { w.invalidated++ }
func (w *testWindow) Close() {
	w.closed++
	if w.close != nil {
		w.close()
	}
}

func nextUI(t *testing.T, queue <-chan func()) {
	t.Helper()
	select {
	case f := <-queue:
		f()
	case <-time.After(3 * time.Second):
		t.Fatal("UI update never arrived")
	}
}

func TestWindowEscapeClosesAndReopens(t *testing.T) {
	for _, fluent := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "fluent"}[fluent], func(t *testing.T) {
			updates := make(chan func(), 2)
			var windows []*testWindow
			var view func(*ui.Context)
			c := &Controller{Fluent: fluent, Dispatch: func(f func()) { updates <- f },
				Query: func(context.Context) (Info, error) { return Info{Version: "0.12.6"}, nil },
				NewWindow: func(v func(*ui.Context), closed func()) Window {
					view = v
					w := &testWindow{close: closed}
					windows = append(windows, w)
					return w
				},
			}
			c.Open()
			nextUI(t, updates)
			tt := ui.NewTester(view, 720, 780)
			tt.Frame()
			for _, key := range []struct {
				mods ui.Modifiers
				key  ui.Key
			}{{0, ui.KeyF1}, {ui.Ctrl, ui.KeyW}, {ui.Cmd, ui.KeyW}, {ui.Shift, ui.KeyEscape}} {
				tt.Key(key.mods, key.key)
			}
			tt.Frame()
			if windows[0].closed != 0 || c.window != windows[0] {
				t.Fatal("a key other than unmodified Escape closed About")
			}
			// A button holding keyboard focus must not hide the window shortcut.
			tt.Key(0, ui.KeyTab)
			tt.Key(0, ui.KeyEscape)
			if windows[0].closed != 1 || c.window != nil || c.model != nil {
				t.Fatal("Escape did not close the window and clear About state")
			}
			c.Open()
			nextUI(t, updates)
			if len(windows) != 2 || c.window != windows[1] || windows[1] == windows[0] {
				t.Fatal("reopening About did not create a new window")
			}
			windows[1].Close()
		})
	}
}

func TestWindowEscapeWhileLoadingDiscardsLateQuery(t *testing.T) {
	for _, reopen := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "reopened"}[reopen], func(t *testing.T) {
			updates := make(chan func(), 2)
			requests := make(chan chan Info, 2)
			var windows []*testWindow
			var view func(*ui.Context)
			c := &Controller{Dispatch: func(f func()) { updates <- f },
				Query: func(ctx context.Context) (Info, error) {
					reply := make(chan Info, 1)
					requests <- reply
					select {
					case info := <-reply:
						return info, nil
					case <-ctx.Done():
						return Info{}, ctx.Err()
					}
				},
				NewWindow: func(v func(*ui.Context), closed func()) Window {
					view = v
					w := &testWindow{close: closed}
					windows = append(windows, w)
					return w
				},
			}
			nextRequest := func() chan Info {
				t.Helper()
				select {
				case reply := <-requests:
					return reply
				case <-time.After(3 * time.Second):
					t.Fatal("About query never started")
					return nil
				}
			}
			c.Open()
			oldReply := nextRequest()
			oldModel := c.model
			oldPaths := append([]Path(nil), oldModel.paths...)
			tt := ui.NewTester(view, 720, 780)
			tt.Frame()
			if !tt.HasText("Neovim: Loading…") {
				t.Fatal("query was not pending when Escape was pressed")
			}
			tt.Key(0, ui.KeyEscape)
			if windows[0].closed != 1 || c.window != nil || c.model != nil {
				t.Fatal("Escape did not close About while loading")
			}
			var newReply chan Info
			if reopen {
				c.Open()
				newReply = nextRequest()
			}
			oldReply <- Info{Version: "stale"}
			nextUI(t, updates)
			if windows[0].invalidated != 0 || oldModel.nvim != "Loading…" || !reflect.DeepEqual(oldModel.paths, oldPaths) {
				t.Fatal("late query updated the closed window")
			}
			if reopen {
				if c.model == oldModel || c.model.nvim != "Loading…" || windows[1].invalidated != 0 {
					t.Fatal("old query updated the new window")
				}
				newReply <- Info{Version: "current"}
				nextUI(t, updates)
				if c.model.nvim != "current" || windows[1].invalidated != 1 {
					t.Fatal("new window did not receive its own query result")
				}
				windows[1].Close()
			} else if c.window != nil || c.model != nil {
				t.Fatal("late query restored closed About state")
			}
		})
	}
}

func TestWindowOpensBeforeQueryAndReusesExistingWindow(t *testing.T) {
	updates := make(chan func(), 8)
	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	windows := []*testWindow{}
	var view func(*ui.Context)
	c := &Controller{Process: Process{Version: "dev"}, Dispatch: func(f func()) { updates <- f },
		Query: func(context.Context) (Info, error) {
			entered <- struct{}{}
			<-release
			return Info{Version: "0.12.6"}, nil
		},
		NewWindow: func(v func(*ui.Context), closed func()) Window {
			view = v
			w := &testWindow{close: closed}
			windows = append(windows, w)
			return w
		},
	}
	c.Open()
	if len(windows) != 1 {
		t.Fatal("window did not open immediately")
	}
	tt := ui.NewTester(view, 720, 780)
	tt.Frame()
	if !tt.HasText("Neovim: Loading…") {
		t.Fatal("no loading state")
	}
	<-entered
	c.Open()
	if len(windows) != 1 || windows[0].shown != 1 || windows[0].focused != 1 || windows[0].restored != 1 {
		t.Fatalf("did not reuse window: %+v", windows)
	}
	windows[0].close()
	if c.window != nil {
		t.Fatal("closed window retained")
	}
	c.Open()
	if len(windows) != 2 {
		t.Fatal("could not reopen")
	}
	<-entered
	close(release)
	nextUI(t, updates)
	nextUI(t, updates)
	if windows[0].invalidated != 0 || windows[1].invalidated != 1 {
		t.Fatal("late reply changed a closed window")
	}
	if c.model.nvim != "0.12.6" {
		t.Fatal(c.model.nvim)
	}
	// Closing About discards only its own state; no editor backend or quit callback is involved.
	windows[1].close()
	if c.window != nil || c.model != nil {
		t.Fatal("About state survived close")
	}
}

func TestQueryFailureStillShowsWindowAndUnavailableFields(t *testing.T) {
	updates := make(chan func(), 1)
	var view func(*ui.Context)
	c := &Controller{Process: Process{Version: "dev"}, Dispatch: func(f func()) { updates <- f },
		Query:     func(context.Context) (Info, error) { return Info{}, errors.New("disconnected") },
		NewWindow: func(v func(*ui.Context), closed func()) Window { view = v; return &testWindow{} },
	}
	c.Open()
	nextUI(t, updates)
	tt := ui.NewTester(view, 720, 780)
	tt.Frame()
	if !tt.HasText("Neovim: Unavailable: disconnected") {
		t.Fatal("query failure hidden")
	}
	for _, p := range c.model.paths[3:] {
		if p.Value != "" || p.Error != "disconnected" {
			t.Fatalf("path=%+v", p)
		}
	}
}

func TestLoginEnvironmentStatusUpdatesWhileQueryIsPending(t *testing.T) {
	updates := make(chan func(), 1)
	release := make(chan struct{})
	var view func(*ui.Context)
	c := &Controller{Process: Process{LoginEnv: "Pending"}, Dispatch: func(f func()) { updates <- f },
		Query:     func(context.Context) (Info, error) { <-release; return Info{Version: "0.12.6"}, nil },
		NewWindow: func(v func(*ui.Context), _ func()) Window { view = v; return &testWindow{} },
	}
	c.Open()
	tt := ui.NewTester(view, 720, 780)
	tt.Frame()
	if !tt.HasText("Login shell environment: Pending") {
		t.Fatal("pending status missing")
	}
	c.UpdateProcess(Process{LoginEnv: "Imported", Nvim: "/selected/nvim"})
	tt.Frame()
	if !tt.HasText("Login shell environment: Imported") || !tt.HasText("/selected/nvim") {
		t.Fatal("completion did not update open About")
	}
	close(release)
	nextUI(t, updates)
	tt.Frame()
	if !tt.HasText("Login shell environment: Imported") || !tt.HasText("/selected/nvim") {
		t.Fatal("old query snapshot overwrote startup result")
	}
	c.UpdateProcess(Process{LoginEnv: "Fallback: timeout"})
	tt.Frame()
	if !tt.HasText("Login shell environment: Fallback: timeout") {
		t.Fatal("fallback status missing")
	}
}

func TestViewFieldsPortableModeLongPathsAndLinks(t *testing.T) {
	for _, portable := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "portable"}[portable], func(t *testing.T) {
			updates := make(chan func(), 2)
			links := make(chan string, 1)
			long := strings.Repeat("long-directory/", 40) + "settings.json"
			m := &viewModel{process: Process{Version: "0.0.4", Portable: portable}, nvim: "0.12.6", paths: []Path{{Label: "settings.json", Value: long}}, dispatch: func(f func()) { updates <- f }, invalidate: func() {}, services: Services{OpenExternal: func(url string) error { links <- url; return nil }}}
			tt := ui.NewTester(m.view, 520, 700)
			tt.Frame()
			for _, text := range []string{"MyGoNVim 0.0.4", "Author: r404r", "MIT · Copyright (c) 2026 r404r", Website, "Neovim: 0.12.6", long, "Copy", "Show in folder", "Open directory"} {
				if !tt.HasText(text) {
					t.Errorf("missing %q", text)
				}
			}
			expected := "No"
			if portable {
				expected = "Yes"
			}
			if !tt.HasText("Portable mode: " + expected) {
				t.Fatal("wrong portable value")
			}
			rect, ok := tt.Find(long)
			if !ok || rect.H < 28 {
				t.Fatalf("long path did not wrap: %+v", rect)
			}
			for _, tc := range []struct{ button, url string }{{"GitHub", Website}, {"License", LicenseURL}} {
				if err := tt.Click(tc.button); err != nil {
					t.Fatal(err)
				}
				nextUI(t, updates)
				if got := <-links; got != tc.url {
					t.Fatalf("URL=%q", got)
				}
			}
		})
	}
}

func TestWindowQueryDeadlineAndVisibleFeedback(t *testing.T) {
	updates := make(chan func(), 1)
	c := &Controller{Dispatch: func(f func()) { updates <- f },
		Query:     func(ctx context.Context) (Info, error) { <-ctx.Done(); return Info{}, ctx.Err() },
		NewWindow: func(func(*ui.Context), func()) Window { return &testWindow{} },
	}
	c.Open()
	if c.window == nil || c.model.nvim != "Loading…" {
		t.Fatal("window waits for query")
	}
	nextUI(t, updates)
	if !strings.Contains(c.model.nvim, "Unavailable: context deadline exceeded") {
		t.Fatal(c.model.nvim)
	}
}

func TestViewPathButtonsAndLinkErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	updates := make(chan func(), 2)
	calls := make(chan string, 1)
	m := &viewModel{paths: []Path{{Label: "settings.json", Value: path}}, dispatch: func(f func()) { updates <- f }, invalidate: func() {}, services: Services{
		Stat:          os.Stat,
		Copy:          func(p string) error { calls <- "copy:" + p; return nil },
		Reveal:        func(p string) { calls <- "reveal:" + p },
		OpenDirectory: func(p string) error { calls <- "open:" + p; return nil },
		OpenExternal:  func(string) error { return errors.New("no browser") },
	}}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(m.view, 720, 780)
	tt.Frame()
	for _, tc := range []struct{ button, want, status string }{
		{"Copy", "copy:" + path, "Copied path."},
		{"Show in folder", "reveal:" + path, "Requested file manager to show the location."},
		{"Open directory", "open:" + dir, "Opened directory."},
	} {
		if err := tt.Click(tc.button); err != nil {
			t.Fatal(err)
		}
		nextUI(t, updates)
		if got := <-calls; got != tc.want {
			t.Fatalf("%s = %q", tc.button, got)
		}
		tt.Frame()
		if !tt.HasText(tc.status) {
			t.Fatalf("no feedback for %s", tc.button)
		}
	}
	if err := tt.Click("GitHub"); err != nil {
		t.Fatal(err)
	}
	nextUI(t, updates)
	tt.Frame()
	if !tt.HasText("Open link failed: no browser") {
		t.Fatal("link failure hidden")
	}
}
