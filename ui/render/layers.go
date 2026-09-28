package render

import (
	"math"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// --- RenderStack -----------------------------------------------------------

// RenderStack paints children on top of each other. Children wrapped in
// RenderPositioned are placed by their edges; the others are aligned by
// Alignment. The stack sizes itself to its largest non-positioned child, or
// fills the constraints when Expand is set.
type RenderStack struct {
	Box
	MultiChild
	Alignment geom.Alignment
	Expand    bool
}

func (r *RenderStack) PerformLayout(c geom.Constraints) geom.Size {
	var size geom.Size
	if r.Expand {
		size = c.Biggest()
	}
	loose := c.Loosen()
	if r.Expand {
		loose = geom.Tight(size)
	}
	hasPlain := false
	for _, ch := range r.children {
		if _, ok := ch.(*RenderPositioned); ok {
			continue
		}
		hasPlain = true
		s := Layout(ch, loose)
		if !r.Expand {
			size.W, size.H = max(size.W, s.W), max(size.H, s.H)
		}
	}
	if !hasPlain && !r.Expand {
		size = c.Biggest()
	}
	size = c.Constrain(size)
	for _, ch := range r.children {
		if p, ok := ch.(*RenderPositioned); ok {
			p.place(size)
			continue
		}
		off := r.Alignment.Inscribe(size, ch.Base().size)
		SetOffset(ch, geom.Offset{X: round(off.X), Y: round(off.Y)})
	}
	return size
}

func (r *RenderStack) Paint(ctx *PaintContext, o geom.Offset) {
	for _, ch := range r.children {
		ctx.PaintChild(ch, o)
	}
}

// Edge is an optional edge distance for RenderPositioned.
type Edge struct {
	V   float32
	Set bool
}

// At returns a set Edge.
func At(v float32) Edge { return Edge{V: v, Set: true} }

// RenderPositioned places its child inside a RenderStack.
type RenderPositioned struct {
	Box
	SingleChild
	Left, Top, Right, Bottom Edge
	Width, Height            Edge
}

// PerformLayout is only used when not inside a stack.
func (r *RenderPositioned) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderPositioned) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// place lays the positioned child out inside a stack of the given size.
func (r *RenderPositioned) place(stack geom.Size) {
	axis := func(start, end, extent Edge, total float32) (min, max float32) {
		switch {
		case extent.Set:
			return extent.V, extent.V
		case start.Set && end.Set:
			v := math.Max(0, float64(total-start.V-end.V))
			return float32(v), float32(v)
		}
		return 0, geom.Inf
	}
	minW, maxW := axis(r.Left, r.Right, r.Width, stack.W)
	minH, maxH := axis(r.Top, r.Bottom, r.Height, stack.H)
	c := geom.Constraints{MinW: minW, MaxW: maxW, MinH: minH, MaxH: maxH}
	var s geom.Size
	if r.child != nil {
		s = Layout(r.child, c)
		SetOffset(r.child, geom.Offset{})
	} else {
		s = c.Constrain(geom.Size{})
	}
	r.size, r.laidOut, r.needsLayout = s, true, false
	var x, y float32
	switch {
	case r.Left.Set:
		x = r.Left.V
	case r.Right.Set:
		x = stack.W - r.Right.V - s.W
	default:
		x = (stack.W - s.W) / 2
	}
	switch {
	case r.Top.Set:
		y = r.Top.V
	case r.Bottom.Set:
		y = stack.H - r.Bottom.V - s.H
	default:
		y = (stack.H - s.H) / 2
	}
	r.offset = geom.Offset{X: round(x), Y: round(y)}
}

// --- RenderOpacity ---------------------------------------------------------

// RenderOpacity fades its subtree.
type RenderOpacity struct {
	Box
	SingleChild
	Opacity float32
}

