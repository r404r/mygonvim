package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/fontcfg"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

type savedFonts struct{ got []fontcfg.Choice }

func newMenuEnv(t *testing.T, cfg Config) (*fontEnv, *savedFonts) {
	t.Helper()
	sv := &savedFonts{}
	fe := &fontEnv{env: &env{be: &fake{}}}
	cfg.Cols, cfg.Rows = 80, 24
	fe.ed = NewEditor(fe.be, cfg, Hooks{
		Invalidate:  func() { fe.inv++ },
		FontChanged: func(c fontcfg.Choice) { fe.changes = append(fe.changes, c) },
		SaveFont:    func(c fontcfg.Choice) { sv.got = append(sv.got, c) },
	})
	fe.ed.SetDefaultGuiFont("")
	fe.tt = ui.NewTester(fe.ed.View, 640, 400)
	fe.tt.SetScale(1)
	fe.tt.Frame()
	fe.inv = 0
	return fe, sv
}

func TestChooseFamilySavesShowsAndSetsGuifont(t *testing.T) {
	fe, sv := newMenuEnv(t, Config{FontFamily: "monospace", FontSize: 14})
	fe.ed.ChooseFamily("serif")
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 14 {
		t.Errorf("font %+v right after the choice, want serif at the unchanged size 14", got)
	}
	if want := []fontcfg.Choice{{Family: "serif"}}; !reflect.DeepEqual(sv.got, want) {
		t.Errorf("saved %+v, want only the family %+v", sv.got, want)
	}
	if want := []string{"let &guifont = 'serif:h14'"}; !reflect.DeepEqual(fe.be.cmds, want) {
		t.Errorf("commands %q, want %q", fe.be.cmds, want)
	}
	// The echo of that command is not a change.
	changes := len(fe.changes)
	setGuiFont(fe.env, "serif:h14")
	fe.tt.Frame()
	if len(fe.changes) != changes || len(fe.be.cmds) != 1 || len(sv.got) != 1 {
		t.Errorf("the option_set echo caused changes=%d commands=%d saves=%d", len(fe.changes)-changes, len(fe.be.cmds)-1, len(sv.got)-1)
	}
}

func TestChooseSizeKeepsTheFontList(t *testing.T) {
	fe, sv := newMenuEnv(t, Config{FontFamily: "monospace, serif", FontSize: 14})
	fe.ed.ChooseSize(20)
	if got := fe.ed.Font(); got.Size != 20 || got.Family != "monospace, serif" {
		t.Errorf("font %+v, want the same list at size 20", got)
	}
	if want := []fontcfg.Choice{{Size: 20}}; !reflect.DeepEqual(sv.got, want) {
		t.Errorf("saved %+v, want only the size %+v", sv.got, want)
	}
	if want := []string{"let &guifont = 'monospace:h20,serif'"}; !reflect.DeepEqual(fe.be.cmds, want) {
		t.Errorf("commands %q, want %q", fe.be.cmds, want)
	}
	// Family and size are saved independently and both are kept.
	fe.ed.ChooseFamily("serif")
	if want := (fontcfg.Choice{Family: "serif", Size: 20}); sv.got[1] != want {
		t.Errorf("second save %+v, want %+v", sv.got[1], want)
	}
	fe.ed.ChooseSize(12)
	if want := (fontcfg.Choice{Family: "serif", Size: 12}); sv.got[2] != want {
		t.Errorf("third save %+v, want %+v", sv.got[2], want)
	}
}

func TestChoiceWhileGuifontIsSetShowsOnceNvimEchoes(t *testing.T) {
	fe, sv := newMenuEnv(t, Config{FontFamily: "monospace", FontSize: 14})
	setGuiFont(fe.env, "sans-serif:h20") // init.lua set it
	fe.tt.Frame()
	fe.ed.ChooseFamily("serif")
	// guifont still outranks the menu until Neovim has applied our command.
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "sans-serif" {
		t.Errorf("font %+v changed before Neovim echoed the new guifont", got)
	}
	if len(sv.got) != 1 || len(fe.be.cmds) != 1 {
		t.Fatalf("saves=%d commands=%d, want 1 and 1", len(sv.got), len(fe.be.cmds))
	}
	setGuiFont(fe.env, "serif:h20") // the echo
	fe.tt.Frame()
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 20 {
		t.Errorf("font %+v after the echo, want serif at 20 (the size guifont had)", got)
	}
	if len(fe.be.cmds) != 1 {
		t.Errorf("the echo made the editor send %q", fe.be.cmds)
	}
}

