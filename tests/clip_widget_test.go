package tests

import (
	"image"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// clipScene puts a 100×100 red box, clipped by clip, at (20, 20) on white.
func clipScene(clip func(w.Widget) w.Widget, tapped *int) w.Widget {
	box := w.GestureDetector{OnTap: func() { *tapped++ },
		Child: w.SizedBox{Width: 100, Height: 100, Child: w.DecoratedBox{Color: geom.Hex(0xFF0000)}}}
	return w.DecoratedBox{Color: geom.Hex(0xFFFFFF), Child: w.Align{Alignment: geom.TopLeft,
		Child: w.Padding{Padding: geom.Insets(20), Child: clip(box)}}}
}

func redAt(img *image.RGBA, x, y int) bool {
	c := img.RGBAAt(x, y)
	return c.R > 200 && c.G < 60 && c.B < 60
}

func whiteAt(img *image.RGBA, x, y int) bool {
	c := img.RGBAAt(x, y)
	return c.R > 230 && c.G > 230 && c.B > 230
}

func TestClipShapesPixels(t *testing.T) {
	cases := []struct {
		name   string
		clip   func(w.Widget) w.Widget
		corner bool // is the very corner (2px in) painted?
		edgeIn bool // is (10, 3) from the corner painted? (inside for small radii)
	}{
		{"rect", func(c w.Widget) w.Widget { return w.ClipRect{Child: c} }, true, true},
		{"clip radius 0", func(c w.Widget) w.Widget { return w.Clip{Child: c} }, true, true},
		{"rrect 12", func(c w.Widget) w.Widget { return w.ClipRRect{Radius: 12, Child: c} }, false, true},
		{"clip radius 30", func(c w.Widget) w.Widget { return w.Clip{Radius: 30, Child: c} }, false, false},
		{"oval", func(c w.Widget) w.Widget { return w.ClipOval{Child: c} }, false, false},
	}
	for _, c := range cases {
		var taps int
		tt := tester.New(clipScene(c.clip, &taps), 200, 200)
		img, err := tt.Snapshot()
		if err != nil {
			t.Skip("no GPU adapter:", err)
		}
		ox, oy := 20, 20
		if !redAt(img, ox+50, oy+50) {
			t.Errorf("%s: center not painted", c.name)
		}
		if !redAt(img, ox+50, oy+1) || !redAt(img, ox+1, oy+50) {
			t.Errorf("%s: edge middle not painted", c.name)
		}
		if got := redAt(img, ox+2, oy+2); got != c.corner {
			t.Errorf("%s: corner painted = %v, want %v (%v)", c.name, got, c.corner, img.RGBAAt(ox+2, oy+2))
		}
		if !c.corner && !whiteAt(img, ox+1, oy+1) {
			t.Errorf("%s: background should show in the corner, got %v", c.name, img.RGBAAt(ox+1, oy+1))
		}
		// All four corners alike.
		for _, p := range [][2]int{{ox + 97, oy + 2}, {ox + 2, oy + 97}, {ox + 97, oy + 97}} {
			if redAt(img, p[0], p[1]) != c.corner {
				t.Errorf("%s: corner %v differs", c.name, p)
			}
		}
		if got := redAt(img, ox+10, oy+3); got != c.edgeIn {
			t.Errorf("%s: (10,3) painted = %v, want %v", c.name, got, c.edgeIn)
		}
		// Taps in a cut corner miss; in the middle they hit.
		tt.Tap(float32(ox+1), float32(oy+1))
		if want := 0; !c.corner && taps != want {
			t.Errorf("%s: tap in the clipped corner reached the child", c.name)
		}
		tt.Tap(float32(ox+50), float32(oy+50))
		if taps == 0 {
			t.Errorf("%s: tap in the middle missed", c.name)
		}
	}
}

// Anti-aliased edge: some pixel on the curve is partly covered.
func TestClipRoundIsAntialiased(t *testing.T) {
	var taps int
	tt := tester.New(clipScene(func(c w.Widget) w.Widget { return w.ClipOval{Child: c} }, &taps), 200, 200)
	img, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	partial := 0
	for y := 20; y < 120; y++ {
		for x := 20; x < 120; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 240 && c.G > 20 && c.G < 230 {
				partial++
			}
		}
	}
	if partial < 20 {
		t.Fatalf("only %d blended edge pixels: edge isn't anti-aliased", partial)
	}
}

// The rounded clip is recorded on commands, survives repaint-boundary
// replay at a new position, and the innermost one wins.
func TestRoundClipRecorded(t *testing.T) {
	var c render.Canvas
	c.Reset(geom.Rect{W: 500, H: 500})
	c.PushRoundClip(geom.Rect{X: 10, Y: 10, W: 100, H: 100}, 8)
	c.FillRect(geom.Rect{X: 0, Y: 0, W: 50, H: 50}, geom.Hex(0x00FF00))
	c.PushRoundClip(geom.Rect{X: 20, Y: 20, W: 40, H: 40}, 100) // clamped to 20
	c.FillRect(geom.Rect{X: 0, Y: 0, W: 50, H: 50}, geom.Hex(0x00FF00))
	c.PopClip()
	c.PushClip(geom.Rect{X: 0, Y: 0, W: 30, H: 30})
	c.FillRect(geom.Rect{X: 0, Y: 0, W: 50, H: 50}, geom.Hex(0x00FF00))
	c.PopClip()
	c.PopClip()
	c.FillRect(geom.Rect{X: 0, Y: 0, W: 50, H: 50}, geom.Hex(0x00FF00))
	want := []render.RoundClip{
		{Rect: geom.Rect{X: 10, Y: 10, W: 100, H: 100}, Radius: 8},
		{Rect: geom.Rect{X: 20, Y: 20, W: 40, H: 40}, Radius: 20},
		{Rect: geom.Rect{X: 10, Y: 10, W: 100, H: 100}, Radius: 8}, // rect clip inside keeps the round one
		{},
	}
	if len(c.Commands) != len(want) {
		t.Fatalf("%d commands", len(c.Commands))
	}
	for i, cmd := range c.Commands {
		if cmd.Round != want[i] {
			t.Errorf("command %d: round %+v, want %+v", i, cmd.Round, want[i])
		}
	}
	if c.Commands[2].Clip != (geom.Rect{X: 10, Y: 10, W: 20, H: 20}) {
		t.Errorf("scissor box %v", c.Commands[2].Clip)
	}
}

// A rounded clip inside a repaint boundary moves with it when replayed.
func TestRoundClipInsideBoundaryMoves(t *testing.T) {
	x := float32(0)
	var taps int
	tt := tester.New(w.Builder{Builder: func(w.BuildContext) w.Widget {
		return w.Align{Alignment: geom.TopLeft, Child: w.Padding{Padding: geom.InsetsLTRB(x, 0, 0, 0),
			Child: w.RepaintBoundary{Child: w.ClipRRect{Radius: 10, Child: w.GestureDetector{OnTap: func() { taps++ },
				Child: w.SizedBox{Width: 60, Height: 60, Child: w.DecoratedBox{Color: geom.Hex(0xFF0000)}}}}}}}
	}}, 300, 100)
	tt.Pump()
	x = 100
	tt.Root.Reassemble()
	for _, cmd := range tt.Pump().Commands {
		if cmd.Kind == render.CmdRect && cmd.Color == geom.Hex(0xFF0000) {
			if cmd.Round.Rect.X != 100 || cmd.Round.Radius != 10 {
				t.Fatalf("round clip after move: %+v", cmd.Round)
			}
			return
		}
	}
	t.Fatal("box not drawn")
}
