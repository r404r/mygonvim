# MyGoNVim

A [Neovim](https://neovim.io) GUI drawn with [MyGo](https://github.com/egoist/mygo)'s native UI: Go only, no cgo, no web view. Windows and macOS are the targets; Linux works for development.

It runs `nvim --embed`, keeps Neovim's cell grid in memory, and draws it cell by cell with MyGo's `ui.Shape` and `Painter.Glyphs`.

**Status: MVP step 1 (partial), version 0.0.1.** Linux has been verified (unit and integration tests, a real window, and CI): it opens files, edits and saves; syntax colors, CJK and emoji, cursor shapes per mode, window title, window resize, Ctrl/Alt/special keys. Input-method composition is drawn at the cursor in headless tests; real IME on macOS and Windows is untested. macOS and Windows compile, pass CI unit tests, and have unsigned preview builds, but have not been run on a real machine, so MVP 1 is not done. Not yet: mouse, clipboard shortcuts, high-DPI polish, command-line/popup-menu/tab-line/multigrid UI, a bundled Neovim, or signed/notarized packages. Design notes are maintained separately.

## Prebuilt builds

`mygo.json` is version `0.0.1`. Pushing a `v*` tag that matches that version runs [`.github/workflows/release.yml`](.github/workflows/release.yml), which attaches macOS arm64 and amd64 `.app` zips and `.dmg` files, and a Windows amd64 portable zip (an empty `data/` folder included; Neovim is not bundled), to a **draft** GitHub Release. A draft for v0.0.1 already exists; it has not been published, so the assets are not on the public Releases page yet.

Those builds are unsigned. The macOS apps are ad hoc signed and not notarized; Gatekeeper will likely block them on other Macs. The builds declare macOS 13.0 as the minimum (`macos.minimumSystemVersion` in `mygo.json`).

## Run

Needs Go 1.27.3 or later (what MyGo requires) and Neovim on `PATH` (or pass `-nvim`). Developed and tested with Neovim 0.12 (v0.12.6 in our tests); older versions are untested.

```sh
# macOS
brew install neovim go
git clone https://github.com/r404r/mygonvim && cd mygonvim
go run . path/to/file.txt          # or: go tool mygo dev
```

```
usage: mygonvim [flags] [file ...]
  -nvim string        path of the nvim executable (default: $MYGONVIM_NVIM, ./nvim/bin, PATH)
  -font string        font family list, e.g. "JetBrains Mono, monospace"
  -font-size float    font size in DIPs (default 14)
  -width, -height     window size in DIPs (default 1000x650)
  -version            print the version and exit
```

`mygonvim -version` prints `MyGoNVim <version>`. `go run` and tests use `dev`. A release build sets the version from `mygo.json` (`go tool mygo build` links it into the app) and with `-ldflags="-X main.version=…"` or the `MYGO_LDFLAGS` environment variable (which `go tool mygo build` appends). The two should be the same string, without a leading `v`.

Where nvim is looked for: `-nvim`, `$MYGONVIM_NVIM`, `nvim/bin/nvim[.exe]` or `nvim/nvim[.exe]` next to the program (and in the app's resources folder), `PATH`, then `/opt/homebrew/bin` and `/usr/local/bin` on macOS.

Closing the window runs `:confirm qa`: Neovim asks about unsaved changes on its own command line, and the window closes when Neovim has quit.

## Portable mode

If a `data/` folder sits next to the program, Neovim's `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` and `XDG_CACHE_HOME` point into `data/config`, `data/share`, `data/state` and `data/cache`, and the app's own user data goes to `data/mygonvim`.

`go run` puts the executable in a temporary directory, so a `data/` folder in the source tree does not enable portable mode. Build a binary first (`go build -o mygonvim .` or `go tool mygo build`) and place `data/` next to that binary. For a macOS `.app`, the executable lives in `Contents/MacOS/`; whether `data/` should sit there or next to the `.app` has not been verified.

A Windows portable folder is then:

```
MyGoNVim.exe        go tool mygo build -platform windows/amd64 -skip-installer
nvim/bin/nvim.exe   unzip of Neovim's nvim-win64.zip (nvim/share, nvim/lib too)
data/               your config and plugins live here
```

On Windows the process is started with `CREATE_NO_WINDOW`, which is intended to avoid a console flash; that has not been checked on a real machine. A clean Windows 10 may need the Visual C++ runtime (`VCRUNTIME140.dll`) beside `nvim.exe` (also unverified on a clean VM).

## Develop

```sh
go vet ./... && go test ./...                     # the window tests use a headless MyGo window
NVIM_BIN=/path/to/nvim go test ./...              # also runs the tests against a real nvim --embed
MYGONVIM_SCREENSHOT=shot.png NVIM_BIN=... go test -run RealNvim ./internal/gui
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build .
```

Layout: `internal/grid` (screen model from `redraw` events), `internal/input` (keys and IME to `nvim_input` notation), `internal/nvimproc` (find, start and talk to nvim), `internal/gui` (the MyGo view that draws the grid). MyGo is pinned in `go.mod` to the release v0.9.0.

## License

MIT
