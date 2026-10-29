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
	over    float32 // overscroll (bouncing physics): <0 past the start, >0 past the end
	content float32
}

// Overscroller is a scroll axis that can show content pulled past its
// edges (bouncing scroll physics): the content is drawn shifted by v on
// top of the (valid, clamped) scroll offset.
type Overscroller interface {
	SetOverscroll(v float32)
	Overscroll() float32
}

// SetOverscroll shifts the content past its edge by v (negative: past the
// start). Paint only; no relayout.
func (r *RenderViewport) SetOverscroll(v float32) {
	if v != r.over {
		r.over = v
		r.syncChild()
		MarkNeedsPaint(r)
	}
}

// Overscroll returns the current overscroll.
func (r *RenderViewport) Overscroll() float32 { return r.over }

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
	v := r.offset + r.over
	if r.Axis == Vertical {
		SetOffset(r.child, geom.Offset{Y: -v})
	} else {
		SetOffset(r.child, geom.Offset{X: -v})
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

// PaintBleed is how far shadows (and focus rings) may paint past the edge
// of a scroll view or split pane where nothing is actually cut off. It
// covers the deepest Material elevation shadow.
const PaintBleed float32 = 24

func (r *RenderViewport) Paint(ctx *PaintContext, o geom.Offset) {
	rect := geom.RectFrom(o, r.size)
	// Clip hard only at edges the content has scrolled past: at the start
	// (and end) of the list nothing is hidden there, so a shadow of the
	// first (last) item stays visible instead of being cut at the edge.
	// The cross axis never hides content either.
	clip := rect
	atStart, atEnd := r.offset <= 0.5, r.offset >= r.MaxOffset()-0.5
	if r.over != 0 { // content pulled past an edge: nothing may bleed out
		atStart, atEnd = false, false
	}
	if r.Axis == Vertical {
		clip.X, clip.W = clip.X-PaintBleed, clip.W+2*PaintBleed
		if atStart {
			clip.Y, clip.H = clip.Y-PaintBleed, clip.H+PaintBleed
		}
		if atEnd {
			clip.H += PaintBleed
		}
	} else {
		clip.Y, clip.H = clip.Y-PaintBleed, clip.H+2*PaintBleed
		if atStart {
			clip.X, clip.W = clip.X-PaintBleed, clip.W+PaintBleed
		}
		if atEnd {
			clip.W += PaintBleed
		}
	}
	ctx.Canvas.PushClip(clip)
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
	// Overscrolled, the thumb shrinks at that end (like iOS / macOS).
	over := abs32(r.over)
	thumb := max(ext*ext/r.content-over, 12)
	pos := (ext - max(ext*ext/r.content, 24)) * (r.offset / r.MaxOffset())
	switch {
	case r.over < 0:
		pos = 0
	case r.over > 0:
		pos = ext - thumb
	default:
		thumb = max(ext*ext/r.content, 24)
	}
	var tr geom.Rect
	if r.Axis == Vertical {
		tr = geom.Rect{X: rect.Right() - w - 2, Y: rect.Y + pos, W: w, H: thumb}
	} else {
		tr = geom.Rect{X: rect.X + pos, Y: rect.Bottom() - w - 2, W: thumb, H: w}
	}
	ctx.Canvas.FillRoundRect(tr, w/2, r.ThumbColor)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
