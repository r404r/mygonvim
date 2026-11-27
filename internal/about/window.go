package about

import (
	"context"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/r404r/mygonvim/internal/fluent"
)

type Window interface {
	Show()
	Focus()
	Restore()
	Invalidate()
	Close()
}
type Factory func(view func(*ui.Context), closed func()) Window

type Controller struct {
	Fluent    bool
	Process   Process
	Icon      ui.ImageSource
	Query     func(context.Context) (Info, error)
	Services  Services
	NewWindow Factory
	Dispatch  func(func())
	window    Window
	model     *viewModel
}

type viewModel struct {
	fluent       bool
	process      Process
	icon         ui.ImageSource
	paths        []Path
	nvim, status string
	services     Services
	dispatch     func(func())
	invalidate   func()
	close        func()
}

// UpdateProcess runs on the UI thread, including while About is already open.
func (c *Controller) UpdateProcess(p Process) {
	c.Process = p
	if m := c.model; m != nil {
		m.process = p
		for i := range m.paths {
			switch m.paths[i].Label {
			case settingsLabel:
				m.paths[i].Value = p.Settings
			case logLabel:
				m.paths[i].Value = p.Log
			case nvimLabel:
				m.paths[i].Value = p.Nvim
			}
		}
		m.invalidate()
	}
}

// Open is called on the UI thread. It never waits for Neovim.
func (c *Controller) Open() {
	if c.window != nil {
		c.window.Restore()
		c.window.Show()
		c.window.Focus()
		return
	}
	m := &viewModel{fluent: c.Fluent, process: c.Process, icon: c.Icon, nvim: "Loading…", services: c.Services, dispatch: c.Dispatch}
	loading := Info{Errors: map[string]string{}}
	for _, key := range []string{"config", "data", "state", "cache"} {
		loading.Errors[key] = "Loading…"
	}
	m.paths = Paths(c.Process, loading, os.Stat)
	c.model = m
	w := c.NewWindow(m.view, func() { c.window = nil; c.model = nil })
	c.window = w
	m.invalidate = w.Invalidate
	m.close = w.Close
	process := c.Process
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		info, err := c.Query(ctx)
		if err != nil {
			info = Info{Errors: map[string]string{}}
			for _, key := range []string{"config", "data", "state", "cache"} {
				info.Errors[key] = err.Error()
			}
		}
		paths := Paths(process, info, os.Stat)
		c.Dispatch(func() {
			if c.model != m {
				return
			}
			m.nvim = info.Version
			if m.nvim == "" {
				m.nvim = "Unavailable"
				if err != nil {
					m.nvim += ": " + err.Error()
				}
			}
			m.paths = paths
			c.UpdateProcess(c.Process)
		})
	}()
}

func (m *viewModel) act(action string, path Path) {
	go func() {
		status := m.services.Act(action, path)
		m.dispatch(func() { m.status = status; m.invalidate() })
	}()
}

func (m *viewModel) link(url string) {
	go func() {
		err := m.services.OpenExternal(url)
		m.dispatch(func() {
			m.status = "Opened browser."
			if err != nil {
				m.status = "Open link failed: " + err.Error()
			}
			m.invalidate()
		})
	}()
}

func (m *viewModel) view(c *ui.Context) {
	c.OnWindowShortcut(0, ui.KeyEscape, m.close)
	ui.Column(c).Fill().Padding(20).Gap(10).Children(func() {
		ui.Row(c).Gap(16).Children(func() {
			if m.icon != nil {
				ui.Image(c, m.icon).Size(72, 72)
			}
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "MyGoNVim "+m.process.Version).FontSize(22).Bold()
				ui.Text(c, "Author: r404r")
				ui.Text(c, "MIT · Copyright (c) 2026 r404r")
			})
		})
		ui.Row(c).Gap(10).Children(func() {
			m.button(c, "GitHub").OnClick(func() { m.link(Website) })
			m.button(c, "License").OnClick(func() { m.link(LicenseURL) })
		})
		ui.Text(c, Website).Selectable()
		portable := "No"
		if m.process.Portable {
			portable = "Yes"
		}
		ui.Text(c, "Portable mode: "+portable)
		ui.Text(c, "Login shell environment: "+m.process.LoginEnv).FillWidth().Selectable()
		ui.Text(c, "Neovim: "+m.nvim).FillWidth().Selectable()
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(14).Children(func() {
				for _, p := range m.paths {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, p.Label).Bold()
						value := p.Value
						if value == "" {
							value = "Unavailable"
							if p.Error != "" {
								if p.Error == "Loading…" {
									value = p.Error
								} else {
									value += ": " + p.Error
								}
							}
						}
						// Constrained text wraps by default; selection keeps the complete path.
						ui.Text(c, value).FillWidth().Selectable()
						if p.Value != "" {
							ui.Row(c).Gap(8).Children(func() {
								m.button(c, "Copy").OnClick(func() { m.act("copy", p) })
								m.button(c, "Show in folder").OnClick(func() { m.act("reveal", p) })
								m.button(c, "Open directory").OnClick(func() { m.act("open", p) })
							})
						}
					})
				}
			})
		})
		ui.Text(c, m.status).FillWidth().Selectable()
	})
}

func (m *viewModel) button(c *ui.Context, label string) ui.Element {
	b := ui.Button(c, label)
	if m.fluent {
		fluent.StyleButton(c, b)
	}
	return b
}
