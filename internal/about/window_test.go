package about

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type testWindow struct {
	shown, focused, restored, invalidated int
	close                                 func()
}

func (w *testWindow) Show()       { w.shown++ }
func (w *testWindow) Focus()      { w.focused++ }
func (w *testWindow) Restore()    { w.restored++ }
func (w *testWindow) Invalidate() { w.invalidated++ }

func nextUI(t *testing.T, queue <-chan func()) {
	t.Helper()
	select {
	case f := <-queue:
		f()
	case <-time.After(3 * time.Second):
		t.Fatal("UI update never arrived")
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
