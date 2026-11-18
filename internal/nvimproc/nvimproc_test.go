package nvimproc

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCandidatesOrder(t *testing.T) {
	got := Candidates("windows", `C:\x\nvim.exe`, "", []string{`C:\app`, `C:\res`})
	want := []string{
		`C:\x\nvim.exe`,
		filepath.Join(`C:\app`, "nvim", "bin", "nvim.exe"), filepath.Join(`C:\app`, "nvim", "nvim.exe"),
		filepath.Join(`C:\res`, "nvim", "bin", "nvim.exe"), filepath.Join(`C:\res`, "nvim", "nvim.exe"),
		"", // PATH
	}
	if !slices.Equal(got, want) {
		t.Errorf("windows candidates:\n got %q\nwant %q", got, want)
	}
	mac := Candidates("darwin", "", "", nil)
	if !slices.Equal(mac, []string{"", "/opt/homebrew/bin/nvim", "/usr/local/bin/nvim"}) {
		t.Errorf("darwin candidates %q", mac)
	}
}

func TestLocatePrefersBundledOverPath(t *testing.T) {
	bundled := filepath.Join("app", "nvim", "bin", "nvim")
	exists := func(p string) bool { return p == bundled }
	look := func(string) (string, error) { return "/usr/bin/nvim", nil }
	got, err := locate("linux", "", "", []string{"app"}, exists, look)
	if err != nil || got != bundled {
		t.Errorf("got %q, %v; want the bundled copy", got, err)
	}
	got, err = locate("linux", "", "", []string{"other"}, exists, look)
	if err != nil || got != "/usr/bin/nvim" {
		t.Errorf("got %q, %v; want PATH's", got, err)
	}
}

func TestLocateExplicitPathIsNotSilentlyReplaced(t *testing.T) {
	exists := func(string) bool { return false }
	look := func(p string) (string, error) {
		if p == "nvim" {
			return "/usr/bin/nvim", nil
		}
		return "", errors.New("not found")
	}
	if _, err := locate("linux", "/nope/nvim", "", nil, exists, look); err == nil || !strings.Contains(err.Error(), "/nope/nvim") {
		t.Errorf("explicit missing path: err %v", err)
	}
	// The env variable is the same, but the flag wins.
	got, err := locate("linux", "/flag/nvim", "/env/nvim", nil, func(p string) bool { return true }, func(p string) (string, error) { return p, nil })
	if err != nil || got != "/flag/nvim" {
		t.Errorf("flag should beat env: %q %v", got, err)
	}
}

func TestLocateNothingFound(t *testing.T) {
	_, err := locate("linux", "", "", []string{"app"}, func(string) bool { return false }, func(string) (string, error) { return "", errors.New("no") })
	if err == nil || !strings.Contains(err.Error(), "nvim not found") {
		t.Errorf("err %v", err)
	}
}

func TestPortableDirNeedsDataFolder(t *testing.T) {
	dir := t.TempDir()
	if _, ok := PortableDir(dir); ok {
		t.Error("portable without a data folder")
	}
	if _, ok := PortableDir(""); ok {
		t.Error("portable with no exe dir")
	}
	// A file called data is not a folder.
	os.WriteFile(filepath.Join(dir, "data"), nil, 0o644)
	if _, ok := PortableDir(dir); ok {
		t.Error("portable with a data file")
	}
	os.Remove(filepath.Join(dir, "data"))
	os.Mkdir(filepath.Join(dir, "data"), 0o755)
	got, ok := PortableDir(dir)
	if !ok || got != filepath.Join(dir, "data") {
		t.Errorf("got %q %v", got, ok)
	}
}

