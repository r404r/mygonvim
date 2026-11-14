package main

import (
	"reflect"
	"testing"

	"github.com/r404r/mygonvim/internal/fontcfg"
)

func TestInitCmdsPresetsOnlyASavedFamily(t *testing.T) {
	if got := initCmds(fontcfg.Choice{}); got != nil {
		t.Errorf("nothing saved: %q, want no commands", got)
	}
	if got := initCmds(fontcfg.Choice{Size: 18}); got != nil {
		t.Errorf("a size alone: %q, want no commands", got)
	}
	want := []string{"let &guifont = 'Fira Code:h14'"}
	if got := initCmds(fontcfg.Choice{Family: "Fira Code", Size: 14}); !reflect.DeepEqual(got, want) {
		t.Errorf("a saved font: %q, want %q", got, want)
	}
}
