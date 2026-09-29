package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// LayoutBuilder builds its child during layout, from the constraints its
// parent gives it (Flutter's LayoutBuilder). Use it to pick a different
// layout for the space actually available: a sidebar on wide windows and a
// drawer on narrow ones, a grid whose column count follows the width, and
// so on. Unlike WindowOf(ctx).Size() it reacts to the space of this very
// spot in the tree (a split pane, a dialog, a card).
//
//	widgets.LayoutBuilder{Builder: func(ctx widgets.BuildContext, c geom.Constraints) widgets.Widget {
//	    if c.MaxW >= 720 {
//	        return widgets.Row{Children: []widgets.Widget{sidebar, content}}
//	    }
//	    return content
//	}}
//
// The builder runs again when the constraints change, when the
// LayoutBuilder is rebuilt by its parent, or when something it reads from
// ctx (a Provider, the theme) changes. It does not run for layouts that
// keep the same constraints, so moving it around is free. The child gets
// the same constraints and the LayoutBuilder takes the child's size.
//
// c.MaxW / c.MaxH may be geom.Inf (inside a scroll view or a Row); check
// c.HasBoundedWidth() before dividing by them. State below the builder is
// kept when a rebuild returns the same widget types in the same places,
// exactly as in Build; give the branches keys to keep state across them.
type LayoutBuilder struct {
	Builder func(ctx BuildContext, c geom.Constraints) Widget
}

// layoutBuilderElement owns a RenderLayoutBuilder and inflates its child
// from the render object's layout callback.
type layoutBuilderElement struct {
	renderElementBase
	child      Element
	needsBuild bool
	built      bool
	last       geom.Constraints
}

func (e *layoutBuilderElement) mount(parent Element) {
	e.mountBase(e, parent)
	ro := &render.RenderLayoutBuilder{}
	ro.OnLayout = e.layout
	e.ro = ro
	e.needsBuild = true // built at the first layout
}

func (e *layoutBuilderElement) update(w Widget) {
	e.widget = w
	e.scheduleBuild()
}

// rebuild runs when a dependency (DependOn from the builder) changed: the
// actual build waits for layout, where the constraints are known.
func (e *layoutBuilderElement) rebuild() {
	e.dirty = false
	e.scheduleBuild()
}

func (e *layoutBuilderElement) scheduleBuild() {
	e.needsBuild = true
	render.MarkNeedsLayout(e.ro)
}

// layout is RenderLayoutBuilder.OnLayout.
func (e *layoutBuilderElement) layout(c geom.Constraints) {
	if !e.active || (e.built && !e.needsBuild && c == e.last) {
		return
	}
	e.needsBuild, e.built, e.last = false, true, c
	var w Widget
	e.beginBuild()
	if b := e.widget.(LayoutBuilder).Builder; b != nil {
		w = b(e, c)
	}
	e.endBuild()
	e.child = updateChild(e, e.child, w)
	e.syncRenderChildren()
}

func (e *layoutBuilderElement) unmount() {
	if e.child != nil {
		e.child.unmount()
		e.child = nil
	}
	e.unmountBase()
}

func (e *layoutBuilderElement) visitChildren(fn func(Element)) {
	if e.child != nil {
		fn(e.child)
	}
}

func (e *layoutBuilderElement) syncRenderChildren() {
	render.SetChild(e.ro.(*render.RenderLayoutBuilder), renderObjectOf(e.child))
}