func (r *RenderOpacity) PerformLayout(c geom.Constraints) geom.Size { return layoutProxy(r.child, c) }
func (r *RenderOpacity) Paint(ctx *PaintContext, o geom.Offset) {
	if r.Opacity <= 0 {
		return
	}
	ctx.Canvas.PushOpacity(r.Opacity)
	ctx.PaintChild(r.child, o)
	ctx.Canvas.PopOpacity()
}

// --- RenderTranslate -------------------------------------------------------

// RenderTranslate shifts its child when painting (and hit testing) without
// affecting layout. Fraction translates by a multiple of the child's size.
type RenderTranslate struct {
	Box
	SingleChild
	Offset   geom.Offset
	Fraction geom.Offset
}

func (r *RenderTranslate) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		return c.Smallest()
	}
	s := Layout(r.child, c)
	r.syncOffset(s)
	return s
}

func (r *RenderTranslate) syncOffset(s geom.Size) {
	if r.child != nil {
		SetOffset(r.child, geom.Offset{X: r.Offset.X + r.Fraction.X*s.W, Y: r.Offset.Y + r.Fraction.Y*s.H})
	}
}

// SetTranslation updates the shift without a relayout.
func (r *RenderTranslate) SetTranslation(off, frac geom.Offset) {
	if r.Offset == off && r.Fraction == frac {
		return
	}
	r.Offset, r.Fraction = off, frac
	r.syncOffset(r.size)
	MarkNeedsPaint(r)
}

func (r *RenderTranslate) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// HitTestSelf lets pointer events reach a child translated outside our box.
func (r *RenderTranslate) HitTestSelf(geom.Offset) bool { return false }

// --- RenderIgnorePointer ---------------------------------------------------

// RenderIgnorePointer makes its subtree invisible to hit testing.
type RenderIgnorePointer struct {
	Box
	SingleChild
	Ignoring bool
}

func (r *RenderIgnorePointer) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderIgnorePointer) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderIgnorePointer) IgnoresPointer() bool                   { return r.Ignoring }

// --- RenderAbsorbPointer ---------------------------------------------------

// RenderAbsorbPointer swallows hits so nothing below it (in the stack)
// receives them: used for modal barriers.
type RenderAbsorbPointer struct {
	Box
	SingleChild
}

func (r *RenderAbsorbPointer) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		return c.Biggest()
	}
	return layoutProxy(r.child, c)
}
func (r *RenderAbsorbPointer) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderAbsorbPointer) HitTestSelf(geom.Offset) bool           { return true }

// --- RenderWrap ------------------------------------------------------------

// RenderWrap flows children left-to-right, wrapping onto new runs.
type RenderWrap struct {
	Box
	MultiChild
	Spacing    float32 // between children in a run
	RunSpacing float32 // between runs
	Alignment  MainAxisAlignment
}

func (r *RenderWrap) PerformLayout(c geom.Constraints) geom.Size {
	maxW := c.MaxW
	type run struct {
		first, last int
		w, h        float32
	}
	var runs []run
	cur := run{}
	for i, ch := range r.children {
		s := Layout(ch, geom.Constraints{MaxW: maxW, MaxH: geom.Inf})
		gap := float32(0)
		if i > cur.first {
			gap = r.Spacing
		}
		if i > cur.first && cur.w+gap+s.W > maxW {
			cur.last = i
			runs = append(runs, cur)
			cur = run{first: i}
			gap = 0
		}
		cur.w += gap + s.W
		cur.h = max(cur.h, s.H)
	}
	cur.last = len(r.children)
	if len(r.children) > 0 {
		runs = append(runs, cur)
	}
	var w, y float32
	for _, rn := range runs {
		w = max(w, rn.w)
	}
	if c.HasBoundedWidth() && r.Alignment != MainStart {
		w = maxW
	}
	for ri, rn := range runs {
		if ri > 0 {
			y += r.RunSpacing
		}
		x := float32(0)
		free := w - rn.w
		switch r.Alignment {
		case MainCenter:
			x = free / 2
		case MainEnd:
			x = free
		}
		for i := rn.first; i < rn.last; i++ {
			ch := r.children[i]
			s := ch.Base().size
			SetOffset(ch, geom.Offset{X: round(x), Y: round(y + (rn.h-s.H)/2)})
			x += s.W + r.Spacing
		}
		y += rn.h
	}
	return c.Constrain(geom.Size{W: w, H: y})
}

