# MyGoNVim

A [Neovim](https://neovim.io) GUI drawn with [MyGo](https://github.com/egoist/mygo)'s native UI: Go only, no cgo, no web view. Windows and macOS are the targets; Linux works for development.

It runs `nvim --embed`, keeps Neovim's cell grid in memory, and draws it cell by cell with MyGo's `ui.Shape` and `Painter.Glyphs`.

**Status: MVP step 1.** Opens files, edits, saves; syntax colors, CJK and emoji, cursor shapes per mode, window title, window resize, Ctrl/Alt/special keys, input-method composition drawn at the cursor. Not yet: mouse, clipboard shortcuts, high-DPI polish, command-line/popup-menu/tab-line/multigrid UI, packaging scripts. Only Linux has been run on a real screen so far; macOS and Windows build but are untested on real machines. Design notes are maintained separately.

## Run

Needs Go 1.27.3 or later (what MyGo requires) and Neovim 0.10 or later on `PATH` (or pass `-nvim`).

```sh
# macOS
brew install neovim go
git clone https://github.com/r404r/MyGoNVim && cd MyGoNVim
go run . path/to/file.txt          # or: go tool mygo dev
```

```
usage: mygonvim [flags] [file ...]
  -nvim string        path of the nvim executable (default: $MYGONVIM_NVIM, ./nvim/bin, PATH)
  -font string        font family list, e.g. "JetBrains Mono, monospace"
  -font-size float    font size in DIPs (default 14)
  -width, -height     window size in DIPs (default 1000x650)
```

Where nvim is looked for: `-nvim`, `$MYGONVIM_NVIM`, `nvim/bin/nvim[.exe]` or `nvim/nvim[.exe]` next to the program (and in the app's resources folder), `PATH`, then `/opt/homebrew/bin` and `/usr/local/bin` on macOS.

Closing the window runs `:confirm qa`: Neovim asks about unsaved changes on its own command line, and the window closes when Neovim has quit.

## Portable mode

If a `data/` folder sits next to the program, Neovim's `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` and `XDG_CACHE_HOME` point into `data/config`, `data/share`, `data/state` and `data/cache`, and the app's own user data goes to `data/mygonvim`. A Windows portable folder is then:

```
MyGoNVim.exe        go tool mygo build -platform windows/amd64 -skip-installer
nvim/bin/nvim.exe   unzip of Neovim's nvim-win64.zip (nvim/share, nvim/lib too)
data/               your config and plugins live here
```

`nvim.exe` is started without a console window. A clean Windows 10 may need the Visual C++ runtime (`VCRUNTIME140.dll`) beside `nvim.exe`.

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
