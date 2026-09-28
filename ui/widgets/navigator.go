package widgets

import (
	"slices"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// ---------------------------------------------------------------------------
// Overlay: floating layers above the app (menus, tooltips, snackbars).

// OverlayEntry is one floating layer. Its Builder gets the full overlay
// area; use Stack + Positioned inside to place things.
type OverlayEntry struct {
	Builder func(ctx BuildContext) Widget
	overlay *OverlayState
}

// Remove takes the entry off its overlay.
func (e *OverlayEntry) Remove() {
	if e.overlay != nil {
		e.overlay.Remove(e)
	}
}

// MarkNeedsBuild rebuilds the entry.
func (e *OverlayEntry) MarkNeedsBuild() {
	if e.overlay != nil && e.overlay.Mounted() {
		e.overlay.SetState(nil)
	}
}

// Overlay hosts OverlayEntries above Child.
type Overlay struct{ Child Widget }

func (Overlay) CreateState() State { return &OverlayState{} }

// OverlayState is the mutable part of an Overlay; get it with OverlayOf.
type OverlayState struct {
	StateBase
	entries []*OverlayEntry
}

// Insert adds an entry on top.
func (s *OverlayState) Insert(e *OverlayEntry) {
	e.overlay = s
	s.SetState(func() { s.entries = append(s.entries, e) })
}

// Remove removes an entry.
func (s *OverlayState) Remove(e *OverlayEntry) {
	s.SetState(func() { s.entries = slices.DeleteFunc(s.entries, func(x *OverlayEntry) bool { return x == e }) })
	e.overlay = nil
}

func (s *OverlayState) Build(BuildContext) Widget {
	kids := []Widget{KeyedSubtree{ID: "base", Child: WidgetOf[Overlay](s).Child}}
	for _, e := range s.entries {
		kids = append(kids, KeyedSubtree{ID: e, Child: Builder{Builder: e.Builder}})
	}
	return overlayScope{state: s, child: Stack{Expand: true, Children: kids}}
}

type overlayScope struct {
	state *OverlayState
	child Widget
}

func (w overlayScope) ChildWidget() Widget                { return w.child }
func (w overlayScope) UpdateShouldNotify(old Widget) bool { return old.(overlayScope).state != w.state }

// OverlayOf returns the nearest Overlay (nil if none).
func OverlayOf(ctx BuildContext) *OverlayState {
	if sc, ok := Find[overlayScope](ctx); ok {
		return sc.state
	}
	return nil
}

// ---------------------------------------------------------------------------
// Navigator: a stack of routes (pages, dialogs, sheets).

// Route is one entry of a Navigator.
type Route struct {
	Builder func(ctx BuildContext) Widget
	// Opaque routes fully cover the ones below (pages). Non-opaque routes
	// (dialogs) show what's underneath through a Barrier.
	Opaque             bool
	Barrier            geom.Color
	BarrierDismissible bool
	Duration           time.Duration // transition; 0 = 200ms
	// Transition wraps the page for animation value t∈[0,1] (0 = hidden).
	// Nil fades in.
	Transition func(child Widget, t float32) Widget
	// OnPop is called with Pop's result once the route is gone.
	OnPop func(result any)

	anim    *AnimationController
	popping bool
	result  any
}

// Navigator manages a stack of routes, starting with Home.
type Navigator struct {
	Home Widget
}

func (Navigator) CreateState() State { return &NavigatorState{} }

// NavigatorState is the mutable part of a Navigator; get it with NavigatorOf.
type NavigatorState struct {
	StateBase
	routes []*Route
	node   *FocusNode
}

func (s *NavigatorState) InitState() {
	s.node = &FocusNode{CatchAll: true, SkipTraversal: true, OnKey: func(e KeyEvent) bool {
		if e.Key == KeyEscape {
			if top := s.top(); top != nil && top.BarrierDismissible && s.CanPop() {
				s.Pop(nil)
				return true
			}
		}
		return false
	}}
	home := &Route{Opaque: true, Builder: func(BuildContext) Widget { return WidgetOf[Navigator](s).Home }}
	home.anim = NewAnimationController(s, 0)
	home.anim.SetValue(1)
	s.routes = []*Route{home}
}

func (s *NavigatorState) top() *Route {
	for i := len(s.routes) - 1; i >= 0; i-- {
		if !s.routes[i].popping {
			return s.routes[i]
		}
	}
	return nil
}

// Push shows r on top.
func (s *NavigatorState) Push(r *Route) {
	d := r.Duration
	if d == 0 {
		d = 200 * time.Millisecond
	}
	r.anim = NewAnimationController(s, d)
	r.anim.Curve = Emphasized
	r.anim.OnStatus = func(st AnimationStatus) {
		if st == Dismissed && r.popping {
			s.SetState(func() { s.routes = slices.DeleteFunc(s.routes, func(x *Route) bool { return x == r }) })
			if r.OnPop != nil {
				r.OnPop(r.result)
			}
		}
	}
	s.SetState(func() { s.routes = append(s.routes, r) })
	r.anim.Forward()
}

// CanPop reports whether there's a route above Home.
func (s *NavigatorState) CanPop() bool {
	n := 0
	for _, r := range s.routes {
		if !r.popping {
			n++
		}
	}
	return n > 1
}

// Pop removes the top route, passing result to its OnPop.
func (s *NavigatorState) Pop(result any) {
	r := s.top()
	if r == nil || !s.CanPop() {
		return
	}
	r.popping, r.result = true, result
	r.anim.Curve = EmphasizedAccelerate
	r.anim.Reverse()
	s.SetState(nil)
}

func (s *NavigatorState) Build(BuildContext) Widget {
	// Routes below the top-most fully visible opaque route are hidden.
	hideBelow := 0
	for i := len(s.routes) - 1; i >= 0; i-- {
		r := s.routes[i]
		if r.Opaque && !r.popping && r.anim.Value() >= 1 {
			hideBelow = i
			break
		}
	}
	var kids []Widget
	for i, r := range s.routes {
		t := r.anim.Value()
		var page Widget = Builder{Builder: r.Builder}
		if r.Transition != nil {
			page = r.Transition(page, t)
		} else if t < 1 {
			page = Opacity{Opacity: t, Child: page}
		}
		layer := []Widget{}
		if !r.Opaque {
			barrier := r.Barrier
			barrier.A *= t
			rr := r
			layer = append(layer, PositionedFill(GestureDetector{
				OnTap: func() {
					if rr.BarrierDismissible && s.top() == rr {
						s.Pop(nil)
					}
				},
				Child: AbsorbPointer{Child: DecoratedBox{Color: barrier}},
			}))
		}
		layer = append(layer, PositionedFill(page))
		var w Widget = Stack{Expand: true, Children: layer}
		if i < hideBelow {
			w = Opacity{Opacity: 0, Child: IgnorePointer{Ignoring: true, Child: w}}
		} else if r.popping {
			w = IgnorePointer{Ignoring: true, Child: w}
		}
		kids = append(kids, KeyedSubtree{ID: r, Child: routeScope{index: i, route: r, child: w}})
	}
	return Focus{Node: s.node, Child: navigatorScope{state: s, child: Stack{Expand: true, Children: kids}}}
}

type navigatorScope struct {
	state *NavigatorState
	child Widget
}

func (w navigatorScope) ChildWidget() Widget { return w.child }
func (w navigatorScope) UpdateShouldNotify(old Widget) bool {
	return old.(navigatorScope).state != w.state
}

// NavigatorOf returns the nearest Navigator (nil if none).
func NavigatorOf(ctx BuildContext) *NavigatorState {
	if sc, ok := Find[navigatorScope](ctx); ok {
		return sc.state
	}
	return nil
}

// routeScope tells widgets inside a route which route they belong to.
type routeScope struct {
	index int
	route *Route
	child Widget
}

func (w routeScope) ChildWidget() Widget { return w.child }
func (w routeScope) UpdateShouldNotify(old Widget) bool {
	o := old.(routeScope)
	return o.index != w.index || o.route != w.route
}

// CanPopRoute reports whether ctx's own route sits above the first one,
// i.e. whether a page should show a back button.
func CanPopRoute(ctx BuildContext) bool {
	if sc, ok := DependOn[routeScope](ctx); ok {
		return sc.index > 0
	}
	return false
}
