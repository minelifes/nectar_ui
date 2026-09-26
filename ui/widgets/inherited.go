package widgets

import "fmt"

// Provider is a ready-made InheritedWidget that exposes a single value of
// type T to its subtree. Read it with Of[T] (rebuilds on change) or
// Peek[T] (no subscription).
//
//	widgets.Provider[Theme]{Value: dark, Child: app}
//	theme := widgets.MustOf[Theme](ctx)
//
// Dependents rebuild only when the new Value != old Value, so T must be
// comparable. For a mutable model, provide a pointer and replace it (or
// wrap it in a new value) when it changes. Providers of different T don't
// interfere, and an inner Provider[T] shadows an outer one.
type Provider[T comparable] struct {
	Value T
	Child Widget
}

func (p Provider[T]) ChildWidget() Widget { return p.Child }

func (p Provider[T]) UpdateShouldNotify(old Widget) bool {
	return old.(Provider[T]).Value != p.Value
}

// Of returns the nearest Provider[T] value and makes ctx rebuild when it
// changes.
func Of[T comparable](ctx BuildContext) (T, bool) {
	p, ok := DependOn[Provider[T]](ctx)
	return p.Value, ok
}

// MustOf is Of that panics with a clear message when no provider exists.
func MustOf[T comparable](ctx BuildContext) T {
	v, ok := Of[T](ctx)
	if !ok {
		var zero T
		panic("widgets: no Provider[" + typeName(zero) + "] above this widget")
	}
	return v
}

// Peek returns the nearest Provider[T] value without subscribing.
func Peek[T comparable](ctx BuildContext) (T, bool) {
	p, ok := Find[Provider[T]](ctx)
	return p.Value, ok
}

func typeName(v any) string { return fmt.Sprintf("%T", v) }
