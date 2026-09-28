package tests

import (
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// harness runs the same pipeline as ui.App, minus the window.
type harness struct {
	bo   *widgets.BuildOwner
	po   *render.PipelineOwner
	disp *render.PointerDispatcher
	log  []string
}

func newHarness() *harness {
	return &harness{bo: widgets.NewBuildOwner(), po: render.NewPipelineOwner(), disp: render.NewPointerDispatcher()}
}

// mount must be called after the harness exists, so widget callbacks can
// capture it.
func (h *harness) mount(app widgets.Widget) {
	widgets.Mount(app, geom.White, h.bo, h.po)
	h.frame()
}

func (h *harness) frame() {
	h.bo.FlushBuild()
	h.po.FlushLayout(geom.Size{W: 400, H: 300})
}

func (h *harness) send(kind render.PointerKind, x, y float32) {
	h.disp.Dispatch(h.po.Root(), render.PointerEvent{Kind: kind, ID: 1, Position: geom.Pt(x, y), Button: render.ButtonPrimary})
	h.frame()
}

func (h *harness) tap(x, y float32) {
	h.send(render.PointerDown, x, y)
	h.send(render.PointerUp, x, y)
}

func (h *harness) rec(s string) func() { return func() { h.log = append(h.log, s) } }
func (h *harness) logged() string      { s := strings.Join(h.log, ","); h.log = nil; return s }
func at(x, y float32, child widgets.Widget) widgets.Widget {
	// Positions child at (x, y) with size 100x50 inside the 400x300 window.
	return widgets.Padding{Padding: geom.InsetsLTRB(x, y, 0, 0), Child: widgets.Align{Alignment: geom.TopLeft,
		Child: widgets.SizedBox{Width: 100, Height: 50, Child: child}}}
}

func TestTapInsideAndOutside(t *testing.T) {
	h := newHarness()
	h.mount(widgets.Builder{Builder: func(widgets.BuildContext) widgets.Widget {
		return at(10, 10, widgets.GestureDetector{OnTap: h.rec("tap"), OnTapCancel: h.rec("cancel")})
	}})
	h.tap(50, 30)
	if got := h.logged(); got != "tap" {
		t.Fatalf("inside: %q", got)
	}
	h.tap(300, 200)
	if got := h.logged(); got != "" {
		t.Fatalf("outside: %q", got)
	}
	// Press inside, release outside: cancelled, no tap.
	h.send(render.PointerDown, 50, 30)
	h.send(render.PointerMove, 50, 31) // within slop
	h.send(render.PointerUp, 250, 200)
	if got := h.logged(); got != "cancel" {
		t.Fatalf("release outside: %q", got)
	}
}

func TestNestedDetectorsInnerWins(t *testing.T) {
	h := newHarness()
	h.mount(widgets.Builder{Builder: func(widgets.BuildContext) widgets.Widget {
		return widgets.GestureDetector{OnTap: h.rec("outer"), OnTapCancel: h.rec("outer-cancel"),
			Child: at(10, 10, widgets.GestureDetector{OnTap: h.rec("inner")})}
	}})
	h.tap(50, 30)
	if got := h.logged(); got != "inner,outer-cancel" {
		t.Fatalf("got %q", got)
	}
	h.tap(300, 200) // only the outer one is under the pointer
	if got := h.logged(); got != "outer" {
		t.Fatalf("got %q", got)
	}
}

func TestDragBeatsTapAndIsCaptured(t *testing.T) {
	h := newHarness()
	var total geom.Offset
	h.mount(widgets.Builder{Builder: func(widgets.BuildContext) widgets.Widget {
		return widgets.GestureDetector{
			OnPanStart:  func(widgets.DragDetails) { h.log = append(h.log, "start") },
			OnPanUpdate: func(d widgets.DragDetails) { total = d.Total },
			OnPanEnd:    func(widgets.DragDetails) { h.log = append(h.log, "end") },
			Child:       at(10, 10, widgets.GestureDetector{OnTap: h.rec("tap"), OnTapCancel: h.rec("tap-cancel")}),
		}
	}})
	h.send(render.PointerDown, 50, 30)
	h.send(render.PointerMove, 55, 30) // within slop: nothing yet
	if got := h.logged(); got != "" {
		t.Fatalf("early: %q", got)
	}
	h.send(render.PointerMove, 80, 30)   // past slop: pan wins, tap cancelled
	h.send(render.PointerMove, 390, 290) // far outside the inner box: still delivered
	h.send(render.PointerUp, 390, 290)
	if got := h.logged(); got != "tap-cancel,start,end" {
		t.Fatalf("got %q", got)
	}
	if total != geom.Pt(340, 260) {
		t.Fatalf("total drag %v", total)
	}
}

func TestHoverEnterExitAndCursor(t *testing.T) {
	h := newHarness()
	h.mount(widgets.Builder{Builder: func(widgets.BuildContext) widgets.Widget {
		return at(10, 10, widgets.MouseRegion{Cursor: widgets.CursorPointer,
			OnEnter: func(widgets.PointerEvent) { h.log = append(h.log, "enter") },
			OnExit:  func(widgets.PointerEvent) { h.log = append(h.log, "exit") }})
	}})
	h.send(render.PointerHover, 300, 200)
	h.send(render.PointerHover, 50, 30)
	if h.disp.Cursor() != render.CursorPointer {
		t.Fatal("cursor should be pointer over the region")
	}
	h.send(render.PointerHover, 60, 30)
	h.send(render.PointerHover, 300, 200)
	if got := h.logged(); got != "enter,exit" {
		t.Fatalf("got %q", got)
	}
	if h.disp.Cursor() != render.CursorDefault {
		t.Fatal("cursor should reset")
	}
}

// A stateful button: the classic counter, end to end.
type counterButton struct{}

func (counterButton) CreateState() widgets.State { return &counterButtonState{} }

type counterButtonState struct {
	widgets.StateBase
	n int
}

func (s *counterButtonState) Build(widgets.BuildContext) widgets.Widget {
	return at(10, 10, widgets.GestureDetector{
		OnTap: func() { s.SetState(func() { s.n++ }) },
		Child: widgets.Text{Text: strings.Repeat("|", s.n)},
	})
}

func TestTapSetStateRebuilds(t *testing.T) {
	h := newHarness()
	h.mount(counterButton{})
	h.tap(50, 30)
	h.tap(50, 30)
	h.tap(50, 30)
	var got string
	var find func(ro render.RenderObject)
	find = func(ro render.RenderObject) {
		if p, ok := ro.(*render.RenderParagraph); ok {
			got = p.Text()
		}
		ro.VisitChildren(find)
	}
	find(h.po.Root())
	if got != "|||" {
		t.Fatalf("text after 3 taps = %q", got)
	}
}
