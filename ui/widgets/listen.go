package widgets

import (
	"fmt"
	"reflect"
)

// Listenable is anything a widget can watch for changes: view models and
// observable properties (package mvvm), controllers, services. Subscribe
// registers fn to be called after every change and returns a function that
// removes it. fn may be called from any goroutine.
type Listenable interface {
	Subscribe(fn func()) (cancel func())
}

// Listen makes the element of ctx rebuild whenever l notifies. Call it from
// Build: only that element rebuilds, not its parent, and its children only
// where the widgets they get actually changed. So wrapping the part that
// shows a value in its own Builder keeps each change as local as possible:
//
//	widgets.Builder{Builder: func(ctx widgets.BuildContext) widgets.Widget {
//	    widgets.Listen(ctx, vm.Count)
//	    return widgets.Text{Text: strconv.Itoa(vm.Count.Get())}
//	}}
//
// The subscription lasts as long as each build of the element calls Listen
// for l again: it's cancelled after a build that no longer does, and when
// the element leaves the tree. Changes are coalesced: any number of
// notifications before the next frame cause one rebuild, on the UI
// goroutine, so l may notify from any goroutine. l must be comparable
// (typically a pointer).
func Listen(ctx BuildContext, l Listenable) {
	if l == nil {
		return
	}
	el, ok := ctx.(Element)
	if !ok {
		return
	}
	if t := reflect.TypeOf(l); !t.Comparable() {
		panic(fmt.Sprintf("widgets: Listen needs a comparable Listenable (e.g. a pointer), got %T", l))
	}
	e := el.base()
	if !e.active {
		return
	}
	if e.listens == nil {
		e.listens = map[Listenable]*listenSub{}
	}
	if sub, ok := e.listens[l]; ok {
		sub.gen = e.buildGen
		return
	}
	e.listens[l] = &listenSub{gen: e.buildGen, cancel: l.Subscribe(e.listenWaker())}
}

// listenSub is one Listen subscription of an element.
type listenSub struct {
	cancel func()
	gen    uint32 // build that last asked for it
}

// listenWaker returns the callback shared by the element's subscriptions:
// the first change since the last rebuild posts one markNeedsBuild to the
// UI goroutine, later ones are folded into it.
func (e *elementBase) listenWaker() func() {
	if e.wake != nil {
		return e.wake
	}
	owner, pending := e.owner, &e.listenPending
	e.wake = func() {
		if owner == nil || !pending.CompareAndSwap(false, true) {
			return
		}
		owner.Post(func() {
			pending.Store(false)
			e.markNeedsBuild()
		})
	}
	return e.wake
}

// beginBuild starts a build of the element: Listen calls made by it are
// counted as current.
func (e *elementBase) beginBuild() { e.buildGen++ }

// endBuild cancels the subscriptions the build didn't renew.
func (e *elementBase) endBuild() {
	for l, sub := range e.listens {
		if sub.gen != e.buildGen {
			sub.cancel()
			delete(e.listens, l)
		}
	}
}

// cancelListens drops every subscription (the element left the tree).
func (e *elementBase) cancelListens() {
	for _, sub := range e.listens {
		sub.cancel()
	}
	e.listens = nil
}
