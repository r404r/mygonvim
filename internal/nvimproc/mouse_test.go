package nvimproc

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// longFile is 200 lines "line 1" .. "line 200", in a file nvim opens.
func longFile(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	p := t.TempDir() + "/long.txt"
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// startWithFile runs nvim on file with the given extra --cmd commands and waits
// for its first flush.
func startWithFile(t *testing.T, file string, cmds ...string) *Session {
	t.Helper()
	bin := testBin(t)
	var flushed = make(chan struct{}, 1)
	args := []string{"--clean"}
	for _, c := range cmds {
		args = append(args, "--cmd", c)
	}
	s, err := Start(Options{
		Bin: bin, Args: append(args, file), Env: PortableEnv(os.Environ(), t.TempDir()), Cols: 60, Rows: 12,
		OnRedraw: func(updates ...[]any) {
			for _, u := range updates {
				if u[0] == "flush" {
					select {
					case flushed <- struct{}{}:
					default:
					}
				}
			}
		},
		Logf: t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	select {
	case <-flushed:
	case <-time.After(5 * time.Second):
		t.Fatal("no first flush")
	}
	return s
}

// topLine is the first line shown in the current window.
func topLine(t *testing.T, s *Session) int {
	t.Helper()
	var n int
	if err := s.Nvim().Eval(`line('w0')`, &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// waitTop waits until the top line is want.
func waitTop(t *testing.T, s *Session, want int) {
	t.Helper()
	var got int
	for i := 0; i < 200; i++ {
		if got = topLine(t, s); got == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("top line is %d, want %d", got, want)
}

func TestMouseWheelScrollsTheWindow(t *testing.T) {
	s := startWithFile(t, longFile(t))
	if got := topLine(t, s); got != 1 {
		t.Fatalf("top line starts at %d", got)
	}
	// Each wheel step moves three lines ('mousescroll' default ver:3).
	s.Mouse("wheel", "down", "", 3, 5)
	waitTop(t, s, 4)
	s.Mouse("wheel", "down", "", 3, 5)
	s.Mouse("wheel", "down", "", 3, 5)
	waitTop(t, s, 10)
	s.Mouse("wheel", "up", "", 3, 5)
	waitTop(t, s, 7)
}

func TestMouseWheelFollowsTheKeysBeforeIt(t *testing.T) {
	s := startWithFile(t, longFile(t))
	// Keys and wheel steps share one queue, so they act in the order sent.
	s.Input("50G")
	s.Mouse("wheel", "up", "", 3, 5)
	s.Input("zt")
	var cur int
	for i := 0; i < 200; i++ {
		if err := s.Nvim().Eval(`line('.')`, &cur); err == nil && cur >= 1 && topLine(t, s) >= 40 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if top := topLine(t, s); top < 40 {
		t.Errorf("top line %d after 50G, a wheel step and zt", top)
	}
}

func TestMouseWheelSideways(t *testing.T) {
	long := strings.Repeat("0123456789", 30)
	p := t.TempDir() + "/wide.txt"
	if err := os.WriteFile(p, []byte(long+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := startWithFile(t, p, "set nowrap")
	left := func() (n int) {
		if err := s.Nvim().Eval(`winsaveview().leftcol`, &n); err != nil {
			t.Fatal(err)
		}
		return
	}
	s.Mouse("wheel", "right", "", 0, 5)
	var got int
	for i := 0; i < 200 && got == 0; i++ {
		got = left()
		time.Sleep(25 * time.Millisecond)
	}
	if got != 6 {
		t.Errorf("leftcol %d after one step right, want 6 ('mousescroll' default hor:6)", got)
	}
	s.Mouse("wheel", "left", "", 0, 5)
	for i := 0; i < 200 && left() != 0; i++ {
		time.Sleep(25 * time.Millisecond)
	}
	if got := left(); got != 0 {
		t.Errorf("leftcol %d after stepping back, want 0", got)
	}
}
