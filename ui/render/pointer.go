package render

import "github.com/minelifes/nectar_ui/ui/geom"

// PointerKind is the type of a pointer event.
type PointerKind uint8

const (
	PointerDown   PointerKind = iota // button pressed / finger touched
	PointerMove                      // moved while pressed (sent to the targets hit at Down)
	PointerUp                        // released
	PointerCancel                    // gesture aborted by the system
	PointerHover                     // moved with no button pressed
	PointerEnter                     // pointer entered this target's bounds
	PointerExit                      // pointer left this target's bounds
	PointerScroll                    // wheel / trackpad scroll
)

func (k PointerKind) String() string {
	return [...]string{"Down", "Move", "Up", "Cancel", "Hover", "Enter", "Exit", "Scroll"}[k]
}

// Mouse buttons (same numbering as gpucontext.Button).
const (
	ButtonPrimary   = 0
	ButtonMiddle    = 1
	ButtonSecondary = 2
)

// PointerEvent is delivered to render objects implementing PointerHandler.
type PointerEvent struct {
	Kind     PointerKind
	ID       int         // stable from Down to Up for one finger / the mouse
	Position geom.Offset // window coordinates (logical pixels)
	Local    geom.Offset // position relative to the receiving render object
	Delta    geom.Offset // movement since the previous event of this pointer
	Scroll   geom.Offset // scroll amount in pixels (PointerScroll only)
	Button   int         // button that changed (Down/Up), -1 otherwise
	Touch    bool        // true for touch input
	// Mods are the keyboard modifiers held (widgets.Modifiers bits:
	// Shift 1, Control 2, Alt 4, Super 8).
	Mods uint8

	arena *arena
}

// Claim asks to win this pointer's gesture. The first claimer wins; handlers
// receive events deepest-first, so the innermost detector gets first pick.
// Returns true if ro now owns the pointer (or already did).
func (e PointerEvent) Claim(ro RenderObject) bool {
	if e.arena == nil {
		return true
	}
	if e.arena.winner == nil {
		e.arena.winner = ro
	}
	return e.arena.winner == ro
}

// Winner returns the render object that claimed this pointer, if any.
func (e PointerEvent) Winner() RenderObject {
	if e.arena == nil {
		return nil
	}
	return e.arena.winner
}

// LostTo reports whether some other render object claimed the pointer.
func (e PointerEvent) LostTo(ro RenderObject) bool {
	w := e.Winner()
	return w != nil && w != ro
}

type arena struct{ winner RenderObject }

// PointerHandler receives pointer events routed to it by hit testing.
type PointerHandler interface {
	HandlePointer(e PointerEvent)
}

// Cursor is a mouse cursor shape.
type Cursor uint8

const (
	CursorDefault Cursor = iota
	CursorPointer        // hand, for clickable things
	CursorText
	CursorCrosshair
	CursorMove
	CursorResizeNS
	CursorResizeEW
	CursorNotAllowed
)

// CursorProvider is implemented by render objects that pick the cursor
// while hovered (the deepest one with a non-default cursor wins).
type CursorProvider interface {
	Cursor() Cursor
}

// HitEntry is one render object under the pointer.
type HitEntry struct {
	Target RenderObject
	Origin geom.Offset // target's top-left in window coordinates
}

// HitTestAt returns the render objects under pos (window coordinates),
// deepest first. A node is included if it implements HitTester and reports a
// hit, or if one of its descendants was hit. Children that overflow their
// parent's bounds are not hittable (same as Flutter).
func HitTestAt(root RenderObject, pos geom.Offset) []HitEntry {
	if root == nil {
		return nil
	}
	return hitTest(root, geom.Offset{}, pos, nil)
}

// PointerIgnorer is implemented by render objects that hide their subtree
// from hit testing (IgnorePointer).
type PointerIgnorer interface{ IgnoresPointer() bool }

func hitTest(ro RenderObject, parentOrigin, pos geom.Offset, out []HitEntry) []HitEntry {
	if ig, ok := ro.(PointerIgnorer); ok && ig.IgnoresPointer() {
		return out
	}
	b := ro.Base()
	origin := parentOrigin.Add(b.offset)
	local := pos.Sub(origin)
	if !geom.RectFrom(geom.Offset{}, b.size).Contains(local) {
		return out
	}
	n := len(out)
	// Children painted last are on top, so test them first; stop at the
	// first child that reports a hit.
	var kids []RenderObject
	ro.VisitChildren(func(c RenderObject) { kids = append(kids, c) })
	for i := len(kids) - 1; i >= 0; i-- {
		out = hitTest(kids[i], origin, pos, out)
		if len(out) > n {
			break
		}
	}
	childHit := len(out) > n
	selfHit := false
	if ht, ok := ro.(HitTester); ok {
		selfHit = ht.HitTestSelf(local)
	}
	if childHit || selfHit {
		out = append(out, HitEntry{Target: ro, Origin: origin})
	}
	return out
}

