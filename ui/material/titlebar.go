package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// TitleBar is a Material window title bar for apps that draw their own
// (ui.Config.WithCustomTitleBar): the title (or any Leading widgets) on the
// left, Actions (icon buttons) on the right, between the system window
// buttons. Empty space drags the window. Colors come from the theme:
// SurfaceContainer background, OnSurfaceVariant window buttons.
//
//	material.TitleBar{Title: "Aether", Actions: []widgets.Widget{
//	    material.IconButton{Icon: icons.Tune, OnPressed: openSettings},
//	}}
type TitleBar struct {
	Title   string
	Leading []w.Widget // shown before the title (logo, tabs, ...)
	Center  w.Widget   // optional, in the middle of the window (document name, search)
	Actions []w.Widget
	// Height; 0 = 40 (the macOS traffic lights stay in the top 28 px).
	Height float32
	// Color overrides the background.
	Color *geom.Color
}

func (t TitleBar) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	bg := sc.SurfaceContainer
	if t.Color != nil {
		bg = *t.Color
	}
	h := t.Height
	if h <= 0 {
		h = 40
	}
	left := append([]w.Widget(nil), t.Leading...)
	if t.Title != "" {
		st := th.Text.TitleSmall
		st.Color = sc.OnSurface
		left = append(left, w.Text{Text: t.Title, Style: st})
	}
	row := []w.Widget{
		w.Expanded{Child: w.Row{Cross: w.CrossCenter, Spacing: 8, Children: left}},
		w.Row{ShrinkMain: true, Cross: w.CrossCenter, Spacing: 4, Children: t.Actions},
	}
	var center w.Widget
	if t.Center != nil {
		center = w.DefaultTextStyle{Style: centerStyle(th), Child: t.Center}
	}
	return w.TitleBar{Height: h, Color: bg, Center: center,
		Buttons: w.WindowButtons{Color: sc.OnSurfaceVariant, Height: h},
		Child:   w.IconTheme{Size: 20, Color: sc.OnSurfaceVariant, Child: w.Row{Cross: w.CrossCenter, Children: row}}}
}

func centerStyle(th Theme) text.Style {
	st := th.Text.BodyMedium
	st.Color = th.Scheme.OnSurfaceVariant
	return st
}
