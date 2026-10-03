// Package mvvm is a second way to drive the UI, next to StatefulWidget +
// SetState: keep the state in a view model made of observable values,
// and let widgets subscribe to exactly the values they show.
//
//	type CounterVM struct {
//	    mvvm.ViewModel
//	    Count *mvvm.Property[int]
//	}
//
//	func NewCounterVM() *CounterVM { return &CounterVM{Count: mvvm.NewProperty(0)} }
//	func (vm *CounterVM) Increment() { vm.Count.Update(func(n int) int { return n + 1 }) }
//
//	// the view
//	mvvm.Provide[*CounterVM]{Create: NewCounterVM, Child: CounterPage{}}
//
//	func (CounterPage) Build(ctx w.BuildContext) w.Widget {
//	    vm := mvvm.Use[*CounterVM](ctx)
//	    return w.Column{Children: []w.Widget{
//	        mvvm.Bind(vm.Count, func(ctx w.BuildContext, n int) w.Widget {
//	            return w.Text{Text: strconv.Itoa(n)} // the only widget that rebuilds
//	        }),
//	        Button{Label: "+1", OnPressed: vm.Increment},
//	    }}
//	}
//
// A change rebuilds only the bindings that watch it (Bind, Watch, Select,
// Observer), never the page around them, and each binding's subtree is
// updated only where the new widgets differ from the old ones.
//
// Observables are safe for concurrent use: a view model may update them
// from any goroutine (network callbacks, timers). Bindings coalesce the
// notifications and rebuild once, on the UI goroutine, before the next
// frame.
package mvvm

import (
	"sync"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Listenable is anything that notifies subscribers of changes.
type Listenable = widgets.Listenable

// Observable is a Listenable value: Property, Computed, or your own type.
type Observable[T any] interface {
	Listenable
	Get() T
}

// Notifier keeps a list of subscribers and calls them on Notify. Embed it
// (or ViewModel) in types that notify as a whole. The zero value is ready
// to use; a Notifier must not be copied after first use.
type Notifier struct {
	mu        sync.Mutex
	listeners []*listener
}

type listener struct{ fn func() }

// Subscribe registers fn, called after every Notify. The returned cancel
// removes it and may be called more than once.
func (n *Notifier) Subscribe(fn func()) (cancel func()) {
	l := &listener{fn: fn}
	n.mu.Lock()
	n.listeners = append(n.listeners, l)
	n.mu.Unlock()
	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, x := range n.listeners {
			if x == l {
				n.listeners = append(n.listeners[:i:i], n.listeners[i+1:]...)
				return
			}
		}
	}
}

// Notify calls every subscriber, in the order they subscribed, on the
// calling goroutine. Subscribers may subscribe or cancel while notified.
func (n *Notifier) Notify() {
	n.mu.Lock()
	ls := n.listeners
	n.mu.Unlock()
	for _, l := range ls {
		l.fn()
	}
}

// HasListeners reports whether anyone is subscribed.
func (n *Notifier) HasListeners() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.listeners) > 0
}

// Property is an observable value. Set notifies subscribers only when the
// value actually changes.
type Property[T comparable] struct {
	Notifier
	mu sync.RWMutex
	v  T
}

// NewProperty creates a property holding v.
func NewProperty[T comparable](v T) *Property[T] { return &Property[T]{v: v} }

// Get returns the current value.
func (p *Property[T]) Get() T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v
}

// Watch is Watch(ctx, p): the value, rebuilding the calling widget when it
// changes.
func (p *Property[T]) Watch(ctx widgets.BuildContext) T { return Watch[T](ctx, p) }

// Set stores v and notifies subscribers if it differs from the current
// value. Its signature fits event callbacks: OnChanged: vm.Enabled.Set.
func (p *Property[T]) Set(v T) { p.Update(func(T) T { return v }) }

// Update replaces the value with fn(current) atomically (concurrent
// Updates don't lose each other's changes) and notifies if it changed.
// fn must not use p itself.
func (p *Property[T]) Update(fn func(T) T) {
	p.mu.Lock()
	v := fn(p.v)
	if p.v == v {
		p.mu.Unlock()
		return
	}
	p.v = v
	p.mu.Unlock()
	p.Notify()
}

// Computed is a read-only value derived from other observables. It
// recomputes when one of its sources notifies, and notifies its own
// subscribers only when the result changes, so a binding on a Computed
// ignores source changes that don't affect it.
//
//	total := mvvm.NewComputed(func() int { return vm.Price.Get() * vm.Qty.Get() }, vm.Price, vm.Qty)
//
// Call Dispose when it's no longer needed (ViewModel.Own does it for you),
// or it keeps its sources' subscriptions.
type Computed[T comparable] struct {
	Notifier
	mu      sync.Mutex
	compute func() T
	v       T
	cancels []func()
}

// NewComputed evaluates compute now and again after every change of deps.
func NewComputed[T comparable](compute func() T, deps ...Listenable) *Computed[T] {
	c := &Computed[T]{compute: compute, v: compute()}
	for _, d := range deps {
		c.cancels = append(c.cancels, d.Subscribe(c.recompute))
	}
	return c
}

func (c *Computed[T]) recompute() {
	c.mu.Lock()
	v := c.compute()
	changed := v != c.v
	c.v = v
	c.mu.Unlock()
	if changed {
		c.Notify()
	}
}

// Get returns the last computed value.
func (c *Computed[T]) Get() T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

// Watch is Watch(ctx, c): the value, rebuilding the calling widget when it
// changes.
func (c *Computed[T]) Watch(ctx widgets.BuildContext) T { return Watch[T](ctx, c) }

// Dispose unsubscribes from the sources; the value freezes.
func (c *Computed[T]) Dispose() {
	c.mu.Lock()
	cancels := c.cancels
	c.cancels = nil
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// ViewModel is a convenient base for view models: embed it to get a
// Notifier (for changes that aren't a single property) and a place to
// park subscriptions and Computeds that must end with the view model.
// Provide calls Dispose when the view model leaves the tree.
type ViewModel struct {
	Notifier
	ownMu sync.Mutex
	owned []func()
}

// Own registers cleanup to run on Dispose, e.g. a cancel func returned by
// Subscribe. For values with a Dispose method use the Own function.
func (vm *ViewModel) Own(cleanup func()) {
	vm.ownMu.Lock()
	vm.owned = append(vm.owned, cleanup)
	vm.ownMu.Unlock()
}

// Dispose runs everything registered with Own, newest first.
func (vm *ViewModel) Dispose() {
	vm.ownMu.Lock()
	owned := vm.owned
	vm.owned = nil
	vm.ownMu.Unlock()
	for i := len(owned) - 1; i >= 0; i-- {
		owned[i]()
	}
}

// Own registers d (a Computed, a child view model) to be disposed with vm
// and returns it, so it can wrap a constructor:
//
//	vm.Total = mvvm.Own(&vm.ViewModel, mvvm.NewComputed(...))
func Own[D interface{ Dispose() }](vm *ViewModel, d D) D {
	vm.Own(d.Dispose)
	return d
}
