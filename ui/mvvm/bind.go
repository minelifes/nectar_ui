package mvvm

import (
	"fmt"
	"sync/atomic"

	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// Watch returns o's value and makes the widget whose Build calls it rebuild
// when o changes (see widgets.Listen). Only that widget rebuilds; to keep
// the rebuild small, call it from a small widget or use Bind.
//
// Property, Computed and List have a Watch method that does the same
// (vm.Todos.Watch(ctx)); some editors can't infer T for this function
// from those types, although the compiler can.
func Watch[T any](ctx w.BuildContext, o Observable[T]) T {
	w.Listen(ctx, o)
	return o.Get()
}

// Binding rebuilds Builder with the current value of Source whenever it
// changes. Make one with Bind.
type Binding[T any] struct {
	Source  Observable[T]
	Builder func(ctx w.BuildContext, v T) w.Widget
}

func (b Binding[T]) Build(ctx w.BuildContext) w.Widget {
	return b.Builder(ctx, Watch[T](ctx, b.Source))
}

// Bind shows o through build and rebuilds only that part when o changes:
//
//	mvvm.Bind(vm.Name, func(ctx w.BuildContext, name string) w.Widget {
//	    return w.Text{Text: "Hello, " + name}
//	})
func Bind[T any](o Observable[T], build func(ctx w.BuildContext, v T) w.Widget) w.Widget {
	return Binding[T]{Source: o, Builder: build}
}

// Observer rebuilds Builder whenever any of Sources notifies. Inside
// Builder, Watch works too, so Sources can be left empty.
type Observer struct {
	Sources []Listenable
	Builder func(ctx w.BuildContext) w.Widget
}

func (o Observer) Build(ctx w.BuildContext) w.Widget {
	for _, s := range o.Sources {
		w.Listen(ctx, s)
	}
	return o.Builder(ctx)
}

// Selector shows one derived value of Model and rebuilds only when that
// value changes, however often Model notifies. Use it to bind part of a
// view model that notifies as a whole (ViewModel.Notify). Model must be
// comparable (typically a pointer). Make one with Select.
type Selector[M Listenable, K comparable] struct {
	Model   M
	Select  func(M) K
	Builder func(ctx w.BuildContext, v K) w.Widget
}

// Select binds the part of m picked by sel:
//
//	mvvm.Select(vm, func(vm *CartVM) int { return len(vm.Items) },
//	    func(ctx w.BuildContext, n int) w.Widget { return Badge{Count: n} })
func Select[M Listenable, K comparable](m M, sel func(M) K, build func(ctx w.BuildContext, v K) w.Widget) w.Widget {
	return Selector[M, K]{Model: m, Select: sel, Builder: build}
}

func (Selector[M, K]) CreateState() w.State { return &selectorState[M, K]{} }

type selectorState[M Listenable, K comparable] struct {
	w.StateBase
	model   M
	cancel  func()
	last    K
	pending atomic.Bool
}

func (s *selectorState[M, K]) InitState() { s.subscribe() }

func (s *selectorState[M, K]) DidUpdateWidget(w.Widget) {
	if any(w.WidgetOf[Selector[M, K]](s).Model) != any(s.model) {
		s.cancel()
		s.subscribe()
	}
}

func (s *selectorState[M, K]) Dispose() { s.cancel() }

func (s *selectorState[M, K]) subscribe() {
	s.model = w.WidgetOf[Selector[M, K]](s).Model
	owner := s.Context().Owner()
	s.cancel = s.model.Subscribe(func() {
		if owner == nil || !s.pending.CompareAndSwap(false, true) {
			return
		}
		owner.Post(func() {
			s.pending.Store(false)
			if !s.Mounted() {
				return
			}
			sel := w.WidgetOf[Selector[M, K]](s)
			if sel.Select(sel.Model) != s.last {
				s.SetState(nil)
			}
		})
	})
}

func (s *selectorState[M, K]) Build(ctx w.BuildContext) w.Widget {
	sel := w.WidgetOf[Selector[M, K]](s)
	s.last = sel.Select(sel.Model)
	return sel.Builder(ctx, s.last)
}

// Provide creates a view model when it enters the tree, makes it available
// to its subtree through Use, and disposes it (if it has a Dispose method,
// as ViewModel does) when it leaves. The view model lives as long as the
// Provide stays in place: rebuilding the parent keeps it.
//
//	mvvm.Provide[*TodoVM]{Create: NewTodoVM, Child: TodoPage{}}
//
// To pass in a view model owned elsewhere, use widgets.Provider[VM] (Use
// finds it too).
type Provide[VM comparable] struct {
	Create func() VM
	Child  w.Widget
}

func (Provide[VM]) CreateState() w.State { return &provideState[VM]{} }

type provideState[VM comparable] struct {
	w.StateBase
	vm VM
}

func (s *provideState[VM]) InitState() { s.vm = w.WidgetOf[Provide[VM]](s).Create() }

func (s *provideState[VM]) Dispose() {
	if d, ok := any(s.vm).(interface{ Dispose() }); ok {
		d.Dispose()
	}
}

func (s *provideState[VM]) Build(w.BuildContext) w.Widget {
	return w.Provider[VM]{Value: s.vm, Child: w.WidgetOf[Provide[VM]](s).Child}
}

// Use returns the nearest view model of type VM above ctx (from Provide
// or widgets.Provider). It panics if there is none. Use doesn't subscribe
// to the view model's changes: bind the values you show with Bind, Watch,
// Select or Observer.
func Use[VM comparable](ctx w.BuildContext) VM {
	vm, ok := w.Peek[VM](ctx)
	if !ok {
		var zero VM
		panic("mvvm: no Provide or Provider of " + typeName(zero) + " above this widget")
	}
	return vm
}

func typeName(v any) string { return fmt.Sprintf("%T", v) }
