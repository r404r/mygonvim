package gui

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/r404r/mygonvim/internal/fontcfg"
	"github.com/r404r/mygonvim/internal/grid"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

// fontEnv is an editor in a headless window that tells which fonts it took.
type fontEnv struct {
	*env
	changes []fontcfg.Choice
}

func newFontEnv(t *testing.T, cfg Config) *fontEnv {
	t.Helper()
	fe := &fontEnv{env: &env{be: &fake{}}}
	cfg.Cols, cfg.Rows = 80, 24
	fe.ed = NewEditor(fe.be, cfg, Hooks{
		Invalidate:  func() { fe.inv++ },
		FontChanged: func(c fontcfg.Choice) { fe.changes = append(fe.changes, c) },
	})
	fe.ed.SetDefaultGuiFont("") // Neovim's default is known; here it is empty
	fe.tt = ui.NewTester(fe.ed.View, 640, 400)
	fe.tt.SetScale(1)
	fe.tt.Frame()
	fe.inv = 0
	return fe
}

func setGuiFont(e *env, value string) {
	e.ed.OnRedraw(ev("option_set", []any{"guifont", value}), ev("flush", []any{}))
}

// advance is the cell width the editor should measure for a font at scale 1.
func advance(family string, size float32) float32 {
	g := ui.Shape("0", ui.Font{Family: family, Size: size})
	if len(g) == 0 {
		return 0
	}
	return float32(math.Round(float64(g[0].Advance)))
}

func TestSetFontMeasuresTheCellsAgain(t *testing.T) {
	e := newEnv(t, 640, 400)
	cw, ch := e.ed.cw, e.ed.ch
	e.ed.SetFont("monospace", 28)
	e.tt.Frame()
	if e.ed.cw <= cw*1.5 || e.ed.ch <= ch*1.5 {
		t.Errorf("cell %.0fx%.0f after doubling the size, was %.0fx%.0f", e.ed.cw, e.ed.ch, cw, ch)
	}
	if want := advance("monospace", 28); e.ed.cw != want {
		t.Errorf("cell width %.0f, want the new font's advance %.0f", e.ed.cw, want)
	}
}

// Same scale, other family: the old measure was cached by scale alone before.
func TestSetFontWithOnlyAnotherFamilyMeasuresAgain(t *testing.T) {
	families := []string{"monospace", "sans-serif", "serif"}
	for _, f := range ui.FontFamilies() {
		families = append(families, f.Name)
	}
	var a, b string
	for _, x := range families {
		for _, y := range families {
			if a == "" && advance(x, 24) != 0 && advance(y, 24) != 0 && advance(x, 24) != advance(y, 24) {
				a, b = x, y
			}
		}
	}
	if a == "" {
		t.Skip("this machine has no two fonts whose digits differ in width")
	}
	e := newEnv(t, 640, 400)
	e.ed.SetFont(a, 24)
	e.tt.Frame()
	if want := advance(a, 24); e.ed.cw != want {
		t.Fatalf("cell width %.0f in %q, want %.0f", e.ed.cw, a, want)
	}
	e.ed.SetFont(b, 24)
	e.tt.Frame()
	if want := advance(b, 24); e.ed.cw != want {
		t.Errorf("cell width %.0f after switching %q to %q at the same scale, want %.0f: the old measure was kept", e.ed.cw, a, b, want)
	}
}

func TestSetFontAsksNvimForTheGridThatFits(t *testing.T) {
	e := newEnv(t, 640, 400)
	before := e.be.lastResize()
	e.ed.SetFont("monospace", 28)
	e.tt.Frame()
	want := [2]int{int(640 / e.ed.cw), int(400 / e.ed.ch)}
	got := e.be.lastResize()
	if got != want || got == before {
		t.Errorf("resize request %v after a bigger font, want %v (was %v)", got, want, before)
	}
	n := len(e.be.resizes)
	e.ed.SetFont("monospace", 28) // nothing changes in the cells
	e.tt.Frame()
	if len(e.be.resizes) != n {
		t.Errorf("a font that gives the same cells asked for a resize again")
	}
}

func TestSetFontRequestsAFrame(t *testing.T) {
	e := newEnv(t, 640, 400)
	before := e.inv
	e.ed.SetFont("monospace", 20)
	if e.inv != before+1 {
		t.Errorf("frames requested: %d, want 1", e.inv-before)
	}
}

func TestGuiFontOptionChangesTheFont(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 14})
	cw := fe.ed.cw
	setGuiFont(fe.env, "monospace:h28")
	fe.tt.Frame()
	if got := fe.ed.Font(); got.Size != 28 || fontcfg.FirstFamily(got.Family) != "monospace" {
		t.Errorf("font = %+v, want monospace at 28", got)
	}
	if fe.ed.cw <= cw*1.5 {
		t.Errorf("cell width %.0f, was %.0f: the grid does not follow the font", fe.ed.cw, cw)
	}
	if n := len(fe.changes); n != 1 || fe.changes[0].Size != 28 {
		t.Errorf("font change notifications %+v, want one at size 28", fe.changes)
	}
}

