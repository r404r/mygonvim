package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAboutIconIsEmbeddedRepositoryImage(t *testing.T) {
	want, err := os.ReadFile("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(aboutIcon, want) {
		t.Fatal("embedded icon differs from repository image")
	}
	src, err := ui.DecodeImage(aboutIcon)
	if err != nil {
		t.Fatal(err)
	}
	w, h := ui.ImageSize(src)
	if w <= 0 || h <= 0 {
		t.Fatal("empty icon")
	}
}
