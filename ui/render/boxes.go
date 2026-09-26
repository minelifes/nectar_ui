package render

import "nectar_ui/ui/geom"

// Base implements RenderObject.Base for every type that embeds Box.
func (b *Box) Base() *Box { return b }

// --- child containers ------------------------------------------------------

// SingleChild is embedded by render objects that have at most one child.
type SingleChild struct{ child RenderObject }

func (s *SingleChild) Child() RenderObject     { return s.child }
func (s *SingleChild) ChildSlot() *SingleChild { return s }
func (s *SingleChild) VisitChildren(fn func(RenderObject)) {
	if s.child != nil {
		fn(s.child)
	}
}

// SingleChildHolder is implemented by anything embedding SingleChild.
type SingleChildHolder interface {
	RenderObject
	ChildSlot() *SingleChild
}

// SetChild replaces the child of parent.
func SetChild(parent SingleChildHolder, child RenderObject) {
	s := parent.ChildSlot()
	if s.child == child {
		return
	}
	Drop(parent, s.child)
	s.child = child
	Adopt(parent, child)
}

// MultiChild is embedded by render objects with a list of children.
type MultiChild struct{ children []RenderObject }

func (m *MultiChild) Children() []RenderObject  { return m.children }
func (m *MultiChild) ChildrenSlot() *MultiChild { return m }
func (m *MultiChild) VisitChildren(fn func(RenderObject)) {
	for _, c := range m.children {
		fn(c)
	}
}

// MultiChildHolder is implemented by anything embedding MultiChild.
type MultiChildHolder interface {
	RenderObject
	ChildrenSlot() *MultiChild
}

// SetChildren replaces the children of parent, adopting/dropping as needed.
func SetChildren(parent MultiChildHolder, children []RenderObject) {
	m := parent.ChildrenSlot()
	if sameList(m.children, children) {
		return
	}
	keep := make(map[RenderObject]bool, len(children))
	for _, c := range children {
		keep[c] = true
	}
	for _, c := range m.children {
		if !keep[c] {
			Drop(parent, c)
		}
	}
	old := make(map[RenderObject]bool, len(m.children))
	for _, c := range m.children {
		old[c] = true
	}
	m.children = append(m.children[:0:0], children...)
	for _, c := range m.children {
		if !old[c] {
			Adopt(parent, c)
		}
	}
	MarkNeedsLayout(parent)
}

func sameList(a, b []RenderObject) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// layoutProxy is the default single-child layout: pass constraints through.
func layoutProxy(child RenderObject, c geom.Constraints) geom.Size {
	if child == nil {
		return c.Smallest()
	}
	s := Layout(child, c)
	SetOffset(child, geom.Offset{})
	return s
}

// --- RenderView ------------------------------------------------------------

// RenderView is the root of the render tree; it fills the window.
type RenderView struct {
	Box
	SingleChild
	Background geom.Color
}

func (r *RenderView) PerformLayout(c geom.Constraints) geom.Size {
	layoutProxy(r.child, c)
	return c.Biggest()
}

func (r *RenderView) Paint(ctx *PaintContext, o geom.Offset) {
	ctx.Canvas.FillRect(geom.RectFrom(o, r.size), r.Background)
	ctx.PaintChild(r.child, o)
}

// --- RenderDecoratedBox ----------------------------------------------------

// RenderDecoratedBox paints a (rounded, optionally bordered) background
// behind its child. Without a child it expands to fill its constraints.
type RenderDecoratedBox struct {
	Box
	SingleChild
	Color  geom.Color
	Border *geom.Border
}

func (r *RenderDecoratedBox) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		return c.Biggest()
	}
	return layoutProxy(r.child, c)
}

