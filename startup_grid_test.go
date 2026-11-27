package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/gui"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

func TestStartupGridUsesWindowSize(t *testing.T) {
	for _, before := range []bool{false, true} {
		name := "worker-before-frame"
		if before {
			name = "frames-before-worker"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			late := &lateBackend{}
			ed := gui.NewEditor(late, gui.Config{FontFamily: "monospace", FontSize: 14, Cols: 100, Rows: 32}, gui.Hooks{})
			var mu sync.Mutex
			var sizes [][2]int
			intro := make(chan [2]int, 1)
			onRedraw := func(updates ...[]any) {
				// Apply each update separately to capture exactly the flushed grid.
				for _, u := range updates {
					ed.OnRedraw(u)
					if len(u) > 0 && u[0] == "grid_resize" {
						c, r := ed.Model.Size()
						mu.Lock()
						sizes = append(sizes, [2]int{c, r})
						mu.Unlock()
					}
					if len(u) > 0 && u[0] == "flush" {
						ed.Model.Read(func(s *grid.State) {
							for row := 0; row < s.Rows; row++ {
								if strings.Contains(s.RowText(row), "NVIM v") {
									select {
									case intro <- [2]int{s.Cols, s.Rows}:
									default:
									}
									break
								}
							}
						})
					}
				}
			}
			frame := func() {
				if before {
					tt := ui.NewTester(ed.View, 960, 640)
					tt.SetSize(1440, 900)
					tt.Frame()
				} else {
					ui.NewTester(ed.View, 1440, 900).Frame()
				}
			}
			if before {
				frame()
			}
			type result struct {
				sess *nvimproc.Session
				err  error
			}
			started := make(chan result, 1)
			bin := testNvimBin(t)
			go func() {
				sess, err := late.startSession(ctx, nvimproc.Options{Bin: bin, Args: []string{"--clean", "-n"}, OnRedraw: onRedraw})
				started <- result{sess, err}
			}()
			if !before {
				frame()
			}
			out := <-started
			if out.err != nil {
				t.Fatal(out.err)
			}
			defer out.sess.Close()
			if !late.attach(out.sess) {
				t.Fatal("session not attached")
			}
			late.mu.Lock()
			want := [2]int{late.cols, late.rows}
			late.mu.Unlock()
			select {
			case got := <-intro:
				if got != want {
					t.Fatalf("intro first drawn at %v, want window grid %v", got, want)
				}
			case <-ctx.Done():
				t.Fatal("no intro on the initial grid")
			}
			// The input barrier follows any resize replayed on attachment.
			flushStartupInput(t, late, out.sess)
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(sizes, [][2]int{want}) {
				t.Fatalf("startup grid_resize sequence %v, want only %v", sizes, want)
			}
			t.Logf("initial intro grid %v; grid_resize sequence %v", want, sizes)
		})
	}
}

func TestStartupWaitingForFrameCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	late := &lateBackend{}
	done := make(chan error, 1)
	go func() {
		_, err := late.startSession(ctx, nvimproc.Options{Bin: "must-not-be-started-before-a-frame"})
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting startup returned %v, want cancellation without launching a process", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("closing before the first frame did not stop startup")
	}
}
