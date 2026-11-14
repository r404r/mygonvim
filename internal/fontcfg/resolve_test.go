package fontcfg

import "testing"

func TestResolveTakesEachFieldFromTheFirstSourceThatHasIt(t *testing.T) {
	def := Choice{Family: "Default", Size: 14}
	flag := Choice{Family: "Flag", Size: 15}
	saved := Choice{Family: "Saved", Size: 16}
	gui := Choice{Family: "Gui", Size: 17}
	cases := []struct {
		name                  string
		gui, saved, flag, def Choice
		want                  Choice
	}{
		{"guifont wins over everything", gui, saved, flag, def, gui},
		{"saved wins over flag", Choice{}, saved, flag, def, saved},
		{"flag wins over default", Choice{}, Choice{}, flag, def, flag},
		{"default is last", Choice{}, Choice{}, Choice{}, def, def},
		{"an empty guifont does not override the flag", Choice{}, Choice{}, flag, def, flag},
		{"guifont without size keeps the saved size", Choice{Family: "Gui"}, saved, flag, def, Choice{Family: "Gui", Size: 16}},
		{"guifont size alone keeps the saved family", Choice{Size: 20}, saved, flag, def, Choice{Family: "Saved", Size: 20}},
		{"saved family with flag size", Choice{}, Choice{Family: "Saved"}, flag, def, Choice{Family: "Saved", Size: 15}},
		{"flag without size takes the default size", Choice{}, Choice{}, Choice{Family: "Flag"}, def, Choice{Family: "Flag", Size: 14}},
		{"nothing at all", Choice{}, Choice{}, Choice{}, Choice{}, Choice{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.gui, c.saved, c.flag, c.def); got != c.want {
				t.Errorf("Resolve = %+v, want %+v", got, c.want)
			}
		})
	}
}
