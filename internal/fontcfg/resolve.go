package fontcfg

// Resolve picks the font field by field, from the first source that has it:
// Neovim's 'guifont', the font saved from the menu, the -font and -font-size
// flags, then the defaults. A 'guifont' of "Fira Code" without a size takes
// the size from the next source that has one.
func Resolve(guifont, saved, flag, def Choice) Choice {
	var c Choice
	for _, s := range []Choice{guifont, saved, flag, def} {
		if c.Family == "" {
			c.Family = s.Family
		}
		if c.Size == 0 {
			c.Size = s.Size
		}
	}
	return c
}
