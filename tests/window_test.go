package tests

import (
	"fmt"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// windowButtons resizes and retitles the window from deep inside the tree,
// and records the size its layout gets.
type windowButtons struct{ laidOut *geom.Size }

func (b windowButtons) Build(ctx w.BuildContext) w.Widget {
	win := w.WindowOf(ctx)
	btn := func(label string, f func()) w.Widget {
		return w.GestureDetector{OnTap: f, Child: w.Padding{Padding: geom.Insets(4), Child: w.Text{Text: label}}}
	}
	cw, ch := win.Size()
	return w.CustomPaint{
		Painter: func(_ *render.Canvas, _ geom.Offset, s geom.Size) { *b.laidOut = s },
		Child: w.Column{Cross: w.CrossStart, Children: []w.Widget{
			w.Text{Text: fmt.Sprintf("size %dx%d", cw, ch)},
			btn("Large", func() { win.SetSize(1200, 800) }),
			btn("Tiny", func() { win.SetSize(10, 10) }),
			btn("Rename", func() { win.SetTitle("Renamed") }),
			btn("Fullscreen", func() { win.SetFullscreen(true) }),
		}},
	}
}

func TestWindowFromContext(t *testing.T) {
	var laidOut geom.Size
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: geom.Inf, Height: geom.Inf,
		Child: windowButtons{laidOut: &laidOut}}}, 640, 480)
	if laidOut != (geom.Size{W: 640, H: 480}) {
		t.Fatalf("initial layout %v", laidOut)
	}
	if err := tt.TapText("Large"); err != nil {
		t.Fatal(err)
	}
	tt.Pump()
	if tt.Size != (geom.Size{W: 1200, H: 800}) || laidOut != tt.Size {
		t.Fatalf("after SetSize: window %v, layout %v", tt.Size, laidOut)
	}
	// Min/max clamp requests.
	tt.Window.SetMinSize(400, 300)
	if err := tt.TapText("Tiny"); err != nil {
		t.Fatal(err)
	}
	tt.Pump()
	if tt.Size != (geom.Size{W: 400, H: 300}) {
		t.Fatalf("min size not applied: %v", tt.Size)
	}
	tt.Window.SetMaxSize(1000, 700)
	tt.TapText("Large")
	tt.Pump()
	if tt.Size != (geom.Size{W: 1000, H: 700}) {
		t.Fatalf("max size not applied: %v", tt.Size)
	}
	tt.TapText("Rename")
	if tt.Window.Title() != "Renamed" {
		t.Fatalf("title %q", tt.Window.Title())
	}
	// Fullscreen owns the size.
	tt.TapText("Fullscreen")
	before := tt.Window.Resizes
	tt.Window.SetSize(800, 600)
	if !tt.Window.IsFullscreen() || tt.Window.Resizes != before {
		t.Fatal("SetSize changed a fullscreen window")
	}
}

func TestWindowOfWithoutWindow(t *testing.T) {
	// A tree without a host window still gets a usable (no-op) Window.
	var got w.Window
	owner := w.NewBuildOwner()
	root := w.Mount(w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		got = w.WindowOf(ctx)
		return w.SizedBox{}
	}}, geom.Transparent, owner, render.NewPipelineOwner())
	defer root.Unmount()
	owner.FlushBuild()
	if got == nil {
		t.Fatal("WindowOf returned nil")
	}
	got.SetSize(100, 100) // must not panic
	if wd, h := got.Size(); wd != 0 || h != 0 {
		t.Fatalf("no-op window size %dx%d", wd, h)
	}
}
