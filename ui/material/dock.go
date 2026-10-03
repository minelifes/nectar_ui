package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// Dock is widgets.Dock with Material tabs: panels in tab groups and
// resizable splits that the user rearranges by dragging tabs (see
// widgets.Dock and widgets.DockController).
type Dock struct {
	Controller *w.DockController
	Panels     []w.DockPanel
	OnClose    func(id string)
	Empty      w.Widget
	// Style overrides Theme.Dock.
	Style DockTheme
}

func (d Dock) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Dock, d.Style)
	ts := pickTC(st.TextStyle, th.Text.LabelLarge, sc.OnSurfaceVariant)
	ats := pickTC(st.ActiveTextStyle, ts, sc.OnSurface)
	tabH := pickF(st.TabHeight, 36)
	style := w.DockStyle{
		TabBarColor:     pick(st.TabBarColor, sc.SurfaceContainer),
		TabColor:        st.TabColor,
		ActiveTabColor:  pick(st.ActiveTabColor, sc.Surface),
		IndicatorColor:  pick(st.IndicatorColor, sc.Primary),
		DividerColor:    pick(st.DividerColor, sc.OutlineVariant),
		DropHintColor:   pick(st.DropHintColor, sc.Primary.WithAlpha(0.16)),
		TabTextStyle:    ts,
		ActiveTextStyle: ats,
		TabHeight:       tabH,
	}
	return w.Dock{Controller: d.Controller, Panels: d.Panels, OnClose: d.OnClose, Empty: d.Empty, Style: style,
		Tab: func(ctx w.BuildContext, t w.DockTab) w.Widget {
			bg, fg := style.TabColor, ts
			if t.Active {
				bg, fg = style.ActiveTabColor, ats
			}
			kids := []w.Widget{w.Text{Text: t.Panel.Title, Style: fg, MaxLines: 1}}
			if t.Close != nil {
				kids = append(kids, w.GestureDetector{OnTap: t.Close, Child: w.MouseRegion{Cursor: w.CursorPointer,
					Child: w.Icon{Icon: iconClose, Size: 16, Color: fg.Color}}})
			}
			layers := []w.Widget{w.Padding{Padding: geom.InsetsHV(12, 0), Child: w.Row{Cross: w.CrossCenter, Spacing: 8, Children: kids}}}
			if t.Active {
				layers = append(layers, w.Positioned{Left: w.At(0), Right: w.At(0), Bottom: w.At(0), Height: w.At(2),
					Child: w.DecoratedBox{Color: style.IndicatorColor}})
			}
			return w.DecoratedBox{Color: bg, Child: w.SizedBox{Height: tabH, Child: w.Stack{Children: layers}}}
		}}
}
