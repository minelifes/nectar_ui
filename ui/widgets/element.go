package widgets

import (
	"fmt"

	"github.com/minelifes/nectar_ui/ui/render"
)

// BuildContext is a handle to an element's location in the tree.
type BuildContext interface {
	Widget() Widget
	Owner() *BuildOwner
	Parent() BuildContext
	// RenderObject returns the nearest render object at or below this
	// element (useful for measuring after layout).
	RenderObject() render.RenderObject
}

// Element is a widget instantiated at a location in the tree.
type Element interface {
	BuildContext
	base() *elementBase
	mount(parent Element)
	update(w Widget)
	unmount()
	rebuild()
	visitChildren(fn func(Element))
}

type elementBase struct {
	self   Element
	widget Widget
	parent Element
	owner  *BuildOwner
	depth  int
	dirty  bool
	active bool
	// inherited elements this element depends on
	deps map[*inheritedElement]struct{}
}

func (e *elementBase) base() *elementBase { return e }
func (e *elementBase) Widget() Widget     { return e.widget }
func (e *elementBase) Owner() *BuildOwner { return e.owner }
func (e *elementBase) Parent() BuildContext {
	if e.parent == nil {
		return nil
	}
	return e.parent
}

func (e *elementBase) mountBase(self, parent Element) {
	e.self = self
	e.parent = parent
	if parent != nil {
		pb := parent.base()
		e.owner = pb.owner
		e.depth = pb.depth + 1
	}
	e.active = true
}

func (e *elementBase) unmountBase() {
	for ie := range e.deps {
		delete(ie.dependents, e.self)
	}
	e.deps = nil
	e.active = false
}

func (e *elementBase) markNeedsBuild() {
	if !e.active || e.dirty {
		return
	}
	e.dirty = true
	if e.owner != nil {
		e.owner.scheduleBuildFor(e.self)
	}
}

// inflate creates the right element type for w and mounts it.
func inflate(w Widget, parent Element) Element {
	var el Element
	switch w := w.(type) {
	case StatelessWidget:
		el = &statelessElement{}
	case StatefulWidget:
		el = &statefulElement{}
	case InheritedWidget:
		el = &inheritedElement{dependents: map[Element]struct{}{}}
	case MultiChildWidget:
		el = &multiChildElement{}
	case SingleChildWidget:
		el = &singleChildElement{}
	case RenderObjectWidget:
		el = &leafElement{}
	default:
		panic(fmt.Sprintf("widgets: %T is not a widget (implement Build, CreateState or CreateRenderObject)", w))
	}
	el.base().widget = w
	el.mount(parent)
	return el
}

// updateChild is the core of reconciliation: reuse, replace or remove.
func updateChild(parent Element, child Element, w Widget) Element {
	if w == nil {
		if child != nil {
			child.unmount()
		}
		return nil
	}
	if child != nil {
		if sameWidget(child.Widget(), w) {
			// Identical configuration: nothing to do. The subtree still
			// rebuilds on its own if it's dirty (SetState, inherited change).
			return child
		}
		if canUpdate(child.Widget(), w) {
			child.update(w)
			return child
		}
		child.unmount()
	}
	return inflate(w, parent)
}

// updateChildren reconciles a list, matching keyed widgets by key and the
// rest in order.
func updateChildren(parent Element, old []Element, widgets []Widget) []Element {
	out := make([]Element, 0, len(widgets))
	keyed := map[any]Element{}
	var unkeyed []Element
	for _, e := range old {
		if k := keyOf(e.Widget()); k != nil {
			keyed[k] = e
		} else {
			unkeyed = append(unkeyed, e)
		}
	}
	for _, w := range widgets {
		if w == nil {
			continue
		}
		var match Element
		if k := keyOf(w); k != nil {
			if e, ok := keyed[k]; ok && canUpdate(e.Widget(), w) {
				match = e
				delete(keyed, k)
			}
		} else if len(unkeyed) > 0 && canUpdate(unkeyed[0].Widget(), w) {
			match = unkeyed[0]
			unkeyed = unkeyed[1:]
		}
		out = append(out, updateChild(parent, match, w))
	}
	for _, e := range keyed {
		e.unmount()
	}
	for _, e := range unkeyed {
		e.unmount()
	}
	return out
}

