package widgets

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/render"
)

// Introspection for developer tools (inspectors, tree dumps) and tests.

// VisitChildContexts calls fn for each child element of ctx.
func VisitChildContexts(ctx BuildContext, fn func(child BuildContext)) {
	if e, ok := ctx.(Element); ok {
		e.visitChildren(func(c Element) { fn(c) })
	}
}

// OwnRenderObject returns the render object created by ctx's own widget
// (a RenderObjectWidget), or nil for widgets that only build others.
func OwnRenderObject(ctx BuildContext) render.RenderObject {
	if e, ok := ctx.(renderElement); ok {
		return e.RenderObject()
	}
	return nil
}

// ElementOf returns the element (below root) whose widget created ro.
func ElementOf(root BuildContext, ro render.RenderObject) BuildContext {
	var found BuildContext
	var walk func(c BuildContext)
	walk = func(c BuildContext) {
		if found != nil {
			return
		}
		if OwnRenderObject(c) == ro {
			found = c
			return
		}
		VisitChildContexts(c, walk)
	}
	walk(root)
	return found
}

// FrameStats measures the last frame; the app (and the tester) fill it in.
type FrameStats struct {
	Frames   uint64        // frames so far
	Build    time.Duration // FlushBuild of the last frame
	Layout   time.Duration
	Paint    time.Duration
	Rebuilds int // elements rebuilt in the last frame
	Commands int // display-list commands painted
	// Interval is the time between the last two frames.
	Interval time.Duration
	last     time.Time
}

// Stats returns the frame statistics of the tree.
func (o *BuildOwner) Stats() FrameStats { return o.stats }

// RecordFrame stores one frame's measurements (called by the app's frame
// loop after painting).
func (o *BuildOwner) RecordFrame(build, layout, paint time.Duration, commands int) {
	now := o.now()
	s := &o.stats
	if !s.last.IsZero() {
		s.Interval = now.Sub(s.last)
	}
	s.last = now
	s.Frames++
	s.Build, s.Layout, s.Paint, s.Commands = build, layout, paint, commands
	s.Rebuilds = o.rebuilds
	o.rebuilds = 0
}
