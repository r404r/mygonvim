// Package fluent supplies the Windows 11 window chrome and shared button look.
package fluent

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const TitleBarHeight = 32

func UseChrome(goos string, build uint32) bool { return goos == "windows" && build >= 22000 }

// ConfigureWindow sets material at creation time, when the backing surface is chosen.
func ConfigureWindow(opts mygo.WindowOptions, enabled bool) mygo.WindowOptions {
	if enabled {
		opts.TitleBarStyle = mygo.TitleBarHidden
		opts.TitleBarHeight = TitleBarHeight
		opts.Vibrancy = mygo.VibrancyMica
	}
	return opts
}

// InstallMenu leaves Windows 11 without an application menu or accelerator table.
func InstallMenu(enabled bool, menu *mygo.Menu, install func(*mygo.Menu)) {
	if !enabled {
		install(menu)
	}
}

func titleBarFill(vibrancy, reduceTransparency, dark bool) (bool, ui.Color) {
	if vibrancy && !reduceTransparency {
		return false, ui.Color{}
	}
	if dark {
		return true, ui.Color{R: 32, G: 32, B: 32, A: 255}
	}
	return true, ui.Color{R: 243, G: 243, B: 243, A: 255}
}

func appearanceFrom(background string) mygo.ThemeSource {
	switch background {
	case "dark":
		return mygo.ThemeDark
	case "light":
		return mygo.ThemeLight
	default:
		return mygo.ThemeSystem
	}
}

// ApplyAppearance is called on the UI thread after a background option change.
func ApplyAppearance(background string, app, buttons func(mygo.ThemeSource)) {
	source := appearanceFrom(background)
	app(source)
	buttons(source)
}