func TestPortableEnvOverridesXDG(t *testing.T) {
	base := []string{"PATH=/bin", "XDG_CONFIG_HOME=/home/u/.config", "xdg_data_home=/x", "HOME=/home/u"}
	got := PortableEnv(base, "/app/data")
	has := map[string]string{}
	for _, kv := range got {
		k, v, _ := strings.Cut(kv, "=")
		has[k] = v
	}
	for k, v := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join("/app/data", "config"),
		"XDG_DATA_HOME":   filepath.Join("/app/data", "share"),
		"XDG_STATE_HOME":  filepath.Join("/app/data", "state"),
		"XDG_CACHE_HOME":  filepath.Join("/app/data", "cache"),
		"PATH":            "/bin", "HOME": "/home/u",
	} {
		if has[k] != v {
			t.Errorf("%s = %q, want %q", k, has[k], v)
		}
	}
	if _, dup := has["xdg_data_home"]; dup {
		t.Error("the user's differently-cased XDG variable survived")
	}
	if len(got) != 6 {
		t.Errorf("env %q", got)
	}
	if base[1] != "XDG_CONFIG_HOME=/home/u/.config" {
		t.Error("input env was modified")
	}
}

func TestEnsurePortableDirs(t *testing.T) {
	d := filepath.Join(t.TempDir(), "data")
	if err := EnsurePortableDirs(d); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"config", "share", "state", "cache"} {
		if st, err := os.Stat(filepath.Join(d, s)); err != nil || !st.IsDir() {
			t.Errorf("%s: %v", s, err)
		}
	}
}

func testBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	return bin
}