// renderObjectOf returns the render object an element contributes to its
// nearest render-object ancestor.
func renderObjectOf(e Element) render.RenderObject {
	if e == nil {
		return nil
	}
	return e.RenderObject()
}

// ancestorRenderElement finds the nearest ancestor that owns a render object.
func ancestorRenderElement(e Element) renderElement {
	for p := e.base().parent; p != nil; p = p.base().parent {
		if re, ok := p.(renderElement); ok {
			return re
		}
	}
	return nil
}

// --- component elements (stateless / stateful / inherited) ----------------

type componentElement struct {
	elementBase
	child Element
}

func (e *componentElement) RenderObject() render.RenderObject { return renderObjectOf(e.child) }

func (e *componentElement) visitChildren(fn func(Element)) {
	if e.child != nil {
		fn(e.child)
	}
}

func (e *componentElement) unmountChild() {
	if e.child != nil {
		e.child.unmount()
		e.child = nil
	}
}

// performRebuild swaps in the newly built child and, if the render object
// under this component changed, tells the nearest render ancestor.
func (e *componentElement) performRebuild(built Widget) {
	e.dirty = false
	before := renderObjectOf(e.child)
	e.child = updateChild(e.self, e.child, built)
	if after := renderObjectOf(e.child); after != before {
		if re := ancestorRenderElement(e.self); re != nil {
			re.syncRenderChildren()
		}
	}
}

// statelessElement

type statelessElement struct{ componentElement }

func (e *statelessElement) mount(parent Element) {
	e.mountBase(e, parent)
	e.rebuild()
}

func (e *statelessElement) update(w Widget) {
	e.widget = w
	e.rebuild()
}

func (e *statelessElement) rebuild() {
	e.performRebuild(e.widget.(StatelessWidget).Build(e))
}

func (e *statelessElement) unmount() {
	e.unmountChild()
	e.unmountBase()
}

// statefulElement

type statefulElement struct {
	componentElement
	state State
}

func (e *statefulElement) mount(parent Element) {
	e.mountBase(e, parent)
	e.state = e.widget.(StatefulWidget).CreateState()
	e.state.stateBase().element = e
	if s, ok := e.state.(initStater); ok {
		s.InitState()
	}
	e.rebuild()
}

func (e *statefulElement) update(w Widget) {
	old := e.widget
	e.widget = w
	if s, ok := e.state.(didUpdateWidget); ok {
		s.DidUpdateWidget(old)
	}
	e.rebuild()
}

func (e *statefulElement) rebuild() { e.performRebuild(e.state.Build(e)) }

func (e *statefulElement) unmount() {
	e.unmountChild()
	if s, ok := e.state.(disposer); ok {
		s.Dispose()
	}
	e.unmountBase()
	e.state.stateBase().element = nil
}

// inheritedElement

type inheritedElement struct {
	componentElement
	dependents map[Element]struct{}
}

func (e *inheritedElement) mount(parent Element) {
	e.mountBase(e, parent)
	e.rebuild()
}

func (e *inheritedElement) update(w Widget) {
	old := e.widget
	e.widget = w
	if w.(InheritedWidget).UpdateShouldNotify(old) {
		for d := range e.dependents {
			d.base().markNeedsBuild()
		}
	}
	e.rebuild()
}

func (e *inheritedElement) rebuild() {
	e.performRebuild(e.widget.(InheritedWidget).ChildWidget())
}

func (e *inheritedElement) unmount() {
	e.unmountChild()
	e.unmountBase()
}

// DependOn finds the nearest ancestor InheritedWidget of type T and registers
// ctx to rebuild when it changes (Flutter's dependOnInheritedWidgetOfExactType).
func DependOn[T InheritedWidget](ctx BuildContext) (T, bool) {
	el, ie, w, ok := findInherited[T](ctx)
	if ok {
		ie.dependents[el] = struct{}{}
		b := el.base()
		if b.deps == nil {
			b.deps = map[*inheritedElement]struct{}{}
		}
		b.deps[ie] = struct{}{}
	}
	return w, ok
}

// Find looks up the nearest ancestor InheritedWidget of type T without
// subscribing to changes (Flutter's getInheritedWidgetOfExactType). Use it in
// event handlers or InitState, where a rebuild wouldn't help.
func Find[T InheritedWidget](ctx BuildContext) (T, bool) {
	_, _, w, ok := findInherited[T](ctx)
	return w, ok
}