func (r *RenderWrap) Paint(ctx *PaintContext, o geom.Offset) {
	for _, ch := range r.children {
		ctx.PaintChild(ch, o)
	}
}

// --- RenderCustomPaint -----------------------------------------------------

// Painter draws into a canvas at a given size. origin is the top-left in
// window coordinates.
type Painter func(c *Canvas, origin geom.Offset, size geom.Size)

// RenderCustomPaint calls Painter behind (and ForegroundPainter in front of)
// its child. Without a child it takes the Size it's given (clamped), or the
// smallest size its constraints allow.
type RenderCustomPaint struct {
	Box
	SingleChild
	Painter           Painter
	ForegroundPainter Painter
	PreferredSize     geom.Size
	HitTestable       bool
}

func (r *RenderCustomPaint) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		// Inf on an axis means "fill the available space" (or the minimum
		// when that axis is unbounded).
		s := r.PreferredSize
		if geom.IsInf(s.W) {
			s.W = c.Biggest().W
		}
		if geom.IsInf(s.H) {
			s.H = c.Biggest().H
		}
		return c.Constrain(s)
	}
	return layoutProxy(r.child, c)
}

func (r *RenderCustomPaint) Paint(ctx *PaintContext, o geom.Offset) {
	if r.Painter != nil {
		r.Painter(ctx.Canvas, o, r.size)
	}
	ctx.PaintChild(r.child, o)
	if r.ForegroundPainter != nil {
		r.ForegroundPainter(ctx.Canvas, o, r.size)
	}
}

func (r *RenderCustomPaint) HitTestSelf(geom.Offset) bool { return r.HitTestable }

// --- RenderIntrinsic helpers -----------------------------------------------

// RenderFractionallySized sizes its child to a fraction of the available
// space (FractionallySizedBox). Zero factors mean "don't constrain".
type RenderFractionallySized struct {
	Box
	SingleChild
	WidthFactor, HeightFactor float32
}

func (r *RenderFractionallySized) PerformLayout(c geom.Constraints) geom.Size {
	cc := c
	if r.WidthFactor > 0 && c.HasBoundedWidth() {
		cc = cc.WithTightWidth(c.MaxW * r.WidthFactor)
	}
	if r.HeightFactor > 0 && c.HasBoundedHeight() {
		cc = cc.WithTightHeight(c.MaxH * r.HeightFactor)
	}
	if r.child == nil {
		return cc.Constrain(geom.Size{})
	}
	s := Layout(r.child, cc)
	SetOffset(r.child, geom.Offset{})
	return s
}

func (r *RenderFractionallySized) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }

// --- RenderSizeFactor ------------------------------------------------------

// RenderSizeFactor lays its child out at its natural size along one axis
// and reports Factor × that size, clipping the rest (expand/collapse
// animations).
type RenderSizeFactor struct {
	Box
	SingleChild
	Factor     float32
	Horizontal bool
}

func (r *RenderSizeFactor) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		return c.Smallest()
	}
	cc := c
	if r.Horizontal {
		cc.MinW, cc.MaxW = 0, geom.Inf
	} else {
		cc.MinH, cc.MaxH = 0, geom.Inf
	}
	s := Layout(r.child, cc)
	SetOffset(r.child, geom.Offset{})
	f := min(max(r.Factor, 0), 1)
	if r.Horizontal {
		s.W *= f
	} else {
		s.H *= f
	}
	return s
}

func (r *RenderSizeFactor) Paint(ctx *PaintContext, o geom.Offset) {
	if r.Factor <= 0 {
		return
	}
	ctx.Canvas.PushClip(geom.RectFrom(o, r.size))
	ctx.PaintChild(r.child, o)
	ctx.Canvas.PopClip()
}
