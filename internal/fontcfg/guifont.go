// Package fontcfg decides which font the editor draws with: Neovim's 'guifont'
// option, the font chosen in the menu, the -font flag, and the defaults, in
// that order, and it reads and writes the saved menu choice.
package fontcfg

import (
	"strconv"
	"strings"
)

// Choice is a font family list and a size in DIPs. A zero field means "not
// chosen": Resolve then takes that field from the next source.
type Choice struct {
	// Family is a family list as ui.Font takes it, e.g. "Menlo, monospace".
	Family string
	Size   float32
}

// GuiFont is a parsed 'guifont' value.
type GuiFont struct {
	// Families are the font names in order, underscores turned into spaces.
	Families []string
	// Size is the size of the first font that has an :h option; 0 if none does.
	Size float32
	// Bold and Italic are what :b and :i said for the first font that has them.
	// They are parsed, but the editor does not use them: bold and italic text
	// comes from the highlight groups, and the regular font stays regular.
	Bold, Italic bool
	// Ignored lists, for the log or a message, what was understood but is not
	// used: options this editor has no use for (:w, :c, :s, ...), sizes
	// that are not a positive number, a size on a font after the first that has one,
	// and "*" (the font chooser).
	Ignored []string
}

// Parse reads a 'guifont' value: a comma separated list of fonts, each
// "Name[:option[:option...]]" (the name may be left out when there are options). In a name '_' means a space and a backslash
// takes the next character literally (so "\," is a comma in a name). The
// options understood are :hN (size, may be fractional), :b and :i; others are
// listed in Ignored. An empty or blank value gives the zero GuiFont.
func Parse(value string) GuiFont {
	var g GuiFont
	var sizeSet, boldSet, italicSet bool
	for _, entry := range splitUnescaped(value, ',') {
		parts := splitUnescaped(entry, ':')
		name := strings.TrimSpace(strings.ReplaceAll(unescape(parts[0]), "_", " "))
		if name == "*" {
			g.Ignored = append(g.Ignored, "*")
			continue
		}
		// A blank name still lets its options count: ":h16" sets only a size.
		if name != "" {
			g.Families = append(g.Families, name)
		}
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if opt == "" {
				continue
			}
			switch opt[0] {
			case 'h':
				v, err := strconv.ParseFloat(opt[1:], 32)
				switch {
				case err != nil || v <= 0 || v != v || v > 1000:
					g.Ignored = append(g.Ignored, opt)
				case sizeSet:
					g.Ignored = append(g.Ignored, name+":"+opt)
				default:
					g.Size, sizeSet = float32(v), true
				}
			case 'b':
				if opt == "b" && !boldSet {
					g.Bold, boldSet = true, true
				} else if opt != "b" {
					g.Ignored = append(g.Ignored, opt)
				}
			case 'i':
				if opt == "i" && !italicSet {
					g.Italic, italicSet = true, true
				} else if opt != "i" {
					g.Ignored = append(g.Ignored, opt)
				}
			default:
				g.Ignored = append(g.Ignored, opt)
			}
		}
	}
	return g
}

// Choice is the font list and size of g, with ", monospace" added to the list
// so that a name that is not installed still gives a fixed-width font.
func (g GuiFont) Choice() Choice {
	if len(g.Families) == 0 {
		return Choice{Size: g.Size}
	}
	return Choice{Family: WithFallback(strings.Join(g.Families, ", ")), Size: g.Size}
}

// WithFallback appends the generic "monospace" family to a family list unless
// it is there already.
func WithFallback(family string) string {
	for _, f := range strings.Split(family, ",") {
		if strings.EqualFold(strings.TrimSpace(f), "monospace") {
			return family
		}
	}
	return family + ", monospace"
}

// FirstFamily is the first name of a family list.
func FirstFamily(family string) string {
	first, _, _ := strings.Cut(family, ",")
	return strings.TrimSpace(first)
}

// Format is the 'guifont' value for a family and a size, the inverse of Parse
// for one font: "Fira Code:h14". Spaces stay as they are, and ',', ':' and '\'
// are escaped. A zero size leaves out the option.
func Format(family string, size float32) string {
	var b strings.Builder
	for i := 0; i < len(family); i++ {
		if c := family[i]; c == ',' || c == ':' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(family[i])
	}
	if size > 0 {
		b.WriteString(":h")
		b.WriteString(strconv.FormatFloat(float64(size), 'f', -1, 32))
	}
	return b.String()
}

// splitUnescaped splits s at sep, except where a backslash precedes it.
func splitUnescaped(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case sep:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// unescape drops each backslash and keeps the character after it.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
