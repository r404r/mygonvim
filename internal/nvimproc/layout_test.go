package nvimproc

import (
	"errors"
	"net"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neovim/go-client/msgpack/rpc"
	"github.com/neovim/go-client/nvim"
	"github.com/r404r/mygonvim/internal/winlayout"
)

func TestLayoutRefreshPublishesChangesAndClearsFailedGeometry(t *testing.T) {
	client, server := net.Pipe()
	nv, err := nvim.New(client, client, client, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := rpc.NewEndpoint(server, server, server)
	if err != nil {
		t.Fatal(err)
	}
	type reply struct {
		snapshot *winlayout.Snapshot
		err      error
	}
	entered := make(chan struct{}, 1)
	replies := make(chan reply)
	stop := make(chan struct{})
	if err := peer.Register("nvim_exec_lua", func(string, []any) (*winlayout.Snapshot, error) {
		entered <- struct{}{}
		select {
		case r := <-replies:
			return r.snapshot, r.err
		case <-stop:
			return nil, errors.New("closed")
		}
	}); err != nil {
		t.Fatal(err)
	}
	clientDone, serverDone, refreshDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { defer close(clientDone); nv.Serve() }()
	go func() { defer close(serverDone); peer.Serve() }()
	s := &Session{nv: nv, done: make(chan struct{}), layoutRefresh: make(chan struct{}, 1)}
	published := make(chan *winlayout.Snapshot, 1)
	go func() { defer close(refreshDone); s.refreshLayouts(func(v *winlayout.Snapshot) { published <- v }) }()
	t.Cleanup(func() {
		close(s.done)
		close(stop)
		nv.Close()
		peer.Close()
		<-refreshDone
		<-clientDone
		<-serverDone
	})
	query := func() {
		t.Helper()
		s.requestLayout()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("layout query did not arrive")
		}
	}
	query()
	check := func(name string, snapshot *winlayout.Snapshot, err error, wantPublish bool) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			replies <- reply{snapshot, err}
			// Entering the next RPC proves the previous publication finished.
			// Hold its reply until the next check, avoiding timing-based silence checks.
			query()
			select {
			case got := <-published:
				if !wantPublish {
					t.Fatal("unchanged geometry was published")
				}
				if snapshot == nil {
					if got != nil {
						t.Fatal("failed query did not clear cached geometry")
					}
				} else if got == nil || got.Cols != snapshot.Cols || got.Rows != snapshot.Rows ||
					got.CmdHeight != snapshot.CmdHeight || got.GlobalStatus != snapshot.GlobalStatus ||
					!slices.Equal(got.Windows, snapshot.Windows) {
					t.Fatalf("published %+v, want %+v", got, snapshot)
				}
			default:
				if wantPublish {
					t.Fatal("changed geometry was not published")
				}
			}
		})
	}
	failed := errors.New("query failed")
	check("initial failure", nil, failed, false)
	v := winlayout.Snapshot{Cols: 80, Rows: 24, CmdHeight: 1, Windows: []winlayout.Window{{Width: 80, Height: 22}}}
	check("first snapshot", &v, nil, true)
	check("same geometry", &v, nil, false)
	for _, change := range []struct {
		name string
		edit func(*winlayout.Snapshot)
	}{
		{"columns", func(s *winlayout.Snapshot) { s.Cols++ }},
		{"rows", func(s *winlayout.Snapshot) { s.Rows++ }},
		{"command height", func(s *winlayout.Snapshot) { s.CmdHeight++ }},
		{"global status", func(s *winlayout.Snapshot) { s.GlobalStatus = true }},
		{"window geometry", func(s *winlayout.Snapshot) { s.Windows[0].Width-- }},
		{"split", func(s *winlayout.Snapshot) {
			s.Windows = append(s.Windows, winlayout.Window{Col: 40, Width: 40, Height: 22})
		}},
	} {
		change.edit(&v)
		check(change.name, &v, nil, true)
		check(change.name+" unchanged", &v, nil, false)
	}
	check("clear after failure", nil, failed, true)
	check("repeated failure", nil, failed, false)
	check("recover same geometry", &v, nil, true)
}

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
