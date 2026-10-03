package render

import "github.com/minelifes/nectar_ui/ui/geom"

// ScrollAxis is one scrollable axis of a viewport: what a scroll
// controller drives. RenderViewport is one; RenderViewport2D has two
// (see Axis).
type ScrollAxis interface {
	Offset() float32
	MaxOffset() float32
	ViewportExtent() float32
	SetScrollOffset(v float32) float32
	// Viewport is the render object that owns the axis (for claiming
	// pointer events).
	Viewport() RenderObject
}

// Viewport returns r itself.
func (r *RenderViewport) Viewport() RenderObject { return r }

// RenderViewport2D shows a window onto a child that may be larger than
// itself in both directions: a table, a canvas, an image, code without
// wrapping. The child gets unbounded constraints on both axes (but at least
// the viewport's size, so it can fill it).
type RenderViewport2D struct {
	Box
	SingleChild

	// Scrollbar appearance (zero ThumbColor = no scrollbars).
	ThumbColor geom.Color
	ThumbWidth float32

	// OnMetrics is called after layout with the viewport and content sizes.
	OnMetrics func(viewport, content geom.Size)

	off     geom.Offset
	content geom.Size
}

// Axis returns a ScrollAxis for horizontal or vertical scrolling.
func (r *RenderViewport2D) Axis(a Axis) ScrollAxis { return viewportAxis{r, a} }

// ScrollOffset returns the current offset on both axes.
func (r *RenderViewport2D) ScrollOffset() geom.Offset { return r.off }

// ContentSize returns the child's size.
func (r *RenderViewport2D) ContentSize() geom.Size { return r.content }

// MaxScrollOffset returns the largest valid offsets.
func (r *RenderViewport2D) MaxScrollOffset() geom.Offset {
	return geom.Offset{X: max(0, r.content.W-r.size.W), Y: max(0, r.content.H-r.size.H)}
}

// SetScrollOffset2D scrolls (clamped) without relayout and returns the
// applied offset.
func (r *RenderViewport2D) SetScrollOffset2D(o geom.Offset) geom.Offset {
	if r.laidOut {
		m := r.MaxScrollOffset()
		o.X, o.Y = min(max(o.X, 0), m.X), min(max(o.Y, 0), m.Y)
	} else {
		o.X, o.Y = max(o.X, 0), max(o.Y, 0)
	}
	if o != r.off {
		r.off = o
		r.syncChild()
		MarkNeedsPaint(r)
	}
	return r.off
}

func (r *RenderViewport2D) syncChild() {
	if r.child != nil {
		SetOffset(r.child, geom.Offset{X: -r.off.X, Y: -r.off.Y})
	}
}

func (r *RenderViewport2D) PerformLayout(c geom.Constraints) geom.Size {
	size := c.Biggest()
	if !c.HasBoundedWidth() || !c.HasBoundedHeight() {
		size = c.Smallest()
	}
	if r.child == nil {
		r.content = geom.Size{}
		return size
	}
	cc := geom.Constraints{MaxW: geom.Inf, MaxH: geom.Inf}
	if c.HasBoundedWidth() {
		cc.MinW = c.MaxW
	}
	if c.HasBoundedHeight() {
		cc.MinH = c.MaxH
	}
	r.content = Layout(r.child, cc)
	if !c.HasBoundedWidth() || !c.HasBoundedHeight() {
		size = c.Constrain(r.content)
	}
	r.size = size
	m := r.MaxScrollOffset()
	r.off.X, r.off.Y = min(max(r.off.X, 0), m.X), min(max(r.off.Y, 0), m.Y)
	r.syncChild()
	if r.OnMetrics != nil {
		r.OnMetrics(size, r.content)
	}
	return size
}

func (r *RenderViewport2D) Paint(ctx *PaintContext, o geom.Offset) {
	rect := geom.RectFrom(o, r.size)
	ctx.Canvas.PushClip(rect)
	ctx.PaintChild(r.child, o)
	ctx.Canvas.PopClip()
	if r.ThumbColor.A <= 0 {
		return
	}
	w := r.ThumbWidth
	if w <= 0 {
		w = 4
	}
	m := r.MaxScrollOffset()
	if m.Y > 0.5 {
		ext := r.size.H
		thumb := max(ext*ext/r.content.H, 24)
		pos := (ext - thumb) * (r.off.Y / m.Y)
		ctx.Canvas.FillRoundRect(geom.Rect{X: rect.Right() - w - 2, Y: rect.Y + pos, W: w, H: thumb}, w/2, r.ThumbColor)
	}
	if m.X > 0.5 {
		ext := r.size.W
		thumb := max(ext*ext/r.content.W, 24)
		pos := (ext - thumb) * (r.off.X / m.X)
		ctx.Canvas.FillRoundRect(geom.Rect{X: rect.X + pos, Y: rect.Bottom() - w - 2, W: thumb, H: w}, w/2, r.ThumbColor)
	}
}

// viewportAxis adapts one axis of a RenderViewport2D to ScrollAxis.
type viewportAxis struct {
	r *RenderViewport2D
	a Axis
}

func (v viewportAxis) Offset() float32 {
	if v.a == Horizontal {
		return v.r.off.X
	}
	return v.r.off.Y
}

func (v viewportAxis) MaxOffset() float32 {
	m := v.r.MaxScrollOffset()
	if v.a == Horizontal {
		return m.X
	}
	return m.Y
}

func (v viewportAxis) ViewportExtent() float32 {
	if v.a == Horizontal {
		return v.r.size.W
	}
	return v.r.size.H
}

func (v viewportAxis) SetScrollOffset(x float32) float32 {
	o := v.r.off
	if v.a == Horizontal {
		o.X = x
	} else {
		o.Y = x
	}
	o = v.r.SetScrollOffset2D(o)
	if v.a == Horizontal {
		return o.X
	}
	return o.Y
}

func (v viewportAxis) Viewport() RenderObject { return v.r }
