package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// SplitView is widgets.SplitView with M3 styling: panes separated by a
// spacing gap holding a pill-shaped drag handle that widens and darkens
// while hovered or dragged. Panes are plain; wrap their content in a Card
// or Surface for the M3 "pane" look.
type SplitView struct {
	Panes    []w.Pane
	Vertical bool
	Gap      float32 // default 24 (M3 pane spacing)
	OnResize func(sizes []float32)
	// Style overrides Theme.SplitView.
	Style SplitViewTheme
}

func (s SplitView) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.SplitView, s.Style)
	gap := s.Gap
	if gap == 0 {
		gap = 24
	}
	return w.SplitView{Panes: s.Panes, Vertical: s.Vertical, Gap: gap, OnResize: s.OnResize,
		Divider: func(d w.DividerState) w.Widget {
			col := pick(st.HandleColor, sc.Outline)
			thick, length := float32(4), float32(48)
			if d.Hovered {
				col = pick(st.HoverHandleColor, sc.OnSurfaceVariant)
			}
			if d.Dragging {
				col, thick, length = pick(st.DraggedHandleColor, sc.OnSurface), 12, 52
			}
			return w.CustomPaint{Size: geom.Sz(geom.Inf, geom.Inf), Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				var r geom.Rect
				if d.Vertical {
					r = geom.Rect{X: o.X + (size.W-length)/2, Y: o.Y + (size.H-thick)/2, W: length, H: thick}
				} else {
					r = geom.Rect{X: o.X + (size.W-thick)/2, Y: o.Y + (size.H-length)/2, W: thick, H: length}
				}
				c.FillRoundRect(r, thick/2, col)
			}}
		}}
}
