package render

import (
	"math"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// TouchSlop is how far (logical px) a pointer may move before a tap turns
// into a drag.
const TouchSlop float32 = 8

// --- RenderPointerListener -------------------------------------------------

// RenderPointerListener forwards raw pointer events to callbacks.
type RenderPointerListener struct {
	Box
	SingleChild
	OnEvent func(e PointerEvent)
}

func (r *RenderPointerListener) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderPointerListener) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderPointerListener) HitTestSelf(geom.Offset) bool           { return true }
func (r *RenderPointerListener) HandlePointer(e PointerEvent) {
	if r.OnEvent != nil {
		r.OnEvent(e)
	}
}

// --- RenderMouseRegion -----------------------------------------------------

// RenderMouseRegion reports hover enter/exit/move and sets the cursor.
type RenderMouseRegion struct {
	Box
	SingleChild
	CursorShape Cursor
	OnEnter     func(e PointerEvent)
	OnExit      func(e PointerEvent)
	OnHover     func(e PointerEvent)
}

func (r *RenderMouseRegion) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderMouseRegion) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderMouseRegion) HitTestSelf(geom.Offset) bool           { return true }
func (r *RenderMouseRegion) Cursor() Cursor                         { return r.CursorShape }
func (r *RenderMouseRegion) HandlePointer(e PointerEvent) {
	switch {
	case e.Kind == PointerEnter && r.OnEnter != nil:
		r.OnEnter(e)
	case e.Kind == PointerExit && r.OnExit != nil:
		r.OnExit(e)
	case (e.Kind == PointerHover || e.Kind == PointerMove) && r.OnHover != nil:
		r.OnHover(e)
	}
}

// --- RenderGestureDetector -------------------------------------------------

// TapDetails describes a tap position.
type TapDetails struct {
	Local, Global geom.Offset
}

// DragDetails describes a drag update.
type DragDetails struct {
	Local, Global geom.Offset
	Delta         geom.Offset // movement since the last update
	Total         geom.Offset // movement since the drag started
}

// GestureCallbacks are the recognizers a GestureDetector can have.
type GestureCallbacks struct {
	OnTapDown   func(TapDetails)
	OnTapUp     func(TapDetails)
	OnTap       func()
	OnTapCancel func()

	OnPanStart  func(DragDetails)
	OnPanUpdate func(DragDetails)
	OnPanEnd    func(DragDetails)
}

func (g *GestureCallbacks) hasTap() bool {
	return g.OnTap != nil || g.OnTapDown != nil || g.OnTapUp != nil
}
func (g *GestureCallbacks) hasPan() bool {
	return g.OnPanStart != nil || g.OnPanUpdate != nil || g.OnPanEnd != nil
}

type gestureState uint8

const (
	gestureIdle gestureState = iota
	gestureTapPossible
	gesturePanning
	gestureDead // lost to another detector / moved too far; wait for Up
)

// RenderGestureDetector recognizes taps and drags (pans) of the primary
// button/finger. Nested detectors compete through the pointer arena: the
// innermost one that can handle the gesture wins, the others get
// OnTapCancel.
type RenderGestureDetector struct {
	Box
	SingleChild
	GestureCallbacks

	state   gestureState
	pointer int
	start   geom.Offset // global position at Down
}

func (r *RenderGestureDetector) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderGestureDetector) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderGestureDetector) HitTestSelf(geom.Offset) bool           { return true }

func (r *RenderGestureDetector) HandlePointer(e PointerEvent) {
	switch e.Kind {
	case PointerDown:
		if r.state != gestureIdle || (e.Button != ButtonPrimary && !e.Touch) {
			return
		}
		if !r.hasTap() && !r.hasPan() {
			return
		}
		// "TapPossible" also means "pan possible": we don't know which it
		// is until the pointer moves past TouchSlop or is released.
		r.pointer, r.start = e.ID, e.Position
		r.state = gestureTapPossible
		if r.OnTapDown != nil {
			r.OnTapDown(TapDetails{Local: e.Local, Global: e.Position})
		}

	case PointerMove:
		if e.ID != r.pointer || r.state == gestureIdle || r.state == gestureDead {
			return
		}
		total := e.Position.Sub(r.start)
		switch r.state {
		case gestureTapPossible:
			if e.LostTo(r) {
				r.cancelTap()
				r.state = gestureDead
				return
			}
			if length(total) <= TouchSlop {
				return
			}
			// Moved too far for a tap.
			if r.hasPan() && e.Claim(r) {
				r.cancelTap()
				r.state = gesturePanning
				d := DragDetails{Local: e.Local.Sub(total), Global: r.start}
				if r.OnPanStart != nil {
					r.OnPanStart(d)
				}
				r.panUpdate(e, total)
				return
			}
			r.cancelTap()
			r.state = gestureDead
		case gesturePanning:
			r.panUpdate(e, total)
		}

	case PointerUp:
		if e.ID != r.pointer {
			return
		}
		switch r.state {
		case gestureTapPossible:
			inside := geom.RectFrom(geom.Offset{}, r.size).Contains(e.Local)
			if r.hasTap() && inside && e.Claim(r) {
				if r.OnTapUp != nil {
					r.OnTapUp(TapDetails{Local: e.Local, Global: e.Position})
				}
				if r.OnTap != nil {
					r.OnTap()
				}
			} else {
				r.cancelTap()
			}
		case gesturePanning:
			if r.OnPanEnd != nil {
				r.OnPanEnd(DragDetails{Local: e.Local, Global: e.Position, Total: e.Position.Sub(r.start)})
			}
		}
		r.state = gestureIdle

	case PointerCancel:
		if e.ID != r.pointer {
			return
		}
		if r.state == gestureTapPossible {
			r.cancelTap()
		}
		if r.state == gesturePanning && r.OnPanEnd != nil {
			r.OnPanEnd(DragDetails{Local: e.Local, Global: e.Position, Total: e.Position.Sub(r.start)})
		}
		r.state = gestureIdle
	}
}

func (r *RenderGestureDetector) cancelTap() {
	if r.state == gestureTapPossible && r.hasTap() && r.OnTapCancel != nil {
		r.OnTapCancel()
	}
}

func (r *RenderGestureDetector) panUpdate(e PointerEvent, total geom.Offset) {
	if r.OnPanUpdate != nil {
		r.OnPanUpdate(DragDetails{Local: e.Local, Global: e.Position, Delta: e.Delta, Total: total})
	}
}

func length(o geom.Offset) float32 { return float32(math.Hypot(float64(o.X), float64(o.Y))) }
