package nvimproc

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/r404r/mygonvim/internal/winlayout"
)

func TestLayoutRefreshDoesNotHoldQueuedInput(t *testing.T) {
	var snapshot atomic.Pointer[winlayout.Snapshot]
	s, err := Start(Options{
		Bin: testBin(t), Args: []string{"--clean"},
		Env: PortableEnv(os.Environ(), t.TempDir()), Cols: 80, Rows: 24,
		OnWindowLayout: snapshot.Store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	wait := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	wait("initial geometry without an OnRedraw callback", func() bool { return snapshot.Load() != nil })
	s.Input(`"`)
	wait("Neovim waiting for a register", func() bool {
		mode, err := s.Nvim().Mode()
		return err == nil && mode.Blocking
	})
	for i := 0; i < 1000; i++ {
		s.requestLayout()
	}
	// The Lua query may wait for Esc. Esc must bypass it, then ordered keys
	// create a split and make the redraw refresh the snapshot.
	s.Input("<Esc>:vsplit<CR>")
	wait("geometry and input after a blocking mode", func() bool {
		v := snapshot.Load()
		return v != nil && len(v.Windows) == 2
	})
	s.Resize(90, 30)
	wait("resized geometry", func() bool {
		v := snapshot.Load()
		return v != nil && v.Cols == 90 && v.Rows == 30
	})
	s.Input(":tabnew<CR>")
	wait("current tab geometry", func() bool {
		v := snapshot.Load()
		return v != nil && len(v.Windows) == 1 && v.Windows[0].Row == 1
	})
}