func findInherited[T InheritedWidget](ctx BuildContext) (Element, *inheritedElement, T, bool) {
	var zero T
	el, ok := ctx.(Element)
	if !ok {
		return nil, nil, zero, false
	}
	for p := el.base().parent; p != nil; p = p.base().parent {
		if ie, ok := p.(*inheritedElement); ok {
			if w, ok := ie.widget.(T); ok {
				return el, ie, w, true
			}
		}
	}
	return nil, nil, zero, false
}

// --- render object elements -----------------------------------------------

type renderElement interface {
	Element
	syncRenderChildren()
}

type renderElementBase struct {
	elementBase
	ro render.RenderObject
}

func (e *renderElementBase) RenderObject() render.RenderObject { return e.ro }

func (e *renderElementBase) mountRender(self Element, parent Element) {
	e.mountBase(self, parent)
	e.ro = e.widget.(RenderObjectWidget).CreateRenderObject(self)
}

func (e *renderElementBase) updateRender(self Element, w Widget) {
	e.widget = w
	w.(RenderObjectWidget).UpdateRenderObject(self, e.ro)
}

// leafElement: render object without children (e.g. text).

type leafElement struct{ renderElementBase }

func (e *leafElement) mount(parent Element)        { e.mountRender(e, parent) }
func (e *leafElement) update(w Widget)             { e.updateRender(e, w) }
func (e *leafElement) rebuild()                    { e.dirty = false }
func (e *leafElement) unmount()                    { e.unmountBase() }
func (e *leafElement) visitChildren(func(Element)) {}
func (e *leafElement) syncRenderChildren()         {}

// singleChildElement

type singleChildElement struct {
	renderElementBase
	child Element
}

func (e *singleChildElement) mount(parent Element) {
	e.mountRender(e, parent)
	e.child = updateChild(e, nil, e.widget.(SingleChildWidget).ChildWidget())
	e.syncRenderChildren()
}

func (e *singleChildElement) update(w Widget) {
	e.updateRender(e, w)
	e.child = updateChild(e, e.child, w.(SingleChildWidget).ChildWidget())
	e.syncRenderChildren()
}

func (e *singleChildElement) rebuild() { e.dirty = false }

func (e *singleChildElement) unmount() {
	if e.child != nil {
		e.child.unmount()
		e.child = nil
	}
	e.unmountBase()
}

func (e *singleChildElement) visitChildren(fn func(Element)) {
	if e.child != nil {
		fn(e.child)
	}
}

func (e *singleChildElement) syncRenderChildren() {
	holder, ok := e.ro.(render.SingleChildHolder)
	if !ok {
		panic(fmt.Sprintf("widgets: %T has a child but its render object %T doesn't embed render.SingleChild", e.widget, e.ro))
	}
	render.SetChild(holder, renderObjectOf(e.child))
}

// multiChildElement

type multiChildElement struct {
	renderElementBase
	children []Element
}

func (e *multiChildElement) mount(parent Element) {
	e.mountRender(e, parent)
	e.children = updateChildren(e, nil, e.widget.(MultiChildWidget).ChildWidgets())
	e.syncRenderChildren()
}

func (e *multiChildElement) update(w Widget) {
	e.updateRender(e, w)
	e.children = updateChildren(e, e.children, w.(MultiChildWidget).ChildWidgets())
	e.syncRenderChildren()
}

func (e *multiChildElement) rebuild() { e.dirty = false }

func (e *multiChildElement) unmount() {
	for _, c := range e.children {
		c.unmount()
	}
	e.children = nil
	e.unmountBase()
}

func (e *multiChildElement) visitChildren(fn func(Element)) {
	for _, c := range e.children {
		fn(c)
	}
}

func (e *multiChildElement) syncRenderChildren() {
	holder, ok := e.ro.(render.MultiChildHolder)
	if !ok {
		panic(fmt.Sprintf("widgets: %T has children but its render object %T doesn't embed render.MultiChild", e.widget, e.ro))
	}
	ros := make([]render.RenderObject, 0, len(e.children))
	for _, c := range e.children {
		if ro := renderObjectOf(c); ro != nil {
			ros = append(ros, ro)
		}
	}
	render.SetChildren(holder, ros)
}
