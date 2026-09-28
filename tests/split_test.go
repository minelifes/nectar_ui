package tests

import (
	"fmt"
	"math"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func near(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 0.5 {
			return false
		}
	}
	return true
}

func sum(v []float32) (s float32) {
	for _, x := range v {
		s += x
	}
	return
}

func TestResolveSplit(t *testing.T) {
	// Unset panes share what's left.
	got := render.ResolveSplit([]float32{200, 0, 0}, nil, nil, 3, 800)
	if !near(got, []float32{200, 300, 300}) {
		t.Fatalf("shares: %v", got)
	}
	// Scaling with the window keeps proportions...
	got = render.ResolveSplit([]float32{200, 300, 500}, nil, nil, 3, 500)
	if !near(got, []float32{100, 150, 250}) {
		t.Fatalf("scale: %v", got)
	}
	// ...but respects min/max, giving the rest to the others.
	got = render.ResolveSplit([]float32{200, 300, 500}, []float32{180, 0, 0}, []float32{0, 0, 260}, 3, 1000)
	if got[0] < 180 || got[2] > 260 || math.Abs(float64(sum(got)-1000)) > 0.5 {
		t.Fatalf("limits: %v", got)
	}
	// Minimums that don't fit: everyone at min (overflow).
	got = render.ResolveSplit(nil, []float32{300, 300}, nil, 2, 400)
	if !near(got, []float32{300, 300}) {
		t.Fatalf("overflow: %v", got)
	}
}

func TestDragSplitCascades(t *testing.T) {
	mins := []float32{100, 100, 100, 100}
	sizes := []float32{200, 200, 200, 200}
	// Drag divider 0 right by 250: pane 1 shrinks to its min (-100), pane 2
	// to its min (-100), then pane 3 gives the last 50.
	got := render.DragSplit(sizes, mins, nil, 0, 250)
	if !near(got, []float32{450, 100, 100, 150}) {
		t.Fatalf("cascade right: %v", got)
	}
	// Dragging further than everything can shrink stops at the minimums.
	got = render.DragSplit(sizes, mins, nil, 0, 10000)
	if !near(got, []float32{500, 100, 100, 100}) {
		t.Fatalf("clamp right: %v", got)
	}
	// Left drag on the last divider: panes 2, 1, 0 shrink in turn.
	got = render.DragSplit(sizes, mins, nil, 2, -250)
	if !near(got, []float32{150, 100, 100, 450}) {
		t.Fatalf("cascade left: %v", got)
	}
	// A max on the growing pane passes growth outward too.
	maxs := []float32{0, 250, 0, 0}
	got = render.DragSplit(sizes, mins, maxs, 1, -120) // pane 2 grows, pane 1 shrinks, then pane 0
	// Pane 2 takes all 120; pane 1 can shrink by 100, pane 0 gives the other 20.
	if !near(got, []float32{180, 100, 320, 200}) {
		t.Fatalf("mixed: %v", got)
	}
	if math.Abs(float64(sum(got)-800)) > 0.5 {
		t.Fatalf("total changed: %v", got)
	}
	// Growing pane capped by max: the growth passes on to the next pane.
	got = render.DragSplit(sizes, nil, []float32{0, 230, 0, 0}, 0, -100) // pane 1 +30, pane 2 +70
	if !near(got, []float32{100, 230, 270, 200}) {
		t.Fatalf("grow cascade: %v", got)
	}
	// Nothing on the growing side can grow: the move is limited.
	got = render.DragSplit([]float32{200, 200}, nil, []float32{0, 230}, 0, -100)
	if !near(got, []float32{170, 230}) {
		t.Fatalf("grow cap: %v", got)
	}
}

// splitHost shows 3 panes and records sizes.
func splitHost(sizes *[]float32, vertical bool) w.Widget {
	label := func(i int) w.Widget {
		return w.Builder{Builder: func(w.BuildContext) w.Widget { return w.Text{Text: fmt.Sprintf("pane %d", i)} }}
	}
	return w.SplitView{Vertical: vertical, OnResize: func(v []float32) { *sizes = v }, Panes: []w.Pane{
		{Min: 100, Max: 300, Size: 200, Child: label(0)},
		{Min: 150, Child: label(1)},
		{Min: 100, Child: label(2)},
	}}
}

func paneWidths(tt *tester.Tester) []float32 {
	var out []float32
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if s, ok := ro.(*render.RenderSplit); ok {
			out = s.Resolved()
			return
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	return out
}

func TestSplitViewDragAndResize(t *testing.T) {
	var sizes []float32
	tt := tester.New(splitHost(&sizes, false), 816, 300) // 816 - 2*8 gaps = 800 for panes
	if got := paneWidths(tt); !near(got, []float32{200, 300, 300}) {
		t.Fatalf("initial %v", got)
	}
	// Divider 0 sits at x 200..208. Drag it right by 150: pane 0 grows to its
	// max 300 (+100) — the move is limited to what pane 0 can take.
	tt.Drag(geom.Pt(204, 150), geom.Pt(354, 150))
	if got := paneWidths(tt); !near(got, []float32{300, 200, 300}) {
		t.Fatalf("after drag 0: %v", got)
	}
	// Divider 1 (at 300+8+200 = 508..516): drag left by 300. Pane 1 shrinks to
	// 150 (-50), then pane 0 gives 200 (down to 100); pane 2 grows by 250.
	tt.Drag(geom.Pt(512, 150), geom.Pt(212, 150))
	if got := paneWidths(tt); !near(got, []float32{100, 150, 550}) {
		t.Fatalf("after drag 1: %v", got)
	}
	if !near(sizes, []float32{100, 150, 550}) {
		t.Fatalf("OnResize got %v", sizes)
	}
	// Window grows: proportions kept, limits respected.
	tt.Size = geom.Size{W: 1616, H: 300}
	tt.Pump()
	got := paneWidths(tt)
	if math.Abs(float64(sum(got)-1600)) > 0.5 || got[0] > 300 {
		t.Fatalf("after window resize: %v", got)
	}
	// Keyboard: focus divider 0 by tapping it, then → moves it by 10px.
	before := paneWidths(tt)
	x := before[0] + 4
	tt.Tap(x, 150)
	tt.Key(w.KeyRight)
	after := paneWidths(tt)
	if math.Abs(float64(after[0]-before[0]-10)) > 0.5 && after[0] != 300 {
		t.Fatalf("arrow key: %v -> %v", before, after)
	}
}

func TestSplitViewVertical(t *testing.T) {
	var sizes []float32
	tt := tester.New(splitHost(&sizes, true), 300, 816)
	if got := paneWidths(tt); !near(got, []float32{200, 300, 300}) {
		t.Fatalf("initial %v", got)
	}
	tt.Drag(geom.Pt(150, 204), geom.Pt(150, 254))
	if got := paneWidths(tt); !near(got, []float32{250, 250, 300}) {
		t.Fatalf("after drag: %v", got)
	}
	// Panes are clipped: pane 1's text is inside its own rect.
	r, ok := tt.Find("pane 1")
	if !ok || r.Y < 250+8-0.5 {
		t.Fatalf("pane 1 text at %v (ok=%v)", r, ok)
	}
}
