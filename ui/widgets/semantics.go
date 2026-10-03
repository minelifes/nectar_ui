package widgets

import (
	"strings"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// SemanticsData describes a part of the UI for assistive technology and
// tests: its role, label, value and state, and its default action.
type SemanticsData = render.SemanticsData

// Semantics annotates Child (Material controls annotate themselves). It
// changes nothing on screen; SemanticsTree collects the annotations: a
// screen reader bridge, UI tests ("tap the button labeled Save") and
// automation read them.
type Semantics struct {
	SemanticsData
	Child Widget
}

func (w Semantics) ChildWidget() Widget { return w.Child }

func (w Semantics) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderSemantics{Data: w.SemanticsData}
}

func (Semantics) MarksOwnPaint() {}

func (w Semantics) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderSemantics).Data = w.SemanticsData
}

// SemanticsNode is an annotated part of the UI with its place in the
// window and the annotated parts inside it.
type SemanticsNode struct {
	SemanticsData
	ID       uint64    // stable while the node is in the tree
	Rect     geom.Rect // window coordinates (clipped to what's visible)
	Focused  bool      // keyboard focus is inside it (see MarkFocus)
	Children []*SemanticsNode
	ro       render.RenderObject
}

// SemanticsTree returns the annotated parts of the UI under root, as a
// forest in paint order. Hidden parts (offstage, transparent, clipped
// away) are left out.
func SemanticsTree(root render.RenderObject) []*SemanticsNode {
	var walk func(r render.RenderObject, clip geom.Rect) []*SemanticsNode
	walk = func(r render.RenderObject, clip geom.Rect) []*SemanticsNode {
		switch x := r.(type) {
		case *render.RenderOffstage:
			if x.Offstage {
				return nil
			}
		case *render.RenderOpacity:
			if x.Opacity <= 0 {
				return nil
			}
		}
		rect := geom.RectFrom(render.GlobalOrigin(r), r.Base().Size())
		switch r.(type) {
		case *render.RenderViewport, *render.RenderViewport2D, *render.RenderClipRect:
			clip = clip.Intersect(rect)
		}
		var kids []*SemanticsNode
		r.VisitChildren(func(c render.RenderObject) { kids = append(kids, walk(c, clip)...) })
		if s, ok := r.(*render.RenderSemantics); ok {
			vis := rect.Intersect(clip)
			if vis.Empty() {
				return kids
			}
			return []*SemanticsNode{{SemanticsData: s.Data, ID: s.ID(), Rect: vis, Children: kids, ro: s}}
		}
		return kids
	}
	if root == nil {
		return nil
	}
	big := geom.Rect{X: -1e9, Y: -1e9, W: 2e9, H: 2e9}
	if b := root.Base().Size(); b.W > 0 {
		big = geom.RectFrom(geom.Offset{}, b)
	}
	return walk(root, big)
}

// MarkFocus marks the innermost node that holds the keyboard focus of fm
// (Focused) and returns it (nil if none).
func MarkFocus(nodes []*SemanticsNode, fm *FocusManager) *SemanticsNode {
	n := fm.Primary()
	if n == nil || n.element == nil {
		return nil
	}
	ro := n.element.RenderObject()
	var found *SemanticsNode
	all := FlattenSemantics(nodes)
	for r := ro; r != nil && found == nil; r = r.Base().Parent() {
		for _, sn := range all {
			if sn.ro == r {
				found = sn
				break
			}
		}
	}
	if found != nil {
		found.Focused = true
	}
	return found
}

// FlattenSemantics lists every node of a semantics forest, depth first.
func FlattenSemantics(nodes []*SemanticsNode) []*SemanticsNode {
	var out []*SemanticsNode
	var walk func([]*SemanticsNode)
	walk = func(ns []*SemanticsNode) {
		for _, n := range ns {
			out = append(out, n)
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

// String describes the node on one line ("button "Save" disabled").
func (n *SemanticsNode) String() string {
	var b strings.Builder
	b.WriteString(n.Role)
	if n.Label != "" {
		b.WriteString(" \"" + n.Label + "\"")
	}
	if n.Value != "" {
		b.WriteString(" = " + n.Value)
	}
	if n.Checkable {
		if n.Checked {
			b.WriteString(" checked")
		} else {
			b.WriteString(" unchecked")
		}
	}
	if n.Selected {
		b.WriteString(" selected")
	}
	if n.Disabled {
		b.WriteString(" disabled")
	}
	return b.String()
}
