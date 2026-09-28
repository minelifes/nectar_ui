package widgets

import "github.com/minelifes/nectar_ui/ui/render"

// Re-exported so users only need to import widgets.
type (
	PointerEvent = render.PointerEvent
	TapDetails   = render.TapDetails
	DragDetails  = render.DragDetails
	Cursor       = render.Cursor
)

const (
	CursorDefault    = render.CursorDefault
	CursorPointer    = render.CursorPointer
	CursorText       = render.CursorText
	CursorCrosshair  = render.CursorCrosshair
	CursorMove       = render.CursorMove
	CursorResizeNS   = render.CursorResizeNS
	CursorResizeEW   = render.CursorResizeEW
	CursorNotAllowed = render.CursorNotAllowed
)

// GestureDetector recognizes taps and drags on its child.
//
// Nested detectors compete: the innermost one that handles the gesture wins,
// the outer ones get OnTapCancel. A drag starts once the pointer moves more
// than render.TouchSlop; that cancels the tap.
type GestureDetector struct {
	OnTapDown   func(TapDetails)
	OnTapUp     func(TapDetails)
	OnTap       func()
	OnTapCancel func()

	OnPanStart  func(DragDetails)
	OnPanUpdate func(DragDetails)
	OnPanEnd    func(DragDetails)

	Child Widget
}

func (w GestureDetector) ChildWidget() Widget { return w.Child }

func (w GestureDetector) callbacks() render.GestureCallbacks {
	return render.GestureCallbacks{
		OnTapDown: w.OnTapDown, OnTapUp: w.OnTapUp, OnTap: w.OnTap, OnTapCancel: w.OnTapCancel,
		OnPanStart: w.OnPanStart, OnPanUpdate: w.OnPanUpdate, OnPanEnd: w.OnPanEnd,
	}
}

func (w GestureDetector) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderGestureDetector{GestureCallbacks: w.callbacks()}
}

// UpdateRenderObject swaps in the new closures. Gesture state (a tap in
// progress) lives in the render object, so it survives rebuilds.
func (w GestureDetector) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderGestureDetector).GestureCallbacks = w.callbacks()
}

// MouseRegion reports hover and sets the mouse cursor over its child.
type MouseRegion struct {
	Cursor  Cursor
	OnEnter func(PointerEvent)
	OnExit  func(PointerEvent)
	OnHover func(PointerEvent)
	Child   Widget
}

func (w MouseRegion) ChildWidget() Widget { return w.Child }

func (w MouseRegion) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderMouseRegion{}
	w.UpdateRenderObject(nil, r)
	return r
}

func (w MouseRegion) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderMouseRegion)
	r.CursorShape, r.OnEnter, r.OnExit, r.OnHover = w.Cursor, w.OnEnter, w.OnExit, w.OnHover
}

// Listener passes raw pointer events (down/move/up/scroll/...) to OnEvent.
// Use it for things gestures don't cover, like scroll wheels.
type Listener struct {
	OnEvent func(PointerEvent)
	Child   Widget
}

func (w Listener) ChildWidget() Widget { return w.Child }

func (w Listener) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderPointerListener{OnEvent: w.OnEvent}
}

func (w Listener) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderPointerListener).OnEvent = w.OnEvent
}