func TestGuiFontIsAppliedOnTheFrameNotOnNvimsGoroutine(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 14})
	setGuiFont(fe.env, "monospace:h28")
	if got := fe.ed.Font(); got.Size != 14 {
		t.Errorf("the font changed to %+v before a frame: it must change on the main thread", got)
	}
	if fe.inv == 0 {
		t.Error("no frame was requested for the new font")
	}
	fe.tt.Frame()
	if got := fe.ed.Font(); got.Size != 28 {
		t.Errorf("font %+v after the frame, want size 28", got)
	}
}

func TestEmptyGuiFontDoesNotOverrideTheFlag(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 17})
	want := fe.ed.Font()
	if want.Size != 17 {
		t.Fatalf("start font %+v, want size 17 from the flag", want)
	}
	// Neovim reports guifont="" when it attaches.
	setGuiFont(fe.env, "")
	fe.tt.Frame()
	if got := fe.ed.Font(); got != want || len(fe.changes) != 0 {
		t.Errorf("an empty guifont changed the font to %+v (changes %+v)", got, fe.changes)
	}
	// Set and cleared again: the flag's font comes back.
	setGuiFont(fe.env, "monospace:h30")
	fe.tt.Frame()
	if fe.ed.Font().Size != 30 {
		t.Fatalf("guifont did not apply: %+v", fe.ed.Font())
	}
	setGuiFont(fe.env, "")
	fe.tt.Frame()
	if got := fe.ed.Font(); got != want {
		t.Errorf("after clearing guifont the font is %+v, want the flag's %+v", got, want)
	}
}

func TestGuiFontWithoutSizeKeepsTheOtherSourcesSize(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 17})
	setGuiFont(fe.env, "serif")
	fe.tt.Frame()
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 17 {
		t.Errorf("font %+v, want serif at the flag's size 17", got)
	}
}

func TestFontSourcesOrder(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 12, SavedFont: fontcfg.Choice{Family: "serif", Size: 16}})
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 16 {
		t.Fatalf("start font %+v, want the saved menu choice to beat the flag", got)
	}
	setGuiFont(fe.env, "sans-serif:h20")
	fe.tt.Frame()
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "sans-serif" || got.Size != 20 {
		t.Errorf("font %+v, want guifont to beat the saved choice", got)
	}
	// While guifont is set, a new menu choice is remembered but does not show.
	fe.ed.SetSavedFont(fontcfg.Choice{Family: "monospace", Size: 18})
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "sans-serif" || got.Size != 20 {
		t.Errorf("font %+v changed under a guifont that is still set", got)
	}
	setGuiFont(fe.env, "")
	fe.tt.Frame()
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "monospace" || got.Size != 18 {
		t.Errorf("font %+v after clearing guifont, want the saved choice monospace:18", got)
	}
}

func TestSetSavedFontAppliesAtOnceWithoutTellingNvim(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 12})
	fe.ed.SetSavedFont(fontcfg.Choice{Family: "serif", Size: 20})
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "serif" || got.Size != 20 {
		t.Errorf("font %+v, want serif:20", got)
	}
	fe.tt.Frame()
	if len(fe.be.cmds) != 0 || len(fe.be.keys) != 0 {
		t.Errorf("the editor sent %q %q to nvim by itself", fe.be.cmds, fe.be.keys)
	}
}

// What nvim echoes back after the app set guifont must not change anything.
func TestEchoedGuiFontIsNotAChange(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 12})
	fe.ed.SetSavedFont(fontcfg.Choice{Family: "serif", Size: 20})
	fe.tt.Frame()
	changes, resizes, cmds := len(fe.changes), len(fe.be.resizes), len(fe.be.cmds)
	setGuiFont(fe.env, "serif:h20") // option_set after ":let &guifont = ..."
	fe.tt.Frame()
	fe.tt.Frame()
	if len(fe.changes) != changes || len(fe.be.resizes) != resizes || len(fe.be.cmds) != cmds {
		t.Errorf("the echo caused %d font changes, %d resizes, %d commands",
			len(fe.changes)-changes, len(fe.be.resizes)-resizes, len(fe.be.cmds)-cmds)
	}
}

