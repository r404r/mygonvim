package openfiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neovim/go-client/nvim"
)

func TestCommand(t *testing.T) {
	cases := []struct {
		name  string
		goos  string
		paths []string
		want  string
	}{
		{"nil list", "linux", nil, ""},
		{"empty list", "linux", []string{}, ""},
		{"only empty names", "linux", []string{"", ""}, ""},
		{"one file", "linux", []string{"/home/me/a.go"}, "confirm drop /home/me/a.go"},
		{"spaces", "linux", []string{"/home/me/my notes.txt"}, `confirm drop /home/me/my\ notes.txt`},
		{"percent and hash", "linux", []string{"/tmp/50%#1.txt"}, `confirm drop /tmp/50\%\#1.txt`},
		{"bar", "linux", []string{"/tmp/a|b"}, `confirm drop /tmp/a\|b`},
		{"backslash on unix", "linux", []string{`/tmp/a\b`}, `confirm drop /tmp/a\\b`},
		{"quotes and wildcards", "darwin", []string{`/tmp/it's "x" *?[{$` + "`"}, `confirm drop /tmp/it\'s\ \"x\"\ \*\?\[\{\$\` + "`"},
		{"bang and angle", "linux", []string{"/tmp/a!<b"}, `confirm drop /tmp/a\!\<b`},
		{"leading plus", "linux", []string{"+a"}, `confirm drop \+a`},
		{"leading angle", "linux", []string{">a"}, `confirm drop \>a`},
		{"lone dash", "linux", []string{"-"}, `confirm drop \-`},
		{"dash prefix is kept", "linux", []string{"-a"}, `confirm drop -a`},
		{"plus inside", "linux", []string{"/tmp/a+b"}, `confirm drop /tmp/a+b`},
		{"several files", "linux", []string{"/a/one.go", "/a/t wo.go", "/a/three.go"}, `confirm drop /a/one.go /a/t\ wo.go /a/three.go`},
		{"empty names are skipped", "linux", []string{"", "/a/b"}, `confirm drop /a/b`},
		{"directory", "linux", []string{"/home/me/project/"}, `confirm drop /home/me/project/`},
		{"chinese", "darwin", []string{"/Users/我/文档/说明 书.md"}, `confirm drop /Users/我/文档/说明\ 书.md`},
		{"windows literal bang and brackets", "windows", []string{`C:\tmp\a!b[c{d.txt`}, `confirm drop C:\tmp\a!b[c{d.txt`},
		{"windows keeps backslashes", "windows", []string{`C:\Users\me\a b.txt`}, `confirm drop C:\Users\me\a\ b.txt`},
		{"windows specials", "windows", []string{`C:\tmp\50%#1|x$y.txt`}, `confirm drop C:\tmp\50\%\#1\|x$y.txt`},
		{"windows chinese and forward slashes", "windows", []string{`D:/文档/说明.md`}, `confirm drop D:/文档/说明.md`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Command(c.goos, c.paths); got != c.want {
				t.Errorf("Command(%q, %q)\n got %q\nwant %q", c.goos, c.paths, got, c.want)
			}
		})
	}
}

// nvimBin finds the nvim to test against, as the other packages do.
func nvimBin(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("NVIM_BIN"); b != "" {
		return b
	}
	b, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
	}
	return b
}

