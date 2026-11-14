package main

import (
	"bytes"
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

func TestParseOptionsVersion(t *testing.T) {
	for _, args := range [][]string{{"-version"}, {"--version"}} {
		o, err := parseOptions(args, io.Discard)
		if err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if !o.printVersion {
			t.Errorf("%q: printVersion is false", args)
		}
	}
	o, err := parseOptions([]string{"-nvim", "/x/nvim"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.printVersion {
		t.Error("printVersion set without -version")
	}
}

func TestWriteVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "0.0.1"
	var buf bytes.Buffer
	writeVersion(&buf)
	if got, want := buf.String(), "MyGoNVim 0.0.1\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	version = "dev"
	buf.Reset()
	writeVersion(&buf)
	// `go test` does not link a mygo.json version, so this is the default.
	if got, want := buf.String(), "MyGoNVim dev\n"; got != want {
		t.Fatalf("default: got %q, want %q", got, want)
	}
}
