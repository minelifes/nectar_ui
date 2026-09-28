package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// chip draws the shared 32px chip body.
func chip(ctx w.BuildContext, label string, leading w.Widget, trailing w.Widget, selected, elevated bool, onTap func(), labelColor geom.Color) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	bg, border := geom.Transparent, s.OutlineVariant
	fg := labelColor
	if selected {
		bg, border, fg = s.SecondaryContainer, geom.Transparent, s.OnSecondaryContainer
	}
	elev := 0
	if elevated {
		bg, border, elev = s.SurfaceContainerLow, geom.Transparent, 1
	}
	if onTap == nil {
		dc, dbg := disabledColors(s)
		fg = dc
		if bg.A > 0 {
			bg = dbg
		}
		border = dbg
		elev = 0
	}
	padL, padR := float32(16), float32(16)
	kids := []w.Widget{}
	if leading != nil {
		padL = 8
		kids = append(kids, leading)
	}
	kids = append(kids, w.Text{Text: label, Style: Styled(th.Text.LabelLarge, fg), MaxLines: 1})
	if trailing != nil {
		padR = 8
		kids = append(kids, trailing)
	}
	bw := float32(1)
	if border.A == 0 {
		bw = 0
	}
	return InkSurface{OnTap: onTap, Color: bg, ContentColor: fg, BorderColor: border, BorderWidth: bw, Radius: CornerSmall, Elevation: elev,
		Child: w.SizedBox{Height: 32, Child: w.Padding{Padding: geom.InsetsLTRB(padL, 0, padR, 0),
			Child: w.IconTheme{Size: 18, Color: fg, Child: w.Row{Cross: w.CrossCenter, ShrinkMain: true, Spacing: 8, Children: kids}}}}}
}

// AssistChip suggests a smart action ("Add to calendar").
type AssistChip struct {
	Label     string
	Icon      *vector.Icon
	Elevated  bool
	OnPressed func()
}

func (c AssistChip) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var lead w.Widget
	if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: s.Primary}
	}
	return chip(ctx, c.Label, lead, nil, false, c.Elevated, c.OnPressed, s.OnSurface)
}

// FilterChip toggles a filter; shows a check mark when selected.
type FilterChip struct {
	Label      string
	Icon       *vector.Icon
	Selected   bool
	Elevated   bool
	OnSelected func(selected bool)
}

func (c FilterChip) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var lead w.Widget
	if c.Selected {
		lead = w.Icon{Icon: iconCheck, Size: 18, Color: s.OnSecondaryContainer}
	} else if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: s.Primary}
	}
	var tap func()
	if c.OnSelected != nil {
		tap = func() { c.OnSelected(!c.Selected) }
	}
	return chip(ctx, c.Label, lead, nil, c.Selected, c.Elevated && !c.Selected, tap, s.OnSurfaceVariant)
}

// ChoiceChip is a single-choice FilterChip (no check mark toggle-off).
type ChoiceChip struct {
	Label      string
	Selected   bool
	OnSelected func()
}

func (c ChoiceChip) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var lead w.Widget
	if c.Selected {
		lead = w.Icon{Icon: iconCheck, Size: 18, Color: s.OnSecondaryContainer}
	}
	return chip(ctx, c.Label, lead, nil, c.Selected, false, c.OnSelected, s.OnSurfaceVariant)
}

// InputChip represents user input (a contact, a tag) and can be removed.
type InputChip struct {
	Label     string
	Icon      *vector.Icon
	Avatar    w.Widget // 24px, replaces Icon
	Selected  bool
	OnPressed func()
	OnDeleted func() // shows the × button
}

func (c InputChip) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var lead, trail w.Widget
	if c.Avatar != nil {
		lead = w.SizedBox{Width: 24, Height: 24, Child: c.Avatar}
	} else if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: s.OnSurfaceVariant}
	}
	if c.OnDeleted != nil {
		fg := s.OnSurfaceVariant
		if c.Selected {
			fg = s.OnSecondaryContainer
		}
		trail = InkSurface{OnTap: c.OnDeleted, ContentColor: fg, Radius: CornerFull, NoFocus: true,
			Child: w.SizedBox{Width: 18, Height: 18, Child: w.Icon{Icon: iconClose, Size: 18, Color: fg}}}
	}
	tap := c.OnPressed
	if tap == nil && c.OnDeleted != nil {
		tap = func() {}
	}
	return chip(ctx, c.Label, lead, trail, c.Selected, false, tap, s.OnSurfaceVariant)
}

// SuggestionChip offers a dynamically generated suggestion ("Sounds good").
type SuggestionChip struct {
	Label     string
	Icon      *vector.Icon
	Elevated  bool
	OnPressed func()
}

func (c SuggestionChip) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var lead w.Widget
	if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: s.Primary}
	}
	return chip(ctx, c.Label, lead, nil, false, c.Elevated, c.OnPressed, s.OnSurfaceVariant)
}