func startNvim(t *testing.T) *nvim.Nvim {
	t.Helper()
	dir := t.TempDir()
	nv, err := nvim.NewChildProcess(
		nvim.ChildProcessCommand(nvimBin(t)),
		nvim.ChildProcessArgs("--embed", "--headless", "--clean"),
		nvim.ChildProcessDir(dir),
		nvim.ChildProcessEnv(append(os.Environ(), "XDG_CONFIG_HOME="+dir, "XDG_DATA_HOME="+dir, "XDG_STATE_HOME="+dir)),
		nvim.ChildProcessServe(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() { nv.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	return nv
}

// Escape must agree with the real fnameescape() on every character that
// matters, so the rule above cannot drift from Neovim's.
func TestEscapeMatchesNvimFnameescape(t *testing.T) {
	nv := startNvim(t)
	var all strings.Builder
	for c := byte(1); c < 128; c++ {
		all.WriteByte(c)
	}
	names := []string{all.String(), "+a", ">a", "-", "-a", "a b", `a\b`, "50%#", "a|b", "文档 说明"}
	for c := byte(1); c < 128; c++ {
		names = append(names, string([]byte{c}), "x"+string([]byte{c})+"y")
	}
	for _, n := range names {
		var want string
		if err := nv.Call("fnameescape", &want, n); err != nil {
			t.Fatal(err)
		}
		if got := Escape(runtime.GOOS, n); got != want {
			t.Errorf("Escape(%q) = %q, fnameescape() = %q", n, got, want)
		}
	}
}

// The command must make a real Neovim open the files, whatever the names.
func TestCommandOpensFilesInNvim(t *testing.T) {
	nv := startNvim(t)
	root := t.TempDir()
	names := []string{
		"plain.txt", "with space.txt", "percent%hash#.txt", "bar|name.txt", "quote'single.txt",
		"dollar$x.txt", "angle<x.txt", "bang!.txt", "star*.txt", "文档 说明.md", "+plus.txt", ">gt.txt",
		"brace{x}.txt", "bracket[x].txt", "back`tick.txt",
	}
	names = append(names, `back\slash.txt`, "tab\there.txt")
	sub := filepath.Join(root, "dir with space")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.ContainsAny(n, "<>|*\\\t") {
				t.Skip("Windows file names cannot contain <, >, |, *, backslash or tab; escaping is covered by TestEscapeMatchesNvimFnameescape")
			}
			p := filepath.Join(sub, n)
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := nv.Command(Command(runtime.GOOS, []string{p})); err != nil {
				t.Fatalf("nvim rejected %q: %v", Command(runtime.GOOS, []string{p}), err)
			}
			var cur string
			if err := nv.Call("expand", &cur, "%:p"); err != nil {
				t.Fatal(err)
			}
			if filepath.Clean(cur) != filepath.Clean(p) {
				t.Errorf("current buffer is %q, want %q", cur, p)
			}
		})
	}
}

func TestCommandOpensLiteralBracketFile(t *testing.T) {
	nv := startNvim(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "report[1].txt")
	for _, path := range []string{want, filepath.Join(root, "report1.txt")} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := nv.Command(Command(runtime.GOOS, []string{want})); err != nil {
		t.Fatal(err)
	}
	var cur string
	if err := nv.Call("expand", &cur, "%:p"); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cur) != want {
		t.Errorf("current buffer is %q, want %q", cur, want)
	}
	var args []string
	if err := nv.Call("argv", &args); err != nil {
		t.Fatal(err)
	}
	for i := range args {
		args[i] = filepath.Clean(args[i])
	}
	if !reflect.DeepEqual(args, []string{want}) {
		t.Errorf("argument list is %q, want only %q", args, want)
	}
}

func TestCommandSeveralFilesAndDirectory(t *testing.T) {
	nv := startNvim(t)
	root := t.TempDir()
	// netrw reports the physical path; macOS's temporary directory uses /var
	// as a symlink to /private/var.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, n := range []string{"a one.txt", "b#two.txt", "c.txt"} {
		p := filepath.Join(root, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	if err := nv.Command(Command(runtime.GOOS, files)); err != nil {
		t.Fatal(err)
	}
	var cur string
	if err := nv.Call("expand", &cur, "%:p"); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cur) != filepath.Clean(files[0]) {
		t.Errorf("shown file is %q, want the first, %q", cur, files[0])
	}
	var args []string
	if err := nv.Call("argv", &args); err != nil {
		t.Fatal(err)
	}
	for i := range args {
		args[i] = filepath.Clean(args[i])
	}
	if !reflect.DeepEqual(args, files) {
		t.Errorf("argument list is %q, want %q", args, files)
	}

	// A directory opens too: netrw, which ships with Neovim, lists it. Its
	// buffer has no name; b:netrw_curdir tells which directory it shows.
	dir := filepath.Join(root, "sub dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := nv.Command(Command(runtime.GOOS, []string{dir})); err != nil {
		t.Fatal(err)
	}
	var shown string
	if err := nv.Eval(`get(b:, "netrw_curdir", "")`, &shown); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(shown) != filepath.Clean(dir) {
		t.Errorf("after opening a directory the explorer shows %q, want %q", shown, dir)
	}
}