func (r *RenderDecoratedBox) Paint(ctx *PaintContext, o geom.Offset) {
	rect := geom.RectFrom(o, r.size)
	if r.Border != nil {
		border := r.Border
		if border.Width > 0 && border.Color.A > 0 {
			ctx.Canvas.FillRoundRect(rect, border.Radius, border.Color)
			bw := border.Width
			rect = rect.Deflate(geom.Insets(bw))
			ctx.Canvas.FillRoundRect(rect, max(0, border.Radius-bw), r.Color)
		} else {
			ctx.Canvas.FillRoundRect(rect, border.Radius, r.Color)
		}
	} else {
		ctx.Canvas.FillRect(rect, r.Color)
	}
	ctx.PaintChild(r.child, o)
}

// --- RenderPadding ---------------------------------------------------------

// RenderPadding insets its child.
type RenderPadding struct {
	Box
	SingleChild
	Padding geom.EdgeInsets
}

func (r *RenderPadding) PerformLayout(c geom.Constraints) geom.Size {
	p := r.Padding
	if r.child == nil {
		return c.Constrain(geom.Size{W: p.Horizontal(), H: p.Vertical()})
	}
	s := Layout(r.child, c.Deflate(p))
	SetOffset(r.child, geom.Offset{X: p.Left, Y: p.Top})
	return geom.Size{W: s.W + p.Horizontal(), H: s.H + p.Vertical()}
}

func (r *RenderPadding) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// --- RenderConstrainedBox --------------------------------------------------

// RenderConstrainedBox applies additional constraints (SizedBox, min/max).
type RenderConstrainedBox struct {
	Box
	SingleChild
	Additional geom.Constraints
}

func (r *RenderConstrainedBox) PerformLayout(c geom.Constraints) geom.Size {
	cc := r.Additional.Enforce(c)
	if r.child == nil {
		return cc.Constrain(geom.Size{})
	}
	return layoutProxy(r.child, cc)
}

func (r *RenderConstrainedBox) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// --- RenderAlign -----------------------------------------------------------

// RenderAlign positions its child inside itself. It expands to fill bounded
// constraints and shrink-wraps the child on unbounded axes.
type RenderAlign struct {
	Box
	SingleChild
	Alignment geom.Alignment
}

func (r *RenderAlign) PerformLayout(c geom.Constraints) geom.Size {
	var cs geom.Size
	if r.child != nil {
		cs = Layout(r.child, c.Loosen())
	}
	size := c.Biggest()
	if !c.HasBoundedWidth() {
		size.W = cs.W
	}
	if !c.HasBoundedHeight() {
		size.H = cs.H
	}
	size = c.Constrain(size)
	if r.child != nil {
		off := r.Alignment.Inscribe(size, cs)
		SetOffset(r.child, geom.Offset{X: round(off.X), Y: round(off.Y)})
	}
	return size
}

func (r *RenderAlign) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// --- RenderClipRect --------------------------------------------------------

// RenderClipRect clips its child to its own bounds.
type RenderClipRect struct {
	Box
	SingleChild
}

func (r *RenderClipRect) PerformLayout(c geom.Constraints) geom.Size { return layoutProxy(r.child, c) }

func (r *RenderClipRect) Paint(ctx *PaintContext, o geom.Offset) {
	ctx.Canvas.PushClip(geom.RectFrom(o, r.size))
	ctx.PaintChild(r.child, o)
	ctx.Canvas.PopClip()
}

// --- RenderFlexible --------------------------------------------------------

// RenderFlexible marks its child as flexible inside a RenderFlex.
type RenderFlexible struct {
	Box
	SingleChild
	Flex int  // share of the remaining space
	Fit  bool // true = child must fill its share (Expanded)
}

func (r *RenderFlexible) PerformLayout(c geom.Constraints) geom.Size { return layoutProxy(r.child, c) }
func (r *RenderFlexible) Paint(ctx *PaintContext, o geom.Offset)     { ctx.PaintChild(r.child, o) }

func round(v float32) float32 {
	if v < 0 {
		return -float32(int(-v + 0.5))
	}
	return float32(int(v + 0.5))
}
