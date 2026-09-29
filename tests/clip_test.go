package tests

import (
	"image/color"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

var red = geom.Hex(0xFF0000)

// overflowBox is h tall and paints a red band `bleed` px outside its top
// and right edge (like a shadow would).
func overflowBox(h, bleed float32) w.Widget {
	return w.CustomPaint{Size: geom.Sz(geom.Inf, h), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		c.FillRect(geom.Rect{X: o.X, Y: o.Y - bleed, W: s.W + bleed, H: s.H + bleed}, red)
	}}
}

func isRed(c color.RGBA) bool { return c.R > 200 && c.G < 60 && c.B < 60 }

func TestScrollViewClipsOnlyScrolledEdges(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(w.Column{Children: []w.Widget{
		w.SizedBox{Height: 40},
		w.SizedBox{Height: 100, Child: w.ScrollView{Controller: ctrl, Child: w.Column{Children: []w.Widget{
			overflowBox(30, 6), w.SizedBox{Height: 300},
		}}}},
	}}, 200, 200)
	defer tt.Close()
	snap, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	// Unscrolled: the first item's "shadow" above the list is visible.
	if !isRed(snap.RGBAAt(50, 37)) {
		t.Fatalf("overflow above an unscrolled list was clipped: %v", snap.RGBAAt(50, 37))
	}
	// Scrolled: content moved past the top edge is clipped there.
	ctrl.JumpTo(10)
	snap, _ = tt.Snapshot()
	if isRed(snap.RGBAAt(50, 37)) {
		t.Fatal("scrolled content painted above the list")
	}
	if !isRed(snap.RGBAAt(50, 45)) {
		t.Fatalf("visible part missing: %v", snap.RGBAAt(50, 45))
	}
}

func TestSplitPanesClipSharedEdgesOnly(t *testing.T) {
	tt := tester.New(w.Column{Children: []w.Widget{
		w.SizedBox{Height: 40},
		w.SizedBox{Height: 100, Child: w.SplitView{Panes: []w.Pane{
			{Size: 100, Child: overflowBox(30, 6)},
			{Child: w.SizedBox{}},
		}}},
	}}, 300, 200)
	defer tt.Close()
	snap, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	if !isRed(snap.RGBAAt(50, 37)) {
		t.Fatal("overflow at the split's outer (top) edge was clipped")
	}
	// Pane 0 is 100 wide; its right edge is shared with the divider/pane 1.
	if isRed(snap.RGBAAt(103, 50)) {
		t.Fatal("pane content spilled into the next pane")
	}
}
