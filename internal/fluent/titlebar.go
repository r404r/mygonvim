package fluent

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TitleBar reads the same objects used by the standard application menu.
// Its fields are owned by the UI thread.
type TitleBar struct {
	Items   []*mygo.MenuItem
	Window  *mygo.Window
	open    []bool
	hadMenu bool
}

// View builds the title row and reports when focus should return to the editor.
func (b *TitleBar) View(c *ui.Context) bool {
	c.SetMenuPresentation(ui.PopoverMenus)
	// The toolkit otherwise paints its default root background over the material.
	if c.Vibrancy() && !c.Preferences().ReduceTransparency {
		c.Root().Background(ui.Transparent)
	}
	tb := c.TitleBar()
	if tb.Height <= 0 {
		clear(b.open)
		closed := b.hadMenu
		b.hadMenu = false
		return closed
	}
	if len(b.open) != len(b.Items) {
		b.open = make([]bool, len(b.Items))
	}
	row := ui.Row(c).FillWidth().Height(tb.Height).Shrink(0).AlignItems(ui.Center).
		Padding(0, tb.Right, 0, max(tb.Left, 8)).Gap(4).DragWindow().Label("Title bar")
	if paint, color := titleBarFill(c.Vibrancy(), c.Preferences().ReduceTransparency, c.Theme().Dark); paint {
		row.Background(color)
	}
	titleFocused := false
	row.Children(func() {
		buttons := make([]ui.Element, len(b.Items))
		anyOpen := false
		for _, open := range b.open {
			anyOpen = anyOpen || open
		}
		hover := -1
		for i, item := range b.Items {
			if !item.IsVisible() {
				continue
			}
			button := ui.Button(c.Key(item.Label), "").Label(item.Label).Height(24).Padding(0, 10).Shrink(1).Disabled(!item.IsEnabled())
			button.Children(func() { ui.Text(c, item.Label).FontSize(12).SingleLine() })
			buttons[i] = button
			if _, _, over := button.PointerPosition(); over && item.IsEnabled() {
				hover = i
			}
		}
		// Decide before building any menu, so switching works in either direction.
		if anyOpen && hover >= 0 && !b.open[hover] {
			clear(b.open)
			b.open[hover] = true
		}
		for i, item := range b.Items {
			if !item.IsVisible() {
				continue
			}
			button := buttons[i]
			button.PopoverMenuButton(func(m *ui.Menu) { b.menu(m, item.Submenu) }, ui.PopoverMenuButtonOptions{Open: &b.open[i]})
			StyleButton(c, button)
			titleFocused = titleFocused || button.Focused()
		}
	})
	open := false
	for _, on := range b.open {
		open = open || on
	}
	closed := (b.hadMenu || titleFocused) && !open
	b.hadMenu = open
	return closed
}

func (b *TitleBar) menu(m *ui.Menu, items []*mygo.MenuItem) {
	for _, item := range items {
		if !item.IsVisible() {
			continue
		}
		if item.Type == mygo.MenuItemSeparator {
			m.Separator()
			continue
		}
		var entry *ui.MenuItem
		if len(item.Submenu) > 0 {
			entry = m.Submenu(item.Label, func(sub *ui.Menu) { b.menu(sub, item.Submenu) })
		} else {
			entry = m.Item(item.Label)
		}
		entry.Disabled(!item.IsEnabled()).Trailing(item.Accelerator)
		if item.ID != "" {
			entry.ID(item.ID)
		}
		switch item.Type {
		case mygo.MenuItemCheckbox:
			entry.Checked(item.IsChecked())
		case mygo.MenuItemRadio:
			entry.Radio(item.IsChecked())
		}
		if entry.Chosen() && item.IsEnabled() {
			switch item.Type {
			case mygo.MenuItemCheckbox:
				item.SetChecked(!item.IsChecked())
			case mygo.MenuItemRadio:
				item.SetChecked(true)
			}
			if item.Click != nil {
				item.Click(item, b.Window)
			}
		}
	}
}