// GlobalOrigin returns ro's top-left corner in window coordinates.
func GlobalOrigin(ro RenderObject) geom.Offset {
	var o geom.Offset
	for n := ro; n != nil; n = n.Base().parent {
		o = o.Add(n.Base().offset)
	}
	return o
}

// PointerDispatcher routes raw pointer input to render objects: hit tests
// on Down, captures the pointer until Up (so drags keep working outside the
// widget), and tracks hover for Enter/Exit and the mouse cursor.
type PointerDispatcher struct {
	captured map[int]*capture
	hovered  []RenderObject
	last     map[int]geom.Offset
	cursor   Cursor
}

type capture struct {
	targets []RenderObject
	arena   *arena
}

// NewPointerDispatcher creates a dispatcher.
func NewPointerDispatcher() *PointerDispatcher {
	return &PointerDispatcher{captured: map[int]*capture{}, last: map[int]geom.Offset{}}
}

// Cursor returns the cursor requested by the hovered render objects.
func (d *PointerDispatcher) Cursor() Cursor { return d.cursor }

// Dispatch delivers one event. Only Position, Kind, ID, Button, Scroll and
// Touch need to be set; Local, Delta and the arena are filled in here.
func (d *PointerDispatcher) Dispatch(root RenderObject, e PointerEvent) {
	if prev, ok := d.last[e.ID]; ok && e.Delta == (geom.Offset{}) {
		e.Delta = e.Position.Sub(prev)
	}
	d.last[e.ID] = e.Position

	switch e.Kind {
	case PointerDown:
		hits := HitTestAt(root, e.Position)
		c := &capture{arena: &arena{}}
		for _, h := range hits {
			if _, ok := h.Target.(PointerHandler); ok {
				c.targets = append(c.targets, h.Target)
			}
		}
		d.captured[e.ID] = c
		d.send(c.targets, e, c.arena)

	case PointerMove:
		if c, ok := d.captured[e.ID]; ok {
			d.send(c.targets, e, c.arena)
		}
		d.updateHover(root, e)

	case PointerUp, PointerCancel:
		if c, ok := d.captured[e.ID]; ok {
			d.send(c.targets, e, c.arena)
			delete(d.captured, e.ID)
		}
		if e.Touch {
			delete(d.last, e.ID)
			d.updateHover(root, PointerEvent{Kind: PointerExit, ID: e.ID, Position: geom.Offset{X: -1e9, Y: -1e9}})
		} else {
			d.updateHover(root, e)
		}

	case PointerHover:
		e.Kind = PointerHover
		d.send(handlers(HitTestAt(root, e.Position)), e, nil)
		d.updateHover(root, e)

	case PointerExit: // pointer left the window
		d.updateHover(root, PointerEvent{Kind: PointerExit, ID: e.ID, Position: geom.Offset{X: -1e9, Y: -1e9}})
		delete(d.last, e.ID)

	case PointerScroll:
		// Each scroll event gets its own arena: the innermost scrollable
		// that can move claims it, outer ones see LostTo and ignore it.
		d.send(handlers(HitTestAt(root, e.Position)), e, &arena{})
	}
}

func handlers(hits []HitEntry) []RenderObject {
	var out []RenderObject
	for _, h := range hits {
		if _, ok := h.Target.(PointerHandler); ok {
			out = append(out, h.Target)
		}
	}
	return out
}

// send delivers e to targets (deepest first) with per-target local coords.
func (d *PointerDispatcher) send(targets []RenderObject, e PointerEvent, a *arena) {
	e.arena = a
	for _, t := range targets {
		if t.Base().owner == nil { // removed from the tree meanwhile
			continue
		}
		ev := e
		ev.Local = e.Position.Sub(GlobalOrigin(t))
		t.(PointerHandler).HandlePointer(ev)
	}
}

// updateHover sends Enter/Exit by diffing the hovered set and recomputes
// the cursor.
func (d *PointerDispatcher) updateHover(root RenderObject, e PointerEvent) {
	var now []RenderObject
	d.cursor = CursorDefault
	cursorSet := false
	for _, h := range HitTestAt(root, e.Position) {
		if _, ok := h.Target.(PointerHandler); ok {
			now = append(now, h.Target)
		}
		if cp, ok := h.Target.(CursorProvider); ok && !cursorSet {
			if c := cp.Cursor(); c != CursorDefault {
				d.cursor, cursorSet = c, true
			}
		}
	}
	in := func(list []RenderObject, ro RenderObject) bool {
		for _, x := range list {
			if x == ro {
				return true
			}
		}
		return false
	}
	for _, old := range d.hovered {
		if !in(now, old) {
			ev := e
			ev.Kind = PointerExit
			d.send([]RenderObject{old}, ev, nil)
		}
	}
	for _, n := range now {
		if !in(d.hovered, n) {
			ev := e
			ev.Kind = PointerEnter
			d.send([]RenderObject{n}, ev, nil)
		}
	}
	d.hovered = now
}
