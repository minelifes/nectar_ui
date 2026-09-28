package render

import "github.com/minelifes/nectar_ui/ui/geom"

// RenderViewport shows a window onto a child that may be larger than itself
// along Axis, shifted by the scroll offset and clipped.
type RenderViewport struct {
	Box
	SingleChild
	Axis Axis

	// Scrollbar appearance (zero ThumbColor = no scrollbar).
	ThumbColor geom.Color
	ThumbWidth float32

	// OnMetrics is called after layout with the viewport and content extents.
	OnMetrics func(viewport, content float32)

	offset  float32
	content float32
}

// Offset returns the current scroll offset.
func (r *RenderViewport) Offset() float32 { return r.offset }

// MaxOffset returns the largest valid scroll offset.
func (r *RenderViewport) MaxOffset() float32 { return max(0, r.content-r.extent()) }

// ViewportExtent returns the visible size along the axis.
func (r *RenderViewport) ViewportExtent() float32 { return r.extent() }

func (r *RenderViewport) extent() float32 {
	if r.Axis == Vertical {
		return r.size.H
	}
	return r.size.W
}

// SetOffset scrolls (clamped) without relayout. Returns the applied offset.
func (r *RenderViewport) SetScrollOffset(v float32) float32 {
	if r.laidOut {
		v = min(max(v, 0), r.MaxOffset())
	}
	if v != r.offset {
		r.offset = v
		r.syncChild()
		MarkNeedsPaint(r)
	}
	return r.offset
}

func (r *RenderViewport) syncChild() {
	if r.child == nil {
		return
	}
	if r.Axis == Vertical {
		SetOffset(r.child, geom.Offset{Y: -r.offset})
	} else {
		SetOffset(r.child, geom.Offset{X: -r.offset})
	}
}

func (r *RenderViewport) PerformLayout(c geom.Constraints) geom.Size {
	if r.child == nil {
		r.content = 0
		return c.Smallest()
	}
	var cc geom.Constraints
	if r.Axis == Vertical {
		cc = geom.Constraints{MinW: c.MaxW, MaxW: c.MaxW, MaxH: geom.Inf}
		if !c.HasBoundedWidth() {
			cc.MinW = 0
		}
	} else {
		cc = geom.Constraints{MaxW: geom.Inf, MinH: c.MaxH, MaxH: c.MaxH}
		if !c.HasBoundedHeight() {
			cc.MinH = 0
		}
	}
	cs := Layout(r.child, cc)
	size := c.Constrain(cs)
	if r.Axis == Vertical {
		r.content = cs.H
		if c.HasBoundedHeight() {
			size.H = c.MaxH
		}
	} else {
		r.content = cs.W
		if c.HasBoundedWidth() {
			size.W = c.MaxW
		}
	}
	r.size = size
	r.offset = min(max(r.offset, 0), r.MaxOffset())
	r.syncChild()
	if r.OnMetrics != nil {
		r.OnMetrics(r.extent(), r.content)
	}
	return size
}

func (r *RenderViewport) Paint(ctx *PaintContext, o geom.Offset) {
	rect := geom.RectFrom(o, r.size)
	ctx.Canvas.PushClip(rect)
	ctx.PaintChild(r.child, o)
	ctx.Canvas.PopClip()
	ext := r.extent()
	if r.ThumbColor.A <= 0 || r.content <= ext+0.5 || ext <= 0 {
		return
	}
	w := r.ThumbWidth
	if w <= 0 {
		w = 4
	}
	thumb := max(ext*ext/r.content, 24)
	pos := (ext - thumb) * (r.offset / r.MaxOffset())
	var tr geom.Rect
	if r.Axis == Vertical {
		tr = geom.Rect{X: rect.Right() - w - 2, Y: rect.Y + pos, W: w, H: thumb}
	} else {
		tr = geom.Rect{X: rect.X + pos, Y: rect.Bottom() - w - 2, W: thumb, H: w}
	}
	ctx.Canvas.FillRoundRect(tr, w/2, r.ThumbColor)
}
