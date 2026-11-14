package main

import (
	"flag"
	"fmt"
	"io"
)

// options are the command line's.
type options struct {
	nvim     string  // -nvim: the nvim executable
	font     string  // -font: a font family list
	fontSize float64 // -font-size, in DIPs
	width    int     // -width and -height: the window's size in DIPs
	height   int
	files    []string // what is left: files for nvim to open; put "--" before names that start with "-"
}

func parseOptions(args []string, out io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("mygonvim", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&o.nvim, "nvim", "", "path of the nvim executable (default: $MYGONVIM_NVIM, ./nvim/bin, PATH)")
	fs.StringVar(&o.font, "font", "", "font family list, e.g. \"JetBrains Mono, monospace\" (default: the system's monospace)")
	fs.Float64Var(&o.fontSize, "font-size", 14, "font size in DIPs")
	fs.IntVar(&o.width, "width", 1000, "window width in DIPs")
	fs.IntVar(&o.height, "height", 650, "window height in DIPs")
	fs.Usage = func() {
		fmt.Fprintf(out, "usage: mygonvim [flags] [file ...]\n\nA Neovim GUI drawn with MyGo's native UI. Needs Neovim 0.10 or later.\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if o.fontSize < 6 || o.fontSize > 96 {
		return o, fmt.Errorf("-font-size %v: want 6 to 96", o.fontSize)
	}
	if o.width < 200 || o.height < 120 {
		return o, fmt.Errorf("-width/-height %dx%d: too small", o.width, o.height)
	}
	o.files = fs.Args()
	return o, nil
}
