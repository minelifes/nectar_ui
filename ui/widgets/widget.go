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
	"context"
	"fmt"
	"reflect"
	"sync/atomic"

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
// Optional hooks, in the order they run:
//
//   - InitState(): once, when the state is created and enters the tree,
//     before its first Build. Never again for this state, however often it
//     rebuilds.
//   - DidUpdateWidget(old Widget): when the parent rebuilds it with new
//     fields.
//   - Reassemble(): on hot reload (see Root.Reassemble).
//   - OnDestroy() (or Dispose(), the same hook under its Flutter name; a
//     state may have both): once, when it leaves the tree for good. Then
//     everything registered with Own and Listen is released, newest first,
//     and the state is detached, so nothing the tree holds keeps it alive.
//
// Free resources in OnDestroy, or register them with Own as you create them
// so they can't be forgotten:
//
//	func (s *clockState) InitState() {
//	    t := time.NewTicker(time.Second)
//	    s.Own(t)                                  // t.Stop() on destroy
//	    s.Listen(settings.Changed, s.reload)      // unsubscribed on destroy
//	    ctx, cancel := context.WithCancel(context.Background())
//	    s.Own(cancel)                             // cancels in-flight work
//	    go s.tick(ctx, t.C)
//	}
type State interface {
	Build(ctx BuildContext) Widget
	stateBase() *StateBase
}

// RenderObjectWidget creates and configures a render object.
type RenderObjectWidget interface {
	CreateRenderObject(ctx BuildContext) render.RenderObject
	UpdateRenderObject(ctx BuildContext, ro render.RenderObject)
}

