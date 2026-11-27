package fluent

import "github.com/egoist/mygo/ui"

type shadow struct {
	blur  float32
	color ui.Color
}

type buttonFace struct {
	fill, text   ui.Color
	outer, inner shadow
}

type palette struct {
	normal, hover, pressed, disabled buttonFace
	inactiveText, focus              ui.Color
	inactiveShadow                   shadow
}

func rgba(r, g, b, a uint8) ui.Color { return ui.Color{R: r, G: g, B: b, A: a} }

// All colors use byte alpha; shadow offsets are (0, 1) DIP with zero spread.
var buttonPalettes = [2]palette{
	{
		normal:       buttonFace{rgba(255, 255, 255, 184), rgba(0, 0, 0, 230), shadow{2, rgba(0, 0, 0, 36)}, shadow{0, rgba(255, 255, 255, 180)}},
		hover:        buttonFace{rgba(255, 255, 255, 235), rgba(0, 0, 0, 230), shadow{3, rgba(0, 0, 0, 46)}, shadow{}},
		pressed:      buttonFace{rgba(0, 0, 0, 20), rgba(0, 0, 0, 230), shadow{}, shadow{2, rgba(0, 0, 0, 40)}},
		disabled:     buttonFace{fill: rgba(255, 255, 255, 102), text: rgba(0, 0, 0, 97)},
		inactiveText: rgba(0, 0, 0, 140), focus: rgba(0, 95, 184, 230), inactiveShadow: shadow{1, rgba(0, 0, 0, 20)},
	},
	{
		normal:       buttonFace{rgba(255, 255, 255, 20), rgba(255, 255, 255, 230), shadow{2, rgba(0, 0, 0, 102)}, shadow{0, rgba(255, 255, 255, 28)}},
		hover:        buttonFace{rgba(255, 255, 255, 36), rgba(255, 255, 255, 230), shadow{3, rgba(0, 0, 0, 128)}, shadow{}},
		pressed:      buttonFace{rgba(0, 0, 0, 70), rgba(255, 255, 255, 230), shadow{}, shadow{2, rgba(0, 0, 0, 90)}},
		disabled:     buttonFace{fill: rgba(255, 255, 255, 10), text: rgba(255, 255, 255, 97)},
		inactiveText: rgba(255, 255, 255, 128), focus: rgba(140, 190, 255, 230), inactiveShadow: shadow{1, rgba(0, 0, 0, 60)},
	},
}

func buttonStyle(dark, active, hovered, pressed, disabled bool) (buttonFace, ui.Color) {
	i := 0
	if dark {
		i = 1
	}
	p := buttonPalettes[i]
	f := p.normal
	if hovered {
		f = p.hover
	}
	if pressed {
		f = p.pressed
	}
	if !active {
		f.text = p.inactiveText
		if !hovered && !pressed {
			f.outer, f.inner = p.inactiveShadow, shadow{}
		}
	}
	if disabled {
		f = p.disabled
	}
	return f, p.focus
}

// StyleButton preserves the control's label, padding and action.
func StyleButton(c *ui.Context, b ui.Element) ui.Element {
	f, focus := buttonStyle(c.Theme().Dark, c.WindowActive(), b.Hovered(), b.Pressed(), b.IsDisabled())
	b.Radius(4).Background(f.fill).TextColor(f.text).NoShadow().Border(0, ui.Color{}).DimDisabled(false)
	if f.outer.color.A != 0 {
		b.Shadow(0, 1, f.outer.blur, 0, f.outer.color)
	}
	if f.inner.color.A != 0 {
		b.InsetShadow(0, 1, f.inner.blur, 0, f.inner.color)
	}
	b.FocusRing(!b.IsDisabled()).FocusRingColor(focus).FocusRingOffset(-2)
	return b
}
