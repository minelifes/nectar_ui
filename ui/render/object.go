// Package render is the layout + paint layer (Flutter's RenderObject tree).
//
// Every node embeds *Box via RenderObject.Base(). Parents lay out children
// with render.Layout(child, constraints), set the child's offset, and paint
// children with ctx.PaintChild(child, offset). Painting records commands into
// a Canvas display list; the gpu package turns that list into draw calls.
package render

import "nectar_ui/ui/geom"

// RenderObject is a node of the render tree.
type RenderObject interface {
	// Base gives access to the shared layout state.
	Base() *Box
	// PerformLayout computes the node's size under c. It must lay out its
	// children via Layout() and position them via SetOffset. The returned
	// size is clamped into c by the caller.
	PerformLayout(c geom.Constraints) geom.Size
	// Paint draws the node at offset (its top-left in window coordinates).
	Paint(ctx *PaintContext, offset geom.Offset)
	// VisitChildren calls fn for each direct child.
	VisitChildren(fn func(RenderObject))
}

// HitTester is optionally implemented by render objects that want to take
// part in pointer hit testing (buttons, gesture detectors, ...).
type HitTester interface {
	// HitTestSelf reports whether the local position hits this node.
	HitTestSelf(local geom.Offset) bool
}

// Box holds the state common to every render object.
type Box struct {
	parent      RenderObject
	owner       *PipelineOwner
	size        geom.Size
	offset      geom.Offset // position inside parent, set by the parent
	constraints geom.Constraints
	needsLayout bool
	laidOut     bool
	depth       int

	// ParentData is free-form data owned by the parent (e.g. flex factor).
	ParentData any
}

func (b *Box) Size() geom.Size               { return b.size }
func (b *Box) Offset() geom.Offset           { return b.offset }
func (b *Box) SetOffset(o geom.Offset)       { b.offset = o }
func (b *Box) Parent() RenderObject          { return b.parent }
func (b *Box) Owner() *PipelineOwner         { return b.owner }
func (b *Box) Constraints() geom.Constraints { return b.constraints }
func (b *Box) NeedsLayout() bool             { return b.needsLayout || !b.laidOut }
func (b *Box) Depth() int                    { return b.depth }

// Layout lays out ro under c, reusing the previous result when neither the
// constraints nor the node changed. Returns the resulting size.
func Layout(ro RenderObject, c geom.Constraints) geom.Size {
	b := ro.Base()
	if b.laidOut && !b.needsLayout && b.constraints == c {
		return b.size
	}
	b.constraints = c
	b.size = c.Constrain(ro.PerformLayout(c))
	b.needsLayout = false
	b.laidOut = true
	return b.size
}

// SetOffset positions a child inside its parent.
func SetOffset(ro RenderObject, o geom.Offset) { ro.Base().offset = o }

// MarkNeedsLayout flags ro and its ancestors for re-layout and schedules a
// frame. Call it from property setters that affect size.
func MarkNeedsLayout(ro RenderObject) {
	for n := ro; n != nil; n = n.Base().parent {
		b := n.Base()
		if b.needsLayout {
			break
		}
		b.needsLayout = true
	}
	MarkNeedsPaint(ro)
}

// MarkNeedsPaint schedules a repaint. (The whole tree is repainted each
// frame; the display list is cheap. Layers/caching can be added later.)
func MarkNeedsPaint(ro RenderObject) {
	if o := ro.Base().owner; o != nil {
		o.requestVisualUpdate()
	}
}

// Adopt makes child a child of parent (call from SetChild/SetChildren).
func Adopt(parent, child RenderObject) {
	if child == nil {
		return
	}
	pb, cb := parent.Base(), child.Base()
	cb.parent = parent
	cb.depth = pb.depth + 1
	if pb.owner != nil {
		attach(child, pb.owner)
	}
	MarkNeedsLayout(parent)
}

// Drop detaches child from its parent.
func Drop(parent, child RenderObject) {
	if child == nil {
		return
	}
	cb := child.Base()
	if cb.parent == parent {
		cb.parent = nil
		detach(child)
	}
	MarkNeedsLayout(parent)
}

func attach(ro RenderObject, o *PipelineOwner) {
	b := ro.Base()
	b.owner = o
	b.needsLayout = true
	ro.VisitChildren(func(c RenderObject) {
		c.Base().depth = b.depth + 1
		attach(c, o)
	})
}

func detach(ro RenderObject) {
	ro.Base().owner = nil
	ro.VisitChildren(detach)
}

// HitTest collects the path of render objects under position (deepest
// first). position is in the coordinate space of ro's parent.
func HitTest(ro RenderObject, position geom.Offset, out []RenderObject) []RenderObject {
	b := ro.Base()
	local := position.Sub(b.offset)
	if !geom.RectFrom(geom.Offset{}, b.size).Contains(local) {
		return out
	}
	// Children painted last are on top, so test them first.
	var kids []RenderObject
	ro.VisitChildren(func(c RenderObject) { kids = append(kids, c) })
	for i := len(kids) - 1; i >= 0; i-- {
		n := len(out)
		out = HitTest(kids[i], local, out)
		if len(out) > n {
			break
		}
	}
	if ht, ok := ro.(HitTester); ok && ht.HitTestSelf(local) {
		out = append(out, ro)
	} else if len(out) > 0 {
		out = append(out, ro) // ancestors of a hit are on the path too
	}
	return out
}

// PipelineOwner drives layout and paint for a render tree.
type PipelineOwner struct {
	root RenderObject
	// OnNeedVisualUpdate is called whenever something needs a new frame.
	OnNeedVisualUpdate func()
	canvas             Canvas
}

// NewPipelineOwner creates an owner; set the root with SetRoot.
func NewPipelineOwner() *PipelineOwner { return &PipelineOwner{} }

// SetRoot attaches root to the owner.
func (o *PipelineOwner) SetRoot(root RenderObject) {
	if o.root != nil {
		detach(o.root)
	}
	o.root = root
	if root != nil {
		root.Base().depth = 0
		attach(root, o)
	}
	o.requestVisualUpdate()
}

// Root returns the root render object.
func (o *PipelineOwner) Root() RenderObject { return o.root }

func (o *PipelineOwner) requestVisualUpdate() {
	if o.OnNeedVisualUpdate != nil {
		o.OnNeedVisualUpdate()
	}
}

// FlushLayout lays out the tree to fill a window of the given logical size.
func (o *PipelineOwner) FlushLayout(window geom.Size) {
	if o.root == nil {
		return
	}
	Layout(o.root, geom.Tight(window))
	o.root.Base().offset = geom.Offset{}
}

// FlushPaint paints the tree and returns the recorded display list. The
// returned canvas is reused next frame.
func (o *PipelineOwner) FlushPaint(window geom.Size) *Canvas {
	o.canvas.Reset(geom.Rect{W: window.W, H: window.H})
	if o.root != nil {
		ctx := &PaintContext{Canvas: &o.canvas}
		ctx.PaintChild(o.root, geom.Offset{})
	}
	return &o.canvas
}

// PaintContext is passed down during painting.
type PaintContext struct {
	Canvas *Canvas
}

// PaintChild paints child at parentOffset + child's own offset.
func (ctx *PaintContext) PaintChild(child RenderObject, parentOffset geom.Offset) {
	if child == nil {
		return
	}
	b := child.Base()
	o := parentOffset.Add(b.offset)
	// Skip subtrees that are entirely clipped away.
	if b.size.W > 0 && b.size.H > 0 && ctx.Canvas.Clip().Intersect(geom.RectFrom(o, b.size)).Empty() {
		return
	}
	child.Paint(ctx, o)
}
