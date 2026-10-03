package render

import (
	"sync/atomic"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// SemanticsData describes what a part of the UI is, for assistive
// technology and tests (see widgets.Semantics).
type SemanticsData struct {
	Role      string // "button", "checkbox", "switch", "radio", "textfield", "slider", "tab", "menuitem", "header", "image", "link", ...
	Label     string
	Value     string
	Hint      string
	Checkable bool
	Checked   bool
	Selected  bool
	Disabled  bool
	// OnTap performs the element's default action (nil = none).
	OnTap func()
}

// RenderSemantics annotates its child with SemanticsData. It doesn't
// change layout or painting.
type RenderSemantics struct {
	Box
	SingleChild
	Data SemanticsData
	id   uint64
}

var semanticsIDs atomic.Uint64

// ID identifies the node for as long as it's in the tree (accessibility
// bridges hand it to the OS).
func (r *RenderSemantics) ID() uint64 {
	if r.id == 0 {
		r.id = semanticsIDs.Add(1)
	}
	return r.id
}

func (r *RenderSemantics) PerformLayout(c geom.Constraints) geom.Size { return layoutProxy(r.child, c) }
func (r *RenderSemantics) Paint(ctx *PaintContext, o geom.Offset)     { ctx.PaintChild(r.child, o) }
