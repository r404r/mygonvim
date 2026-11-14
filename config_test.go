package main

import (
	"io"
	"slices"
	"testing"
)

func TestParseOptions(t *testing.T) {
	o, err := parseOptions([]string{"-nvim", "/x/nvim", "-font", "Menlo", "-font-size", "16", "a.txt", "--", "-weird"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.nvim != "/x/nvim" || o.font != "Menlo" || o.fontSize != 16 || o.width != 1000 || o.height != 650 {
		t.Errorf("%+v", o)
	}
	// After the first non-flag, everything is a file: nvim's own flags are not passed through.
	if want := []string{"a.txt", "--", "-weird"}; !slices.Equal(o.files, want) {
		t.Errorf("files %q, want %q", o.files, want)
	}
}

func TestParseOptionsRejectsNonsense(t *testing.T) {
	for _, args := range [][]string{{"-font-size", "2"}, {"-width", "10"}, {"-nope"}} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Errorf("%q accepted", args)
		}
	}
}