// MarksOwnPaint is implemented by RenderObjectWidgets whose
// UpdateRenderObject calls render.MarkNeedsPaint or MarkNeedsLayout itself
// whenever the update changes what the render object draws. Without it the
// engine plays safe and invalidates the repaint boundaries around the
// render object after every update, so a rebuild that changes only, say, a
// callback still repaints the whole boundary (a scroll view's content).
// The method is a marker and is never called.
type MarksOwnPaint interface {
	MarksOwnPaint()
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
// skipped entirely. This is Go's version of Flutter's const widgets:
// hoisting or reusing a widget value avoids rebuilding it, and so does
// building an equal one. It relies on widgets being immutable once built.
//
// Values are compared structurally: pointers, channels by identity; funcs
// and maps are never equal (unless both nil), since a closure may capture
// new state; slices element by element (so a Row whose children didn't
// change is skipped too). A slice that shares its backing array with the
// old one counts as changed: it may have been modified in place. The
// comparison gives up (reports a change) after sameWidgetBudget values, so
// comparing a huge unchanged tree never costs more than updating it.
func sameWidget(old, w Widget) bool {
	a, b := reflect.ValueOf(old), reflect.ValueOf(w)
	if a.Type() != b.Type() {
		return false
	}
	budget := sameWidgetBudget
	return sameValue(a, b, &budget)
}

// sameWidgetBudget caps how many values one sameWidget call compares.
const sameWidgetBudget = 512

// sameValue compares a and b, which have the same type.
func sameValue(a, b reflect.Value, budget *int) bool {
	if *budget--; *budget < 0 {
		return false
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Chan:
		return a.Pointer() == b.Pointer()
	case reflect.Func, reflect.Map:
		return a.IsNil() && b.IsNil()
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() && b.IsNil()
		}
		ea, eb := a.Elem(), b.Elem()
		return ea.Type() == eb.Type() && sameValue(ea, eb, budget)
	case reflect.Struct:
		for i := range a.NumField() {
			if !sameValue(a.Field(i), b.Field(i), budget) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.Len() != b.Len() {
			return false
		}
		if a.Len() == 0 {
			return true
		}
		if a.Pointer() == b.Pointer() {
			return false // same backing array: may have been edited in place
		}
		fallthrough
	case reflect.Array:
		for i := range a.Len() {
			if !sameValue(a.Index(i), b.Index(i), budget) {
				return false
			}
		}
		return true
	default:
		return a.Equal(b)
	}
}

// canUpdate reports whether an element built for old can be reused for w.
func canUpdate(old, w Widget) bool {
	return reflect.TypeOf(old) == reflect.TypeOf(w) && keyOf(old) == keyOf(w)
}

// --- State -----------------------------------------------------------------

// StateBase must be embedded in every State implementation.
type StateBase struct {
	element   *statefulElement
	cleanups  []func()
	destroyed bool
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

// Own registers v to be released when the state is destroyed (after
// OnDestroy / Dispose), so subscriptions, timers and background work end
// with the widget. Releases run newest first. v may be:
//
//   - a func() or func() error, e.g. the cancel func from Subscribe or a
//     context.CancelFunc: it's called;
//   - a value with Close() error / Close(), Dispose(), Cancel(),
//     Stop() / Stop() bool (time.Timer, time.Ticker) or Unsubscribe():
//     that method is called.
//
// Own returns v. Owning something after the state was destroyed releases
// it right away. Call it on the UI goroutine (InitState, Build, handlers).
func (s *StateBase) Own(v any) any {
	release := releaser(v)
	if s.destroyed {
		release()
		return v
	}
	s.cleanups = append(s.cleanups, release)
	return v
}

// Listen calls fn on the UI goroutine after every change of l, until the
// state is destroyed; then it unsubscribes. Unlike the package-level
// Listen it doesn't rebuild by itself: call SetState in fn if the change
// should show (or use widgets.Listen in Build). Changes that arrive while
// the previous fn is still pending are folded into one call.
func (s *StateBase) Listen(l Listenable, fn func()) {
	if l == nil || fn == nil {
		return
	}
	var pending atomic.Bool
	var gone atomic.Bool
	cancel := l.Subscribe(func() {
		e := s.element
		if gone.Load() || e == nil || e.owner == nil || !pending.CompareAndSwap(false, true) {
			return
		}
		e.owner.Post(func() {
			pending.Store(false)
			if !gone.Load() && s.Mounted() {
				fn()
			}
		})
	})
	s.Own(func() { gone.Store(true); cancel() })
}

// Destroyed reports whether the state has left the tree for good
// (OnDestroy has run). Background work can check it before posting.
func (s *StateBase) Destroyed() bool { return s.destroyed }

// destroy releases everything owned, newest first. A panicking release
// doesn't stop the others; the first panic is re-raised after.
func (s *StateBase) destroy() {
	s.destroyed = true
	cleanups := s.cleanups
	s.cleanups = nil
	var first any
	for i := len(cleanups) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil && first == nil {
					first = r
				}
			}()
			cleanups[i]()
		}()
	}
	if first != nil {
		panic(first)
	}
}

func releaser(v any) func() {
	switch x := v.(type) {
	case nil:
		return func() {}
	case func():
		return x
	case context.CancelFunc:
		return x
	case func() error:
		return func() { _ = x() }
	case interface{ Close() error }:
		return func() { _ = x.Close() }
	case interface{ Close() }:
		return x.Close
	case interface{ Dispose() }:
		return x.Dispose
	case interface{ Cancel() }:
		return x.Cancel
	case interface{ Stop() bool }:
		return func() { x.Stop() }
	case interface{ Stop() }:
		return x.Stop
	case interface{ Unsubscribe() }:
		return x.Unsubscribe
	}
	panic(fmt.Sprintf("widgets: Own(%T): want a func() or a value with Close, Dispose, Cancel, Stop or Unsubscribe", v))
}

// WidgetOf returns a State's widget as its concrete type.
func WidgetOf[W any](s State) W { return s.stateBase().Widget().(W) }

// Optional State lifecycle hooks.
type (
	initStater      interface{ InitState() }
	didUpdateWidget interface{ DidUpdateWidget(old Widget) }
	disposer        interface{ Dispose() }
	destroyer       interface{ OnDestroy() }
	reassembler     interface{ Reassemble() }
)
