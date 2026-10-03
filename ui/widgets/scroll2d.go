package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// ScrollView2D scrolls its child in both directions: the mouse wheel
// scrolls vertically (horizontally with Shift, or with a trackpad's
// sideways swipe), and dragging pans. The child is laid out at its natural
// size (at least the view's). Pass controllers to read or drive either
// axis; ScrollView2D must get bounded constraints.
type ScrollView2D struct {
	Horizontal *ScrollController // nil = internal
	Vertical   *ScrollController // nil = internal
	ThumbColor geom.Color        // scrollbars; zero = none
	Child      Widget
}

func (ScrollView2D) CreateState() State { return &scroll2DState{} }

type scroll2DState struct {
	StateBase
	h, v *ScrollController
}

func (s *scroll2DState) InitState() { s.controllers() }

func (s *scroll2DState) DidUpdateWidget(Widget) { s.controllers() }

func (s *scroll2DState) controllers() {
	w := WidgetOf[ScrollView2D](s)
	if w.Horizontal != nil {
		s.h = w.Horizontal
	} else if s.h == nil {
		s.h = NewScrollController()
	}
	if w.Vertical != nil {
		s.v = w.Vertical
	} else if s.v == nil {
		s.v = NewScrollController()
	}
}

func (s *scroll2DState) Build(ctx BuildContext) Widget {
	w := WidgetOf[ScrollView2D](s)
	h, v := s.h, s.v
	return Listener{
		OnEvent: func(e PointerEvent) {
			if e.Kind != render.PointerScroll || e.Winner() != nil {
				return
			}
			moved := false
			if e.Scroll.X != 0 {
				moved = h.ScrollBy(e.Scroll.X) || moved
			}
			if e.Scroll.Y != 0 {
				if Modifiers(e.Mods).Shift() && e.Scroll.X == 0 {
					moved = h.ScrollBy(e.Scroll.Y) || moved
				} else {
					moved = v.ScrollBy(e.Scroll.Y) || moved
				}
			}
			if moved && v.vp != nil {
				e.Claim(v.vp.Viewport())
			}
		},
		Child: GestureDetector{
			OnPanUpdate: func(d DragDetails) {
				h.ScrollBy(-d.Delta.X)
				v.ScrollBy(-d.Delta.Y)
			},
			Child: viewport2D{h: h, v: v, thumb: w.ThumbColor, owner: ctx.Owner(), child: RepaintBoundary{Child: w.Child}},
		},
	}
}

type viewport2D struct {
	h, v  *ScrollController
	thumb geom.Color
	owner *BuildOwner
	child Widget
}

func (w viewport2D) ChildWidget() Widget { return w.child }

func (w viewport2D) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderViewport2D{ThumbColor: w.thumb}
	w.attach(r)
	return r
}

func (w viewport2D) attach(r *render.RenderViewport2D) {
	w.h.attachAxis(r.Axis(render.Horizontal), w.owner)
	w.v.attachAxis(r.Axis(render.Vertical), w.owner)
	h, v := w.h, w.v
	r.OnMetrics = func(view, content geom.Size) {
		h.metrics(view.W, content.W)
		v.metrics(view.H, content.H)
	}
}

func (viewport2D) MarksOwnPaint() {}

func (w viewport2D) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderViewport2D)
	if r.ThumbColor != w.thumb {
		r.ThumbColor = w.thumb
		render.MarkNeedsPaint(r)
	}
	if w.h.vp == nil || w.h.vp.Viewport() != r || w.v.vp == nil || w.v.vp.Viewport() != r {
		w.attach(r)
	}
}