func TestChoiceWithoutSaveHookStillWorks(t *testing.T) {
	e := newEnv(t, 400, 200) // no SaveFont hook
	e.ed.ChooseSize(18)
	e.ed.ChooseFamily("serif")
	if got := e.ed.Font(); got.Size != 18 || fontcfg.FirstFamily(got.Family) != "serif" {
		t.Errorf("font %+v", got)
	}
}

func TestSavedChoiceFromAnEarlierRunIsInEffectAtStart(t *testing.T) {
	fe, _ := newMenuEnv(t, Config{FontFamily: "monospace", FontSize: 14, SavedFont: fontcfg.Choice{Family: "serif", Size: 18}})
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 18 {
		t.Errorf("font %+v, want the saved serif:18 to beat the flag", got)
	}
	if len(fe.changes) != 0 {
		t.Errorf("the starting font was announced as a change: %+v", fe.changes)
	}
}

// With a real Neovim: the command the menu sends sets guifont, Neovim echoes
// it back, and that echo changes nothing and starts nothing.
func TestRealNvimChoiceSetsGuifontAndTheEchoIsQuiet(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	dir := t.TempDir()
	var changes []fontcfg.Choice
	var saves []fontcfg.Choice
	var ed *Editor
	ed = NewEditor(nil, Config{FontFamily: "monospace", FontSize: 14, Cols: 80, Rows: 24}, Hooks{
		FontChanged: func(c fontcfg.Choice) { changes = append(changes, c) },
		SaveFont:    func(c fontcfg.Choice) { saves = append(saves, c) },
	})
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
	tt := ui.NewTester(ed.View, 800, 480)
	tt.SetScale(1)
	tt.Frame()
	def, err := sess.OptionDefault("guifont")
	if err != nil {
		t.Fatal(err)
	}
	ed.SetDefaultGuiFont(def)

	guifont := func() (v string) {
		if err := sess.Nvim().Eval("&guifont", &v); err != nil {
			t.Fatal(err)
		}
		return
	}
	wait := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			if cond() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s; font %+v, guifont %q", what, ed.Font(), guifont())
	}

	wait("Neovim's attach-time guifont", func() bool { return echoed(ed, def) || def == "" })
	tt.Frame()
	changes = nil // none are expected, but the count below is of the menu's own

	// A name with an apostrophe and a space goes through the command intact.
	ed.ChooseFamily("It's a font")
	wait("guifont set by the menu", func() bool { return guifont() == "It's a font:h14" })
	wait("Neovim's echo", func() bool { return echoed(ed, "It's a font:h14") })
	for i := 0; i < 5; i++ { // more frames, in case an echo loop was going to start
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if got := ed.Font(); got.Family != "It's a font, monospace" || got.Size != 14 {
		t.Errorf("font %+v, want It's a font at 14", got)
	}
	if len(changes) != 1 {
		t.Errorf("%d font changes (%+v), want exactly 1: the echo must not count", len(changes), changes)
	}
	if len(saves) != 1 {
		t.Errorf("%d saves, want 1", len(saves))
	}

	ed.ChooseSize(19)
	wait("size 19 in guifont", func() bool { return guifont() == "It's a font:h19,monospace" })
	wait("size 19 in use", func() bool { return ed.Font().Size == 19 })
	for i := 0; i < 5; i++ {
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if len(changes) != 2 || len(saves) != 2 {
		t.Errorf("changes=%d saves=%d after two choices, want 2 and 2", len(changes), len(saves))
	}
}
