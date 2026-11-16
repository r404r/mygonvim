package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/r404r/mygonvim/internal/appmenu"
	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

func TestOpenFilesSendsOneConfirmDropCommand(t *testing.T) {
	e := newEnv(t, 400, 200)
	e.ed.OpenFiles([]string{"/tmp/a b.txt", "/tmp/c#d.txt"})
	if len(e.be.cmds) != 1 {
		t.Fatalf("commands = %q, want exactly one", e.be.cmds)
	}
	want := `confirm drop /tmp/a\ b.txt /tmp/c\#d.txt`
	if e.be.cmds[0] != want {
		t.Errorf("command = %q, want %q", e.be.cmds[0], want)
	}
}

func TestOpenFilesWithNothingSendsNothing(t *testing.T) {
	e := newEnv(t, 400, 200)
	e.ed.OpenFiles(nil)
	e.ed.OpenFiles([]string{})
	e.ed.OpenFiles([]string{""})
	if len(e.be.cmds) != 0 {
		t.Errorf("commands = %q, want none", e.be.cmds)
	}
}

// A file dropped on the window shows up in a real Neovim, also when its name
// has characters that are special on the command line.
func TestRealNvimOpensDroppedFiles(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	dir := t.TempDir()
	first := filepath.Join(dir, "first one #1.txt")
	second := filepath.Join(dir, "100%'second.txt")
	if err := os.WriteFile(first, []byte("content of the first file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("content of the second file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var ed *Editor
	ed = NewEditor(nil, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{})
	sess, err := nvimproc.Start(nvimproc.Options{
		Bin: bin, Cols: 80, Rows: 24,
		Args:     []string{"--clean"},
		Env:      nvimproc.PortableEnv(os.Environ(), filepath.Join(dir, "data")),
		OnRedraw: func(u ...[]any) { ed.OnRedraw(u...) },
		Logf:     t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	ed.be = sess
	defer sess.Close()

	screen := func() string {
		var rows []string
		ed.Model.Read(func(s *grid.State) {
			for r := 0; r < s.Rows; r++ {
				rows = append(rows, s.RowText(r))
			}
		})
		return strings.Join(rows, "\n")
	}
	wait := func(what, text string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screen(), text) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s; screen:\n%s", what, screen())
	}

	ed.OpenFiles([]string{first})
	wait("the first file", "content of the first file")

	ed.OpenFiles([]string{second})
	wait("the second file", "content of the second file")

	if runtime.GOOS != "windows" {
		third := filepath.Join(dir, "pipe|third.txt")
		if err := os.WriteFile(third, []byte("content of the pipe file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ed.OpenFiles([]string{third})
		wait("the file with a pipe in its name", "content of the pipe file")
	}
}

// The File > Open... path end to end, with the dialog stubbed: chosen files
// reach the backend as one command, and a canceled dialog sends nothing.
func TestOpenDialogFeedsOpenFiles(t *testing.T) {
	e := newEnv(t, 400, 200)
	appmenu.ChooseAndOpen(func() ([]string, error) { return nil, nil }, e.ed.OpenFiles, nil)
	if len(e.be.cmds) != 0 {
		t.Fatalf("a canceled dialog sent %q, want nothing", e.be.cmds)
	}
	appmenu.ChooseAndOpen(func() ([]string, error) { return []string{"/tmp/x y.txt"}, nil }, e.ed.OpenFiles, nil)
	if want := []string{`confirm drop /tmp/x\ y.txt`}; !reflect.DeepEqual(e.be.cmds, want) {
		t.Errorf("commands = %q, want %q", e.be.cmds, want)
	}
}
