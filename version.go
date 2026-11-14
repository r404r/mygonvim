package main

import (
	"fmt"
	"io"

	"github.com/egoist/mygo"
)

// version is the build version printed by -version. Release builds set it
// with -ldflags="-X main.version=0.1.0" (or MYGO_LDFLAGS for
// `go tool mygo build`). The default is "dev" for `go run` and tests.
var version = "dev"

func displayedVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	// `go tool mygo build` also links mygo.json's version into App.Version().
	if v := mygo.App.Version(); v != "" {
		return v
	}
	if version != "" {
		return version
	}
	return "dev"
}

func writeVersion(w io.Writer) {
	fmt.Fprintln(w, appTitle+" "+displayedVersion())
}
