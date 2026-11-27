//go:build unix

package loginenv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// The streams share a byte budget. Stderr is counted but never retained.
type capture struct {
	mu     sync.Mutex
	out    bytes.Buffer
	total  int
	over   bool
	cancel context.CancelFunc
}

type stream struct {
	c    *capture
	keep bool
}

func (s stream) Write(p []byte) (int, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.over {
		return len(p), nil
	}
	if len(p) > outputLimit-s.c.total {
		s.c.over = true
		s.c.cancel()
		return len(p), nil
	}
	s.c.total += len(p)
	if s.keep {
		_, _ = s.c.out.Write(p)
	}
	return len(p), nil
}

func runProcess(parent context.Context, request Request) Output {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, request.Path, request.Args...)
	cmd.Env, cmd.Dir = request.Env, request.Dir
	// A nil Stdin makes os/exec connect the child to the null device.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.WaitDelay = waitDelay
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	buf := &capture{cancel: cancel}
	cmd.Stdout, cmd.Stderr = stream{buf, true}, stream{buf, false}
	if err := cmd.Start(); err != nil {
		return Output{ExitCode: -1, Failure: "process unavailable"}
	}
	err := cmd.Wait()
	// This also releases descendants which kept a pipe open after shell exit.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	buf.mu.Lock()
	defer buf.mu.Unlock()
	out := Output{Stdout: append([]byte{}, buf.out.Bytes()...), ExitCode: cmd.ProcessState.ExitCode()}
	switch {
	case buf.over:
		out.Failure = "output limit"
	case parent.Err() != nil:
		out.Failure = "cancelled"
	case err != nil && !errors.Is(err, exec.ErrWaitDelay):
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			out.Failure = "process failure"
		}
	}
	return out
}
