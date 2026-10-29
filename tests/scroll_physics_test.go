package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// physicsList is a 300px-tall list of 50 rows of 40px (max offset 1700).
func physicsList(ctrl *w.ScrollController, p w.ScrollPhysics, n int) w.Widget {
	rows := make([]w.Widget, n)
	for i := range rows {
		rows[i] = w.SizedBox{Height: 40, Child: w.Text{Text: fmt.Sprintf("row %d", i)}}
	}
	return w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 200, Height: 300,
		Child: w.ListView{Controller: ctrl, Physics: p, Children: rows}}}
}

// press, move and hold (no release), returning a release func.
func hold(tt *tester.Tester, from, to geom.Offset) func() {
	tt.Pointer(render.PointerDown, from.X, from.Y)
	for i := 1; i <= 10; i++ {
		f := float32(i) / 10
		tt.Pointer(render.PointerMove, w.Lerp(from.X, to.X, f), w.Lerp(from.Y, to.Y, f))
	}
	return func() { tt.Pointer(render.PointerUp, to.X, to.Y) }
}

func TestBounceAtTheStart(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(physicsList(ctrl, w.InheritPhysics, 50), 400, 400) // default: bouncing
	release := hold(tt, geom.Pt(100, 50), geom.Pt(100, 250))            // pull down 200px
	over := ctrl.Overscroll()
	if over >= 0 || ctrl.Offset() != 0 {
		t.Fatalf("pulled past the top: overscroll %v offset %v", over, ctrl.Offset())
	}
	if -over >= 200 || -over < 30 {
		t.Fatalf("overscroll %v for a 200px pull: want resistance", over)
	}
	if r, _ := tt.Find("row 0"); r.Y < 20 {
		t.Fatalf("content not shifted: row 0 at %v", r)
	}
	release()
	tt.Settle()
	if ctrl.Overscroll() != 0 || ctrl.Offset() != 0 {
		t.Fatalf("didn't spring back: %v %v", ctrl.Overscroll(), ctrl.Offset())
	}
	if r, _ := tt.Find("row 0"); r.Y != 0 {
		t.Fatalf("row 0 at %v after bounce", r)
	}
}

func TestBounceAtTheEndAndBackWithinADrag(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(physicsList(ctrl, w.BouncingScrollPhysics, 50), 400, 400)
	ctrl.JumpTo(ctrl.MaxOffset() - 20)
	release := hold(tt, geom.Pt(100, 250), geom.Pt(100, 100)) // 150px up: 20 scroll + 130 over
	if ctrl.Offset() != ctrl.MaxOffset() || ctrl.Overscroll() <= 0 {
		t.Fatalf("past the end: offset %v/%v over %v", ctrl.Offset(), ctrl.MaxOffset(), ctrl.Overscroll())
	}
	// Moving back first undoes the overscroll, then scrolls.
	for y := float32(110); y <= 260; y += 10 {
		tt.Pointer(render.PointerMove, 100, y)
	}
	if ctrl.Overscroll() != 0 || ctrl.Offset() >= ctrl.MaxOffset() {
		t.Fatalf("drag back: over %v offset %v", ctrl.Overscroll(), ctrl.Offset())
	}
	release()
	tt.Settle()
}

func TestClampingPhysicsDoesNotOverscroll(t *testing.T) {
	for name, build := range map[string]func(*w.ScrollController) w.Widget{
		"field": func(c *w.ScrollController) w.Widget { return physicsList(c, w.ClampingScrollPhysics, 50) },
		"config": func(c *w.ScrollController) w.Widget {
			return w.ScrollConfiguration{Physics: w.ClampingScrollPhysics, Child: physicsList(c, 0, 50)}
		},
	} {
		ctrl := w.NewScrollController()
		tt := tester.New(build(ctrl), 400, 400)
		release := hold(tt, geom.Pt(100, 50), geom.Pt(100, 250))
		if ctrl.Overscroll() != 0 {
			t.Errorf("%s: clamping overscrolled %v", name, ctrl.Overscroll())
		}
		release()
		tt.Scroll(100, 100, -300)
		tt.Advance(50 * time.Millisecond)
		if ctrl.Overscroll() != 0 {
			t.Errorf("%s: clamping wheel overscrolled", name)
		}
	}
}

func TestShortListDoesNotBounce(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(physicsList(ctrl, w.BouncingScrollPhysics, 3), 400, 400)
	release := hold(tt, geom.Pt(100, 20), geom.Pt(100, 200))
	if ctrl.Overscroll() != 0 {
		t.Fatalf("a list that fits bounced: %v", ctrl.Overscroll())
	}
	release()
}

