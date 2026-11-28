package main

import (
	"context"
	"errors"

	"github.com/r404r/mygonvim/internal/loginenv"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

// startup owns the pending probe and the late session. Close cancels the probe
// and joins its worker before the log file is closed by main.
type startup struct {
	cancel context.CancelFunc
	done   chan struct{}
	late   *lateBackend
}

func (s *startup) Close() {
	s.late.discard()
	s.cancel()
	<-s.done
	if sess := s.late.be.Load(); sess != nil {
		sess.Close()
	}
}

// Show the replacement before closing the editor: closing the last native
// window first would quit the application before it could show the error.
func startupFailure(late *lateBackend, err error, show func(error), closeEditor func()) {
	late.discard()
	show(err)
	closeEditor()
}

func prepareEnvironment(ctx context.Context, opts loginenv.Options, portableDir string) loginenv.Result {
	result := loginenv.Import(ctx, opts)
	if portableDir != "" {
		result.Env = nvimproc.PortableEnv(result.Env, portableDir)
	}
	return result
}

// startSession waits for the editor's first draw to measure the window, so the
// initial UI attachment uses its grid rather than the blank model's size.
// The startup worker waits here; the UI thread remains free to draw or close.
func (l *lateBackend) startSession(ctx context.Context, opts nvimproc.Options) (*nvimproc.Session, error) {
	l.mu.Lock()
	if l.cols < 1 || l.rows < 1 {
		if l.sizeReady == nil {
			l.sizeReady = make(chan struct{})
		}
		ready := l.sizeReady
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ready:
		}
		l.mu.Lock()
	}
	opts.Cols, opts.Rows = l.cols, l.rows
	l.initialCols, l.initialRows = l.cols, l.rows
	l.mu.Unlock()
	return nvimproc.StartContext(ctx, opts)
}

type lateLuaClient struct{ late *lateBackend }

func (l lateLuaClient) ExecLua(code string, result interface{}, args ...interface{}) error {
	if sess := l.late.be.Load(); sess != nil {
		return sess.Nvim().ExecLua(code, result, args...)
	}
	return errors.New("Neovim has not started")
}
