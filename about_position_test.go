package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAboutWindowPosition(t *testing.T) {
	primary := mygo.Display{
		Bounds:   mygo.Rectangle{Width: 1920, Height: 1080},
		WorkArea: mygo.Rectangle{Y: 30, Width: 1920, Height: 1010},
	}
	right := mygo.Display{
		Bounds:      mygo.Rectangle{X: 1920, Width: 1600, Height: 1000},
		WorkArea:    mygo.Rectangle{X: 1920, Y: 40, Width: 1600, Height: 920},
		ScaleFactor: 1.5,
	}
	left := mygo.Display{
		Bounds:   mygo.Rectangle{X: -1600, Y: -200, Width: 1600, Height: 1200},
		WorkArea: mygo.Rectangle{X: -1600, Y: -170, Width: 1600, Height: 1130},
	}
	for _, tc := range []struct {
		name    string
		parent  mygo.Rectangle
		display mygo.Display
		want    mygo.Point
	}{
		{"right display", mygo.Rectangle{X: 2100, Y: 100, Width: 1000, Height: 850}, right, mygo.Point{X: 2240, Y: 135}},
		{"larger parent centers", mygo.Rectangle{X: 100, Y: 50, Width: 1400, Height: 900}, primary, mygo.Point{X: 440, Y: 110}},
		{"bottom right edge", mygo.Rectangle{X: 3300, Y: 850, Width: 500, Height: 500}, right, mygo.Point{X: 2800, Y: 180}},
		{"top left edge", mygo.Rectangle{X: 1910, Y: -100, Width: 500, Height: 500}, right, mygo.Point{X: 1920, Y: 40}},
		{"outside displays", mygo.Rectangle{X: 9000, Y: 9000, Width: 1000, Height: 900}, right, mygo.Point{X: 600, Y: 145}},
		{"minimized coordinates", mygo.Rectangle{X: -32000, Y: -32000, Width: 160, Height: 28}, primary, mygo.Point{X: 600, Y: 145}},
		{"empty minimized parent", mygo.Rectangle{}, primary, mygo.Point{X: 600, Y: 145}},
		{"touching only", mygo.Rectangle{X: 3520, Width: 500, Height: 500}, right, mygo.Point{X: 600, Y: 145}},
		{"negative coordinates", mygo.Rectangle{X: -1500, Y: -100, Width: 1200, Height: 1000}, left, mygo.Point{X: -1260, Y: 10}},
		{"small work area", mygo.Rectangle{X: 2050, Y: 100, Width: 800, Height: 700}, mygo.Display{
			Bounds: right.Bounds, WorkArea: mygo.Rectangle{X: 1920, Y: 40, Width: 600, Height: 500},
		}, mygo.Point{X: 1920, Y: 40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := aboutWindowPosition(tc.parent, tc.display, primary); got != tc.want {
				t.Fatalf("position = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAboutPositionReadAtWindowCreation(t *testing.T) {
	p := mygo.Point{X: 2000, Y: 80}
	calls := 0
	var positions []mygo.Point
	factory := aboutWindowFactory(&mygo.Window{}, func() mygo.Point {
		calls++
		return p
	}, func(opts mygo.WindowOptions) aboutNativeWindow {
		if opts.Width != 720 || opts.Height != 780 {
			t.Fatal("About size changed")
		}
		positions = append(positions, mygo.Point{X: opts.X, Y: opts.Y})
		return &aboutMenuWindow{}
	})
	if calls != 0 {
		t.Fatal("position read before window creation")
	}
	factory(func(*ui.Context) {}, func() {})
	p = mygo.Point{X: -1000, Y: 120}
	factory(func(*ui.Context) {}, func() {})
	if calls != 2 || positions[0] != (mygo.Point{X: 2000, Y: 80}) || positions[1] != p {
		t.Fatalf("positions = %+v, calls = %d", positions, calls)
	}
}
