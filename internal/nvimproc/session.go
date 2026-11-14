package nvimproc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/neovim/go-client/nvim"
)

// Options configures Start.
type Options struct {
	// Bin is the nvim executable (see Locate).
	Bin string
	// Args are given to nvim after --embed, e.g. the files to open.
	Args []string
	// Env is the child's whole environment; nil means this process's.
	Env []string
	// Dir is the working directory; "" means this process's.
	Dir string
	// InitCmds are Ex commands run before the user's init file (nvim --cmd), in
	// order: what the app presets, for the user's own settings to override.
	InitCmds []string
	// Cols and Rows are the grid the UI attaches with.
	Cols, Rows int
	// OnRedraw gets each "redraw" notification's updates, on the client's
	// reader goroutine, in order. It must not call back into the Session
	// synchronously for anything that waits for a reply.
	OnRedraw func(updates ...[]any)
	// OnExit is called once, from its own goroutine, when nvim has exited
	// or the connection was lost; err is nil on a clean exit.
	OnExit func(err error)
	// Logf logs client problems; nil discards them.
	Logf func(format string, args ...any)
}

// UIOptions are the options of nvim_ui_attach: the line grid protocol with
// RGB colors. ext_multigrid, ext_cmdline, ext_popupmenu, ext_tabline come with
// later MVP steps.
func UIOptions() map[string]any {
	return map[string]any{"ext_linegrid": true, "rgb": true}
}

// Session is a running embedded nvim. Input, Resize and Command may be called
// from any goroutine, the UI thread included: they queue the call, which a
// worker goroutine makes, in order, so that the UI thread never waits for nvim.
type Session struct {
	nv  *nvim.Nvim
	cmd *exec.Cmd

	mu     sync.Mutex
	cond   *sync.Cond
	queue  []op
	closed bool

	stderr *tailBuffer
	done   chan struct{} // closed when the process has exited
	logf   func(string, ...any)
}

type opKind uint8

const (
	opInput opKind = iota
	opCommand
	opResize
	opMouse
)

type op struct {
	kind       opKind
	s          string
	cols, rows int
	mouse      mouseOp
}

// mouseOp is one nvim_input_mouse call (grid 0: there is no multigrid yet).
type mouseOp struct {
	button, action, modifier string
	row, col                 int
}

// Start runs nvim --embed and attaches this program as its UI.
func Start(o Options) (*Session, error) {
	if o.Bin == "" {
		return nil, errors.New("nvimproc: no nvim executable")
	}
	if o.Cols < 1 || o.Rows < 1 {
		o.Cols, o.Rows = 80, 24
	}
	logf := o.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	// 'title' is off by default, and nvim then never sends set_title; --cmd
	// runs before the user's init file, which may still turn it off.
	args := []string{"--embed", "--cmd", "set title"}
	for _, c := range o.InitCmds {
		args = append(args, "--cmd", c)
	}
	args = append(args, o.Args...)
	cmd := exec.Command(o.Bin, args...)
	cmd.Env = o.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Dir = o.Dir
	hideWindow(cmd)
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", o.Bin, err)
	}
	nv, err := nvim.New(out, in, in, logf)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	s := &Session{nv: nv, cmd: cmd, stderr: stderr, done: make(chan struct{}), logf: logf}
	s.cond = sync.NewCond(&s.mu)
	if o.OnRedraw != nil {
		if err := nv.RegisterHandler("redraw", o.OnRedraw); err != nil {
			s.kill()
			return nil, err
		}
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- nv.Serve() }()
	go func() {
		err := <-serveErr
		werr := cmd.Wait()
		if err == nil {
			err = werr
		}
		s.mu.Lock()
		closing := s.closed
		s.closed = true
		s.cond.Broadcast()
		s.mu.Unlock()
		close(s.done)
		if closing {
			err = nil
		}
		if err != nil {
			if tail := s.stderr.String(); tail != "" {
				err = fmt.Errorf("%w: %s", err, tail)
			}
		}
		if o.OnExit != nil {
			o.OnExit(err)
		}
	}()
	if err := nv.AttachUI(o.Cols, o.Rows, UIOptions()); err != nil {
		s.kill()
		return nil, fmt.Errorf("nvim_ui_attach: %w (nvim said: %q)", err, s.stderr.String())
	}
	go s.work()
	return s, nil
}

