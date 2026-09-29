package widgets

import (
	"slices"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// Pane is one resizable panel of a SplitView.
type Pane struct {
	Child Widget
	Size  float32 // initial size along the axis; 0 = share the rest equally
	Min   float32 // smallest size (0 = can collapse to nothing)
	Max   float32 // largest size (0 = no limit)
}

// DividerState is what a custom divider gets to draw itself.
type DividerState struct {
	Index    int  // divider between pane Index and Index+1
	Vertical bool // true when the panes are stacked (divider is horizontal)
	Hovered  bool
	Dragging bool
}

// SplitView lays out Panes in a row (or column, with Vertical) separated by
// draggable dividers. Dragging a divider resizes the panes next to it;
// when a pane reaches its Min or Max, the change carries on to the next
// pane in that direction. Pane sizes keep their proportions when the view
// itself is resized.
//
// Keyboard: after clicking a divider, the arrow keys move it by 10px
// (Shift: 50px). Double-clicking isn't recognized yet.
type SplitView struct {
	Panes    []Pane
	Vertical bool
	// Gap is the divider thickness (the space between panes and the drag
	// target); default 8.
	Gap float32
	// Divider draws a divider; nil = a 1px line that highlights on hover.
	Divider func(d DividerState) Widget
	// DividerColor / ActiveColor style the default divider.
	DividerColor geom.Color
	ActiveColor  geom.Color
	// OnResize is called with the new pane sizes after a drag.
	OnResize func(sizes []float32)
}

func (SplitView) CreateState() State { return &splitState{} }

type splitState struct {
	StateBase
	desired  []float32 // what the user dragged to (nil = use Pane.Size)
	resolved []float32 // actual sizes from the last layout
	start    []float32 // sizes when the current drag began
	hovered  int
	dragging int
	nodes    []*FocusNode
}

func (s *splitState) InitState() { s.hovered, s.dragging = -1, -1 }

func (s *splitState) sv() SplitView { return WidgetOf[SplitView](s) }

func (s *splitState) limits() (mins, maxs []float32) {
	for _, p := range s.sv().Panes {
		mins = append(mins, p.Min)
		maxs = append(maxs, p.Max)
	}
	return
}

func (s *splitState) DidUpdateWidget(old Widget) {
	// A different number of panes invalidates the dragged sizes.
	if len(old.(SplitView).Panes) != len(s.sv().Panes) {
		s.desired, s.resolved = nil, nil
	}
}

// moveDivider applies a drag of delta (from the drag start) to divider i.
func (s *splitState) moveDivider(i int, base []float32, delta float32) {
	if len(base) == 0 {
		return
	}
	mins, maxs := s.limits()
	next := render.DragSplit(base, mins, maxs, i, delta)
	if slices.Equal(next, s.desired) {
		return
	}
	s.SetState(func() { s.desired = next })
	if cb := s.sv().OnResize; cb != nil {
		cb(append([]float32(nil), next...))
	}
}

func (s *splitState) Build(BuildContext) Widget {
	sv := s.sv()
	gap := sv.Gap
	if gap == 0 {
		gap = 8
	}
	n := len(sv.Panes)
	for len(s.nodes) < n {
		s.nodes = append(s.nodes, &FocusNode{})
	}
	desired := s.desired
	if desired == nil {
		for _, p := range sv.Panes {
			desired = append(desired, p.Size)
		}
	}
	kids := make([]Widget, 0, 2*n)
	for i, p := range sv.Panes {
		kids = append(kids, KeyedSubtree{ID: i, Child: p.Child})
		if i == n-1 {
			break
		}
		i := i
		st := DividerState{Index: i, Vertical: sv.Vertical, Hovered: s.hovered == i, Dragging: s.dragging == i}
		var look Widget
		if sv.Divider != nil {
			look = sv.Divider(st)
		} else {
			look = defaultDivider(st, sv.DividerColor, sv.ActiveColor)
		}
		cursor := CursorResizeEW
		if sv.Vertical {
			cursor = CursorResizeNS
		}
		axis := func(o geom.Offset) float32 {
			if sv.Vertical {
				return o.Y
			}
			return o.X
		}
		node := s.nodes[i]
		node.OnKey = func(e KeyEvent) bool {
			step := float32(10)
			if e.Mods.Shift() {
				step = 50
			}
			var d float32
			switch {
			case !sv.Vertical && e.Key == KeyLeft, sv.Vertical && e.Key == KeyUp:
				d = -step
			case !sv.Vertical && e.Key == KeyRight, sv.Vertical && e.Key == KeyDown:
				d = step
			default:
				return false
			}
			s.moveDivider(i, s.resolved, d)
			return true
		}
		kids = append(kids, KeyedSubtree{ID: -1 - i, Child: Focus{Node: node, Child: MouseRegion{
			Cursor:  cursor,
			OnEnter: func(PointerEvent) { s.SetState(func() { s.hovered = i }) },
			OnExit: func(PointerEvent) {
				s.SetState(func() {
					if s.hovered == i {
						s.hovered = -1
					}
				})
			},
			Child: GestureDetector{
				OnTapDown: func(TapDetails) { node.RequestFocus() },
				OnPanStart: func(DragDetails) {
					node.RequestFocus()
					s.SetState(func() {
						s.dragging = i
						s.start = append([]float32(nil), s.resolved...)
					})
				},
				// Use the total movement since the drag started, so dragging
				// back past a limit restores the sizes exactly.
				OnPanUpdate: func(d DragDetails) { s.moveDivider(i, s.start, axis(d.Total)) },
				OnPanEnd:    func(DragDetails) { s.SetState(func() { s.dragging = -1 }) },
				Child:       look,
			},
		}}})
	}
	mins, maxs := s.limits()
	return splitLayout{vertical: sv.Vertical, gap: gap, mins: mins, maxs: maxs, desired: desired, state: s, children: kids}
}

// defaultDivider is a 1px line, thicker and colored while hovered/dragged.
func defaultDivider(d DividerState, line, active geom.Color) Widget {
	if line == (geom.Color{}) {
		line = geom.Black.WithAlpha(0.12)
	}
	if active == (geom.Color{}) {
		active = geom.Hex(0x1A73E8)
	}
	col, thick := line, float32(1)
	if d.Hovered || d.Dragging {
		col, thick = active, 3
	}
	return CustomPaint{Size: geom.Sz(geom.Inf, geom.Inf), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		if d.Vertical {
			c.FillRect(geom.Rect{X: o.X, Y: o.Y + (s.H-thick)/2, W: s.W, H: thick}, col)
		} else {
			c.FillRect(geom.Rect{X: o.X + (s.W-thick)/2, Y: o.Y, W: thick, H: s.H}, col)
		}
	}}
}

// splitLayout is the render-object widget behind SplitView.
type splitLayout struct {
	vertical   bool
	gap        float32
	mins, maxs []float32
	desired    []float32
	state      *splitState
	children   []Widget
}

func (w splitLayout) ChildWidgets() []Widget { return w.children }

func (w splitLayout) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderSplit{}
	w.UpdateRenderObject(nil, r)
	return r
}

func (splitLayout) MarksOwnPaint() {}
func (w splitLayout) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderSplit)
	st := w.state
	r.OnResolved = func(sizes []float32) { st.resolved = sizes }
	if r.Vertical != w.vertical || r.Gap != w.gap || !slices.Equal(r.Mins, w.mins) ||
		!slices.Equal(r.Maxs, w.maxs) || !slices.Equal(r.Desired, w.desired) {
		r.Vertical, r.Gap = w.vertical, w.gap
		r.Mins, r.Maxs, r.Desired = w.mins, w.maxs, w.desired
		render.MarkNeedsLayout(r)
	}
}