// Since Neovim 0.12 guifont is never empty: it starts as a list for the
// platform. That list is not a choice and must not beat -font.
func TestNvimsDefaultGuiFontIsNoChoice(t *testing.T) {
	const def = "Source Code Pro,DejaVu Sans Mono,Courier New,monospace"
	fe := newFontEnv(t, Config{FontFamily: "serif", FontSize: 15})
	fe.ed.SetDefaultGuiFont(def)
	setGuiFont(fe.env, def) // option_set at attach
	fe.tt.Frame()
	if got := fe.ed.Font(); got.Family != "serif" || got.Size != 15 || len(fe.changes) != 0 {
		t.Errorf("font %+v (changes %+v): Neovim's default guifont replaced -font", got, fe.changes)
	}
	setGuiFont(fe.env, "Fira Code:h20") // init.lua
	fe.tt.Frame()
	if got := fe.ed.Font(); fontcfg.FirstFamily(got.Family) != "Fira Code" || got.Size != 20 {
		t.Errorf("font %+v, want a guifont that is not the default to apply", got)
	}
	setGuiFont(fe.env, def) // back to the default
	fe.tt.Frame()
	if got := fe.ed.Font(); got.Family != "serif" || got.Size != 15 {
		t.Errorf("font %+v after setting the default again, want -font's serif:15", got)
	}
}

// The option_set of the default may come before or after we learn the default.
func TestGuiFontIsNotUsedUntilTheDefaultIsKnown(t *testing.T) {
	const def = "Source Code Pro,DejaVu Sans Mono,Courier New,monospace"
	e := newEnv(t, 640, 400)
	e.ed.OnRedraw(ev("option_set", []any{"guifont", def}), ev("flush", []any{}))
	e.tt.Frame()
	if got := e.ed.Font(); got.Family != "monospace" || got.Size != 14 {
		t.Errorf("font %+v: guifont was used before the default was known", got)
	}
	e.ed.OnRedraw(ev("option_set", []any{"guifont", "Fira Code:h20"}), ev("flush", []any{}))
	e.tt.Frame()
	if got := e.ed.Font(); got.Size != 14 {
		t.Errorf("font %+v: guifont applied before the default was known", got)
	}
	e.ed.SetDefaultGuiFont(def)
	e.tt.Frame()
	if got := e.ed.Font(); fontcfg.FirstFamily(got.Family) != "Fira Code" || got.Size != 20 {
		t.Errorf("font %+v, want the guifont that arrived earlier to apply once the default is known", got)
	}
}

// Neovim's goroutine sets guifont while the main thread draws.
func TestGuiFontFromAnotherGoroutineIsRaceFree(t *testing.T) {
	fe := newFontEnv(t, Config{FontFamily: "monospace", FontSize: 14})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			setGuiFont(fe.env, []string{"monospace:h12", "monospace:h20", ""}[i%3])
		}
	}()
	for i := 0; i < 50; i++ {
		fe.tt.Frame()
	}
	wg.Wait()
	fe.tt.Frame()
	if got := fe.ed.Font(); got.Size != 14 && got.Size != 12 && got.Size != 20 {
		t.Errorf("font %+v is none of the ones that were set", got)
	}
}

// The real thing: ":set guifont" in a real Neovim reaches the editor as an
// option_set event, and clearing it brings back the font the app started with.
func TestRealNvimSetGuifont(t *testing.T) {
	bin := os.Getenv("NVIM_BIN")
	if bin == "" {
		var err error
		if bin, err = nvimproc.Locate("", "", nil); err != nil {
			t.Skip("no nvim: set NVIM_BIN or put nvim on PATH")
		}
	}
	dir := t.TempDir()
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
	tt := ui.NewTester(ed.View, 800, 480)
	tt.SetScale(1)
	tt.Frame()
	def, err := sess.OptionDefault("guifont")
	if err != nil {
		t.Fatal(err)
	}
	ed.SetDefaultGuiFont(def)

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
		t.Fatalf("timed out waiting for %s; font is %+v", what, ed.Font())
	}
	// Neovim has told us its default guifont by now; it must not have replaced
	// the font the app started with.
	wait("Neovim's attach-time guifont", func() bool { return echoed(ed, def) || def == "" })
	tt.Frame()
	if got := ed.Font(); got.Size != 14 || got.Family != "monospace" {
		t.Fatalf("start font %+v, want monospace at 14, whatever Neovim's default guifont %q is", got, def)
	}
	cw := ed.cw

	sess.Command("set guifont=monospace:h28")
	wait("size 28", func() bool { return ed.Font().Size == 28 })
	if ed.cw <= cw*1.5 {
		t.Errorf("cell width %.0f after :set guifont=...:h28, was %.0f", ed.cw, cw)
	}
	wantCols := int(800 / ed.cw)
	wait("a grid that fits the bigger cells", func() bool { c, _ := ed.Model.Size(); return c == wantCols })

	sess.Command("set guifont=")
	wait("the start font again", func() bool { return ed.Font().Size == 14 })
}

// echoed reports whether Neovim's guifont, as the editor last heard it, is value.
func echoed(ed *Editor, value string) bool {
	var got string
	ed.Model.Read(func(s *grid.State) { got = s.GuiFont })
	return got == value
}
