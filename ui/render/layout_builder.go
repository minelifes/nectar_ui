package render

import "github.com/minelifes/nectar_ui/ui/geom"

// RenderLayoutBuilder calls OnLayout with its incoming constraints before
// laying out its child, so the widget layer can (re)build the child for
// those constraints (widgets.LayoutBuilder). The child is laid out with
// the same constraints and the node takes its size.
type RenderLayoutBuilder struct {
	Box
	SingleChild
	// OnLayout runs at the start of every layout of this node; it may
	// replace the child.
	OnLayout func(c geom.Constraints)
}

func (r *RenderLayoutBuilder) PerformLayout(c geom.Constraints) geom.Size {
	if r.OnLayout != nil {
		r.OnLayout(c)
	}
	if r.child == nil {
		return c.Smallest()
	}
	s := Layout(r.child, c)
	SetOffset(r.child, geom.Offset{})
	return s
}

func (r *RenderLayoutBuilder) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
