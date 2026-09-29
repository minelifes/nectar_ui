package render

import "github.com/minelifes/nectar_ui/ui/geom"

// RenderWindowDragArea marks its box as part of a custom title bar: pressing
// there and dragging moves the window (widgets.WindowDragArea). Interactive
// descendants (buttons, text fields, gesture detectors) keep their clicks.
type RenderWindowDragArea struct {
	Box
	SingleChild
}

func (r *RenderWindowDragArea) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}
func (r *RenderWindowDragArea) Paint(ctx *PaintContext, o geom.Offset) { ctx.PaintChild(r.child, o) }
func (r *RenderWindowDragArea) HitTestSelf(geom.Offset) bool           { return true }

// WindowHit is what the OS should do with a press at some point of a
// window whose chrome the app draws itself.
type WindowHit uint8

const (
	WindowHitClient  WindowHit = iota // the app handles it
	WindowHitCaption                  // drag the window (title bar)
	WindowHitResizeN
	WindowHitResizeS
	WindowHitResizeW
	WindowHitResizeE
	WindowHitResizeNW
	WindowHitResizeNE
	WindowHitResizeSW
	WindowHitResizeSE
)

func (h WindowHit) String() string {
	return [...]string{"client", "caption", "resize-n", "resize-s", "resize-w", "resize-e",
		"resize-nw", "resize-ne", "resize-sw", "resize-se"}[h]
}

// WindowHitAt classifies pos (window coordinates) in a window of the given
// size. Within border of an edge it's a resize handle (border 0 = none, as
// when maximized); otherwise it's the caption if the topmost thing under
// pos is a RenderWindowDragArea rather than something interactive inside
// it.
func WindowHitAt(root RenderObject, size geom.Size, pos geom.Offset, border float32) WindowHit {
	if border > 0 {
		n, s := pos.Y < border, pos.Y >= size.H-border
		w, e := pos.X < border, pos.X >= size.W-border
		// Corners are easier to grab with a larger target.
		corner := 2 * border
		nc, sc := pos.Y < corner, pos.Y >= size.H-corner
		wc, ec := pos.X < corner, pos.X >= size.W-corner
		switch {
		case (n && wc) || (w && nc):
			return WindowHitResizeNW
		case (n && ec) || (e && nc):
			return WindowHitResizeNE
		case (s && wc) || (w && sc):
			return WindowHitResizeSW
		case (s && ec) || (e && sc):
			return WindowHitResizeSE
		case n:
			return WindowHitResizeN
		case s:
			return WindowHitResizeS
		case w:
			return WindowHitResizeW
		case e:
			return WindowHitResizeE
		}
	}
	for _, h := range HitTestAt(root, pos) { // deepest first
		switch h.Target.(type) {
		case *RenderWindowDragArea:
			return WindowHitCaption
		case *RenderMouseRegion:
			// Hover only (tooltips, cursors): still draggable.
		case PointerHandler, *RenderAbsorbPointer:
			return WindowHitClient
		}
	}
	return WindowHitClient
}