// Nvim returns the underlying client, for calls that wait for an answer. Call
// it from a goroutine of your own, never from the UI thread.
func (s *Session) Nvim() *nvim.Nvim { return s.nv }

// OptionDefault returns the default value of a string option, as Neovim has it
// before any configuration (nvim_get_option_info2). It waits for the answer, so
// call it from a goroutine of your own, never from the UI thread.
func (s *Session) OptionDefault(name string) (string, error) {
	var v string
	err := s.nv.Call("luaeval", &v, "vim.api.nvim_get_option_info2(_A, {}).default", name)
	return v, err
}

// Done is closed when nvim has exited.
func (s *Session) Done() <-chan struct{} { return s.done }

// Input queues keys in nvim_input notation.
func (s *Session) Input(keys string) {
	if keys != "" {
		s.push(op{kind: opInput, s: keys})
	}
}

// Mouse queues nvim_input_mouse: button is "left", "right", "middle", "x1",
// "x2" or "wheel"; action is "press", "drag", "release" or "move", and for the
// wheel "up", "down", "left" or "right"; modifier is a string of S, C, A and D
// letters; row and col are zero-based grid cells. It is sent in order with the
// keys around it.
func (s *Session) Mouse(button, action, modifier string, row, col int) {
	s.push(op{kind: opMouse, mouse: mouseOp{button, action, modifier, row, col}})
}

// Command queues an Ex command (nvim_command). It is sent in order with the
// input before it but not waited for, so a command that asks the user a
// question does not hold up the keys that answer it.
func (s *Session) Command(cmd string) { s.push(op{kind: opCommand, s: cmd}) }

// Resize queues nvim_ui_try_resize. A resize still queued when another comes
// is replaced by it, so dragging a window edge sends only the last size.
func (s *Session) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for i := len(s.queue) - 1; i >= 0; i-- {
		if s.queue[i].kind == opResize {
			s.queue[i].cols, s.queue[i].rows = cols, rows
			return
		}
	}
	s.queue = append(s.queue, op{kind: opResize, cols: cols, rows: rows})
	s.cond.Signal()
}

func (s *Session) push(o op) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.queue = append(s.queue, o)
	s.cond.Signal()
}

// work makes the queued calls in order.
func (s *Session) work() {
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.closed {
			s.mu.Unlock()
			return
		}
		o := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		var err error
		switch o.kind {
		case opInput:
			err = s.sendInput(o.s)
		case opCommand:
			// Not waited for: a command may wait for the user (":confirm
			// qa" asks a question), and the keys that answer it are queued
			// behind this one.
			go func(cmd string) {
				if err := s.nv.Command(cmd); err != nil && !s.exitedSoon() {
					s.logf("nvim command %q failed: %v", cmd, err)
				}
			}(o.s)
		case opResize:
			err = s.nv.TryResizeUI(o.cols, o.rows)
		case opMouse:
			m := o.mouse
			err = s.nv.InputMouse(m.button, m.action, m.modifier, 0, m.row, m.col)
		}
		if err != nil && !s.exitedSoon() {
			s.logf("nvim call failed: %v", err)
		}
	}
}

// exitedSoon reports whether nvim exits within a moment: a call that fails
// because it quit (:qa) is no error.
func (s *Session) exitedSoon() bool {
	select {
	case <-s.done:
		return true
	case <-time.After(200 * time.Millisecond):
		return false
	}
}

// sendInput writes keys with nvim_input, which may take fewer bytes than it
// is given when nvim's input buffer is full; the rest is sent again.
func (s *Session) sendInput(keys string) error {
	for tries := 0; keys != "" && tries < 200; tries++ {
		n, err := s.nv.Input(keys)
		if err != nil {
			return err
		}
		if n >= len(keys) {
			return nil
		}
		keys = keys[n:]
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

// Close ends the session: it closes the connection, which makes nvim exit,
// and kills nvim if it has not within two seconds. Pending input is dropped.
func (s *Session) Close() {
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()
	s.nv.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		s.kill()
		<-s.done
	}
}

func (s *Session) kill() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

// tailBuffer keeps the last max bytes written to it: nvim's stderr, for the
// message when startup fails.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

var _ io.Writer = (*tailBuffer)(nil)

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(bytes.TrimSpace(t.buf))
}
