package fontcfg

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  GuiFont
	}{
		{"empty", "", GuiFont{}},
		{"blank", "   ", GuiFont{}},
		{"only separators", ",,:,", GuiFont{}},
		{"name only", "Menlo", GuiFont{Families: []string{"Menlo"}}},
		{"spaces in name", "Fira Code", GuiFont{Families: []string{"Fira Code"}}},
		{"underscores are spaces", "JetBrainsMono_Nerd_Font", GuiFont{Families: []string{"JetBrainsMono Nerd Font"}}},
		{"size", "Menlo:h14", GuiFont{Families: []string{"Menlo"}, Size: 14}},
		{"fractional size", "Menlo:h11.5", GuiFont{Families: []string{"Menlo"}, Size: 11.5}},
		{"size alone", ":h16", GuiFont{Size: 16}},
		{"several fonts", "Fira_Code,Menlo,Noto Sans Mono CJK SC:h13",
			GuiFont{Families: []string{"Fira Code", "Menlo", "Noto Sans Mono CJK SC"}, Size: 13}},
		{"first size wins", "A:h12,B:h20", GuiFont{Families: []string{"A", "B"}, Size: 12, Ignored: []string{"B:h20"}}},
		{"bold and italic", "Menlo:h12:b:i", GuiFont{Families: []string{"Menlo"}, Size: 12, Bold: true, Italic: true}},
		{"bold only", "Menlo:b", GuiFont{Families: []string{"Menlo"}, Bold: true}},
		{"options in any order", "Menlo:i:h9:b", GuiFont{Families: []string{"Menlo"}, Size: 9, Bold: true, Italic: true}},
		{"windows options are reported", "Consolas:h11:cANSI:w5:sx",
			GuiFont{Families: []string{"Consolas"}, Size: 11, Ignored: []string{"cANSI", "w5", "sx"}}},
		{"bad sizes are reported", "A:hx,B:h0,C:h-3,D:h", GuiFont{Families: []string{"A", "B", "C", "D"}, Ignored: []string{"hx", "h0", "h-3", "h"}}},
		{"font chooser", "*", GuiFont{Ignored: []string{"*"}}},
		{"escaped comma", `Odd\,Name:h10`, GuiFont{Families: []string{"Odd,Name"}, Size: 10}},
		{"escaped colon and space", `A\:B\ C`, GuiFont{Families: []string{"A:B C"}}},
		{"spaces around", " Menlo : h14 , Monaco ", GuiFont{Families: []string{"Menlo", "Monaco"}, Size: 14}},
		{"cjk name", "思源等宽:h15", GuiFont{Families: []string{"思源等宽"}, Size: 15}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.value)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Parse(%q)\n got %+v\nwant %+v", c.value, got, c.want)
			}
		})
	}
}

func TestGuiFontChoice(t *testing.T) {
	cases := []struct {
		value string
		want  Choice
	}{
		{"", Choice{}},
		{"Menlo:h14", Choice{Family: "Menlo, monospace", Size: 14}},
		{"Fira_Code,Menlo", Choice{Family: "Fira Code, Menlo, monospace"}},
		{"Menlo,monospace:h12", Choice{Family: "Menlo, monospace", Size: 12}},
		{"Menlo,MONOSPACE", Choice{Family: "Menlo, MONOSPACE"}},
		{":h16", Choice{Size: 16}},
		{"*", Choice{}},
	}
	for _, c := range cases {
		if got := Parse(c.value).Choice(); got != c.want {
			t.Errorf("Parse(%q).Choice() = %+v, want %+v", c.value, got, c.want)
		}
	}
}

func TestFormatIsInverseOfParse(t *testing.T) {
	cases := []struct {
		family string
		size   float32
		want   string
	}{
		{"Menlo", 14, "Menlo:h14"},
		{"Fira Code", 11.5, "Fira Code:h11.5"},
		{"JetBrainsMono Nerd Font", 0, "JetBrainsMono Nerd Font"},
		{"Odd,Name:x", 10, `Odd\,Name\:x:h10`},
		{`Back\slash`, 12, `Back\\slash:h12`},
		{"思源等宽", 15, "思源等宽:h15"},
	}
	for _, c := range cases {
		got := Format(c.family, c.size)
		if got != c.want {
			t.Errorf("Format(%q, %v) = %q, want %q", c.family, c.size, got, c.want)
		}
		back := Parse(got)
		if !reflect.DeepEqual(back.Families, []string{c.family}) || back.Size != c.size {
			t.Errorf("Parse(Format(%q, %v)) = %+v: lost something", c.family, c.size, back)
		}
	}
}

func TestFirstFamily(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "Menlo": "Menlo", "Fira Code, Menlo, monospace": "Fira Code", " Menlo , x": "Menlo",
	} {
		if got := FirstFamily(in); got != want {
			t.Errorf("FirstFamily(%q) = %q, want %q", in, got, want)
		}
	}
}