func TestFlingCoastsAndBounces(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(physicsList(ctrl, w.BouncingScrollPhysics, 50), 400, 400)
	tt.Fling(geom.Pt(100, 280), geom.Pt(100, 180), 80*time.Millisecond) // fast flick up
	atRelease := ctrl.Offset()
	tt.Advance(200 * time.Millisecond)
	if ctrl.Offset() <= atRelease+50 {
		t.Fatalf("no momentum: %v → %v", atRelease, ctrl.Offset())
	}
	// A hard fling into the end overshoots, then settles exactly there.
	ctrl.JumpTo(ctrl.MaxOffset() - 100)
	tt.Fling(geom.Pt(100, 280), geom.Pt(100, 130), 60*time.Millisecond)
	peak := float32(0)
	for i := 0; i < 60; i++ {
		tt.Advance(16 * time.Millisecond)
		peak = max(peak, ctrl.Overscroll())
	}
	tt.Settle()
	if peak < 5 {
		t.Errorf("fling into the end didn't bounce (peak %v)", peak)
	}
	if ctrl.Offset() != ctrl.MaxOffset() || ctrl.Overscroll() != 0 {
		t.Errorf("after bounce: %v/%v over %v", ctrl.Offset(), ctrl.MaxOffset(), ctrl.Overscroll())
	}
	// Clamping: a fling stops at the edge.
	cc := w.NewScrollController()
	tc := tester.New(physicsList(cc, w.ClampingScrollPhysics, 50), 400, 400)
	cc.JumpTo(cc.MaxOffset() - 100)
	tc.Fling(geom.Pt(100, 280), geom.Pt(100, 130), 60*time.Millisecond)
	for i := 0; i < 60; i++ {
		tc.Advance(16 * time.Millisecond)
		if cc.Overscroll() != 0 {
			t.Fatal("clamping fling overscrolled")
		}
	}
	if cc.Offset() != cc.MaxOffset() {
		t.Errorf("clamping fling stopped at %v, want the end %v", cc.Offset(), cc.MaxOffset())
	}
	// A slow drag that pauses before release doesn't fling.
	ctrl.JumpTo(500)
	tt.Pointer(render.PointerDown, 100, 200)
	tt.Pointer(render.PointerMove, 100, 150)
	tt.Advance(300 * time.Millisecond)
	tt.Pointer(render.PointerUp, 100, 150)
	at := ctrl.Offset()
	tt.Settle()
	if ctrl.Offset() != at {
		t.Errorf("paused drag flung: %v → %v", at, ctrl.Offset())
	}
}

func TestWheelBouncesAtEdgeAndNestedScrollsOuterFirst(t *testing.T) {
	ctrl := w.NewScrollController()
	tt := tester.New(physicsList(ctrl, w.BouncingScrollPhysics, 50), 400, 400)
	tt.Scroll(100, 100, -120) // at the top already
	tt.Pump()
	if ctrl.Overscroll() >= 0 {
		t.Fatalf("wheel at the top: overscroll %v", ctrl.Overscroll())
	}
	tt.Settle()
	if ctrl.Overscroll() != 0 || ctrl.Offset() != 0 {
		t.Fatalf("wheel bounce didn't settle: %v %v", ctrl.Overscroll(), ctrl.Offset())
	}

	// An inner list at its top inside a scrolled outer page: the wheel
	// scrolls the outer page instead of bouncing the inner list.
	inner, outer := w.NewScrollController(), w.NewScrollController()
	page := w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 300, Height: 300, Child: w.ScrollView{Controller: outer,
		Child: w.Column{ShrinkMain: true, Children: []w.Widget{
			w.SizedBox{Height: 200},
			physicsList(inner, 0, 50),
			w.SizedBox{Height: 600},
		}}}}}
	tn := tester.New(page, 400, 400)
	outer.JumpTo(150)
	tn.Scroll(100, 150, -60) // over the inner list, upward; it's at its top
	tn.Pump()
	if inner.Overscroll() != 0 {
		t.Fatalf("inner list bounced although the page could scroll (%v)", inner.Overscroll())
	}
	if outer.Offset() != 90 {
		t.Fatalf("outer page didn't take the wheel: %v", outer.Offset())
	}
}

func TestScrollView2DBounces(t *testing.T) {
	h, v := w.NewScrollController(), w.NewScrollController()
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 300, Height: 200,
		Child: w.ScrollView2D{Horizontal: h, Vertical: v, Child: w.SizedBox{Width: 1000, Height: 800}}}}, 400, 300)
	release := hold(tt, geom.Pt(50, 50), geom.Pt(150, 150)) // pull right and down at the origin
	if h.Overscroll() >= 0 || v.Overscroll() >= 0 {
		t.Fatalf("2D overscroll %v %v", h.Overscroll(), v.Overscroll())
	}
	release()
	tt.Settle()
	if h.Overscroll() != 0 || v.Overscroll() != 0 {
		t.Fatal("2D didn't spring back")
	}
}

func TestCarouselBouncesAtTheEnds(t *testing.T) {
	items := make([]w.Widget, 6)
	for i := range items {
		items[i] = w.Text{Text: fmt.Sprintf("card %d", i)}
	}
	tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 400,
		Child: m.Carousel{Items: items, Height: 200}}}}, 500, 300)
	tt.Settle()
	before, _ := tt.Find("card 0")
	release := hold(tt, geom.Pt(100, 100), geom.Pt(250, 100)) // pull right at the first item
	during, _ := tt.Find("card 0")
	if during.X <= before.X+10 || during.X >= before.X+150 {
		t.Fatalf("card 0 at %v while pulled (was %v): want a damped shift", during.X, before.X)
	}
	release()
	tt.Settle()
	if after, _ := tt.Find("card 0"); after.X != before.X {
		t.Fatalf("carousel didn't spring back: %v vs %v", after.X, before.X)
	}
	// Clamping: no shift.
	tc := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 400,
		Child: m.Carousel{Items: items, Height: 200, Physics: w.ClampingScrollPhysics}}}}, 500, 300)
	tc.Settle()
	b2, _ := tc.Find("card 0")
	rel := hold(tc, geom.Pt(100, 100), geom.Pt(250, 100))
	if d, _ := tc.Find("card 0"); d.X != b2.X {
		t.Fatalf("clamping carousel moved: %v vs %v", d.X, b2.X)
	}
	rel()
}