// A real nvim: attach, get a first flush, send keys, resize, quit.
func TestSessionAgainstRealNvim(t *testing.T) {
	bin := testBin(t)
	data := t.TempDir()
	env := PortableEnv(os.Environ(), data)
	var flushes, titles atomic.Int64
	var exited atomic.Bool
	exitErr := make(chan error, 1)
	s, err := Start(Options{
		Bin: bin, Args: []string{"--clean"}, Env: env, Cols: 60, Rows: 12,
		OnRedraw: func(updates ...[]any) {
			for _, u := range updates {
				switch u[0] {
				case "flush":
					flushes.Add(1)
				case "set_title":
					titles.Add(1)
				}
			}
		},
		OnExit: func(err error) { exited.Store(true); exitErr <- err },
		Logf:   t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	wait := func(what string, cond func() bool) {
		t.Helper()
		for i := 0; i < 200; i++ {
			if cond() {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	wait("first flush", func() bool { return flushes.Load() > 0 })

	// Keys go through in order, and a resize between them is honored.
	s.Input("ihello<Esc>")
	s.Resize(70, 15)
	s.Input(":let g:size = &columns . 'x' . &lines<CR>")
	var got string
	wait("the size variable", func() bool {
		if err := s.Nvim().Var("size", &got); err != nil {
			return false
		}
		return true
	})
	if got != "70x15" {
		t.Errorf("nvim sees %q, want 70x15", got)
	}
	var cfg string
	if err := s.Nvim().Eval(`stdpath('config')`, &cfg); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(data, "config", "nvim"); cfg != want {
		t.Errorf("stdpath(config) = %q, want %q (portable XDG env)", cfg, want)
	}

	s.Command("qa!")
	select {
	case err := <-exitErr:
		if err != nil {
			t.Errorf("exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nvim did not exit")
	}
	s.Close()
	s.Input("x") // after exit: dropped, no panic, no block
}

func TestCloseKillsAPromptNvim(t *testing.T) {
	bin := testBin(t)
	s, err := Start(Options{Bin: bin, Args: []string{"--clean"}, Env: PortableEnv(os.Environ(), t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s.Close()
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("Close took %v", d)
	}
	select {
	case <-s.Done():
	default:
		t.Error("Done not closed")
	}
}

func TestStartFailsClearlyForAMissingBinary(t *testing.T) {
	_, err := Start(Options{Bin: filepath.Join(t.TempDir(), "no-such-nvim")})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// OptionDefault tells the value an option has before any configuration, which
// differs from the current value once it is set. Since Neovim 0.12 the default
// of guifont is a font list, not "".
func TestOptionDefaultIsNotTheCurrentValue(t *testing.T) {
	bin := testBin(t)
	dir := t.TempDir()
	s, err := Start(Options{
		Bin: bin, Args: []string{"--clean"}, Env: PortableEnv(os.Environ(), filepath.Join(dir, "data")),
		Cols: 60, Rows: 12, Logf: t.Logf, OnRedraw: func(...[]any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var initial string
	if err := s.Nvim().Eval("&guifont", &initial); err != nil {
		t.Fatal(err)
	}
	def, err := s.OptionDefault("guifont")
	if err != nil {
		t.Fatal(err)
	}
	if def != initial {
		t.Errorf("default %q, but an untouched guifont is %q", def, initial)
	}
	if err := s.Nvim().Command("set guifont=Menlo:h13"); err != nil {
		t.Fatal(err)
	}
	if again, err := s.OptionDefault("guifont"); err != nil || again != def {
		t.Errorf("after :set guifont the default is %q, %v; want it unchanged: %q", again, err, def)
	}
	if _, err := s.OptionDefault("no_such_option"); err == nil {
		t.Error("an unknown option gave no error")
	}
}

// InitCmds run before the user's init file, so the init file can override what
// the app presets.
func TestInitCmdsRunBeforeTheInitFile(t *testing.T) {
	bin := testBin(t)
	dir := t.TempDir()
	initFile := filepath.Join(dir, "init.vim")
	content := "let g:seen = get(g:, 'preset', 'missing')\nlet &guifont = 'User:h20'\n"
	if err := os.WriteFile(initFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Start(Options{
		Bin: bin, Args: []string{"-u", initFile}, Env: PortableEnv(os.Environ(), filepath.Join(dir, "data")),
		InitCmds: []string{"let g:preset = 'yes'", "let &guifont = 'Saved:h12'"},
		Cols:     60, Rows: 12, Logf: t.Logf, OnRedraw: func(...[]any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var seen, guifont string
	if err := s.Nvim().Var("seen", &seen); err != nil {
		t.Fatal(err)
	}
	if seen != "yes" {
		t.Errorf("the init file saw g:preset = %q, want the command to have run first", seen)
	}
	if err := s.Nvim().Eval("&guifont", &guifont); err != nil {
		t.Fatal(err)
	}
	if guifont != "User:h20" {
		t.Errorf("guifont is %q, want the init file's setting to win", guifont)
	}
}

func TestNoInitCmdsMeansNoExtraArguments(t *testing.T) {
	bin := testBin(t)
	dir := t.TempDir()
	s, err := Start(Options{
		Bin: bin, Args: []string{"--clean"}, Env: PortableEnv(os.Environ(), filepath.Join(dir, "data")),
		Cols: 60, Rows: 12, Logf: t.Logf, OnRedraw: func(...[]any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var title int
	if err := s.Nvim().Eval("&title", &title); err != nil || title != 1 {
		t.Errorf("'title' = %d, %v; want the built-in --cmd to still run", title, err)
	}
}

func TestLocateResolvesRelativeExecutableForPathActions(t *testing.T) {
	t.Chdir(t.TempDir())
	relative := filepath.Join("bin", BinaryName())
	if err := os.Mkdir("bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relative, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, relative)
	got, err := Locate(relative, "", nil)
	if err != nil || got != path {
		t.Fatalf("Locate(%q) = %q, %v; want %q", relative, got, err, path)
	}
}

func TestLocatePreservesAbsoluteExecutablePath(t *testing.T) {
	// Keep the checkout's working directory: Windows runners put temp files
	// on C: while the checkout is on D:, exercising different volumes.
	root := t.TempDir()
	path := filepath.Join(root, "nvim", "bin", BinaryName())
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("working directory: %s; executable: %s", cwd, path)
	for _, source := range []string{"flag", "environment", "bundled"} {
		t.Run(source, func(t *testing.T) {
			var flagPath, envPath string
			var dirs []string
			switch source {
			case "flag":
				flagPath = path
			case "environment":
				envPath = path
			case "bundled":
				dirs = []string{root}
			}
			got, err := Locate(flagPath, envPath, dirs)
			if err != nil || got != path {
				t.Fatalf("Locate(%s) = %q, %v; want %q", source, got, err, path)
			}
		})
	}
}
