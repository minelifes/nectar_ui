// Package widgets is the declarative layer (Flutter's widgets library).
//
// Three trees exist at runtime:
//
//	Widget   – immutable description, cheap, rebuilt all the time (your structs)
//	Element  – long-lived instance of a widget at a location in the tree;
//	           diffs old vs new widgets and owns State
//	RenderObject – layout + paint (package render)
//
// You write widgets in one of three flavours, chosen by which interface the
// struct implements (no base type to embed, no CreateElement boilerplate):
//
//	StatelessWidget:    Build(ctx BuildContext) Widget
//	StatefulWidget:     CreateState() State
//	RenderObjectWidget: CreateRenderObject / UpdateRenderObject
//	                    (+ ChildWidget() or ChildWidgets() for children)
package widgets

import (
	"reflect"

	"github.com/minelifes/nectar_ui/ui/render"
)

// Widget is any value implementing one of the widget interfaces below.
type Widget = any

// StatelessWidget builds a subtree from its own fields only.
type StatelessWidget interface {
	Build(ctx BuildContext) Widget
}

// StatefulWidget owns a mutable State that survives rebuilds.
type StatefulWidget interface {
	CreateState() State
}

// State is the mutable half of a StatefulWidget. Embed StateBase.
//
// Optional hooks: InitState(), DidUpdateWidget(old Widget), Dispose().
type State interface {
	Build(ctx BuildContext) Widget
	stateBase() *StateBase
}

// RenderObjectWidget creates and configures a render object.
type RenderObjectWidget interface {
	CreateRenderObject(ctx BuildContext) render.RenderObject
	UpdateRenderObject(ctx BuildContext, ro render.RenderObject)
}

// SingleChildWidget is a RenderObjectWidget with (at most) one child. Its
// render object must embed render.SingleChild.
type SingleChildWidget interface {
	RenderObjectWidget
	ChildWidget() Widget
}

// MultiChildWidget is a RenderObjectWidget with a list of children. Its
// render object must embed render.MultiChild.
type MultiChildWidget interface {
	RenderObjectWidget
	ChildWidgets() []Widget
}

// InheritedWidget makes data available to its whole subtree via
// DependOn[T](ctx). Dependents rebuild when UpdateShouldNotify returns true.
type InheritedWidget interface {
	ChildWidget() Widget
	UpdateShouldNotify(old Widget) bool
}

// Keyed widgets keep their Element (and State) when children are reordered.
type Keyed interface {
	Key() any
}

func keyOf(w Widget) any {
	if k, ok := w.(Keyed); ok {
		return k.Key()
	}
	return nil
}

// sameWidget reports whether w is identical to old, so the subtree can be
// skipped entirely. True for the same pointer, or equal comparable values
// (structs without slices/maps/funcs). This is Go's version of Flutter's
// const widgets: hoisting or reusing a widget value avoids rebuilding it.
// It relies on widgets being immutable once built.
func sameWidget(old, w Widget) bool {
	a, b := reflect.ValueOf(old), reflect.ValueOf(w)
	if a.Type() != b.Type() {
		return false
	}
	if a.Kind() == reflect.Pointer {
		return a.Pointer() == b.Pointer()
	}
	return a.Comparable() && b.Comparable() && a.Equal(b)
}

// canUpdate reports whether an element built for old can be reused for w.
func canUpdate(old, w Widget) bool {
	return reflect.TypeOf(old) == reflect.TypeOf(w) && keyOf(old) == keyOf(w)
}

// --- State -----------------------------------------------------------------

// StateBase must be embedded in every State implementation.
type StateBase struct {
	element *statefulElement
}

func (s *StateBase) stateBase() *StateBase { return s }

// Widget returns the current widget configuration.
func (s *StateBase) Widget() Widget { return s.element.widget }

// Context returns the BuildContext of the state's element.
func (s *StateBase) Context() BuildContext { return s.element }

// Mounted reports whether the state is still in the tree.
func (s *StateBase) Mounted() bool { return s.element != nil && s.element.active }

// SetState runs fn and schedules a rebuild. Must be called on the UI
// goroutine; from other goroutines use Post.
func (s *StateBase) SetState(fn func()) {
	if fn != nil {
		fn()
	}
	if s.Mounted() {
		s.element.markNeedsBuild()
	}
}

// Post runs fn on the UI goroutine before the next frame, then rebuilds.
// Safe to call from any goroutine (timers, network callbacks, ...).
func (s *StateBase) Post(fn func()) {
	e := s.element
	if e == nil || e.owner == nil {
		return
	}
	e.owner.Post(func() {
		if s.Mounted() {
			s.SetState(fn)
		}
	})
}

// WidgetOf returns a State's widget as its concrete type.
func WidgetOf[W any](s State) W { return s.stateBase().Widget().(W) }

// Optional State lifecycle hooks.
type (
	initStater      interface{ InitState() }
	didUpdateWidget interface{ DidUpdateWidget(old Widget) }
	disposer        interface{ Dispose() }
)
