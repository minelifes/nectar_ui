package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// chipLook resolves Theme.Chip with a chip's own Style.
func chipLook(ctx w.BuildContext, own ChipTheme) (Theme, ChipTheme) {
	th := ThemeOf(ctx)
	return th, merge(th.Chip, own)
}

// chip draws the shared 32px chip body.
func chip(th Theme, st ChipTheme, label string, leading w.Widget, trailing w.Widget, selected, elevated bool, onTap func(), labelColor geom.Color) w.Widget {
	s := th.Scheme
	bg, border := pick(st.BackgroundColor, geom.Transparent), pick(st.SideColor, s.OutlineVariant)
	fg := pick(st.LabelColor, labelColor)
	bw := pickF(st.SideWidth, 1)
	if selected {
		bg, border, fg = pick(st.SelectedColor, s.SecondaryContainer), geom.Transparent, pick(st.SelectedLabelColor, s.OnSecondaryContainer)
	}
	elev := 0
	if elevated {
		bg, border, elev = pick(st.BackgroundColor, s.SurfaceContainerLow), geom.Transparent, pickI(st.Elevation, 1)
	}
	if onTap == nil {
		dc, dbg := disabledColors(s)
		fg = dc
		if bg.A > 0 {
			bg = pick(st.DisabledColor, dbg)
		}
		border = dbg
		elev = 0
	}
	pad := pickE(st.Padding, geom.InsetsHV(16, 0))
	padL, padR := pad.Left, pad.Right
	kids := []w.Widget{}
	if leading != nil {
		padL = max(padL-8, 0)
		kids = append(kids, leading)
	}
	kids = append(kids, w.Text{Text: label, Style: pickTC(st.LabelStyle, th.Text.LabelLarge, fg), MaxLines: 1})
	if trailing != nil {
		padR = max(padR-8, 0)
		kids = append(kids, trailing)
	}
	if border.A == 0 {
		bw = 0
	}
	return InkSurface{OnTap: onTap, Color: bg, ContentColor: fg, BorderColor: border, BorderWidth: bw, Radius: pickF(st.Radius, CornerSmall), Elevation: elev,
		Child: w.SizedBox{Height: pickF(st.Height, 32), Child: w.Padding{Padding: geom.InsetsLTRB(padL, 0, padR, 0),
			Child: w.IconTheme{Size: 18, Color: fg, Child: w.Row{Cross: w.CrossCenter, ShrinkMain: true, Spacing: 8, Children: kids}}}}}
}

// AssistChip suggests a smart action ("Add to calendar").
type AssistChip struct {
	Label     string
	Icon      *vector.Icon
	Elevated  bool
	OnPressed func()
	Style     ChipTheme // overrides Theme.Chip
}

func (c AssistChip) Build(ctx w.BuildContext) w.Widget {
	th, st := chipLook(ctx, c.Style)
	s := th.Scheme
	var lead w.Widget
	if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: pick(st.IconColor, s.Primary)}
	}
	return chip(th, st, c.Label, lead, nil, false, c.Elevated, c.OnPressed, s.OnSurface)
}

// FilterChip toggles a filter; shows a check mark when selected.
type FilterChip struct {
	Label      string
	Icon       *vector.Icon
	Selected   bool
	Elevated   bool
	OnSelected func(selected bool)
	Style      ChipTheme // overrides Theme.Chip
}

func (c FilterChip) Build(ctx w.BuildContext) w.Widget {
	th, st := chipLook(ctx, c.Style)
	s := th.Scheme
	var lead w.Widget
	if c.Selected {
		lead = w.Icon{Icon: iconCheck, Size: 18, Color: pick(st.CheckmarkColor, pick(st.SelectedLabelColor, s.OnSecondaryContainer))}
	} else if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: pick(st.IconColor, s.Primary)}
	}
	var tap func()
	if c.OnSelected != nil {
		tap = func() { c.OnSelected(!c.Selected) }
	}
	return chip(th, st, c.Label, lead, nil, c.Selected, c.Elevated && !c.Selected, tap, s.OnSurfaceVariant)
}

// ChoiceChip is a single-choice FilterChip (no check mark toggle-off).
type ChoiceChip struct {
	Label      string
	Selected   bool
	OnSelected func()
	Style      ChipTheme // overrides Theme.Chip
}

func (c ChoiceChip) Build(ctx w.BuildContext) w.Widget {
	th, st := chipLook(ctx, c.Style)
	s := th.Scheme
	var lead w.Widget
	if c.Selected {
		lead = w.Icon{Icon: iconCheck, Size: 18, Color: pick(st.CheckmarkColor, pick(st.SelectedLabelColor, s.OnSecondaryContainer))}
	}
	return chip(th, st, c.Label, lead, nil, c.Selected, false, c.OnSelected, s.OnSurfaceVariant)
}

// InputChip represents user input (a contact, a tag) and can be removed.
type InputChip struct {
	Label     string
	Icon      *vector.Icon
	Avatar    w.Widget // 24px, replaces Icon
	Selected  bool
	OnPressed func()
	OnDeleted func()    // shows the × button
	Style     ChipTheme // overrides Theme.Chip
}

func (c InputChip) Build(ctx w.BuildContext) w.Widget {
	th, st := chipLook(ctx, c.Style)
	s := th.Scheme
	var lead, trail w.Widget
	if c.Avatar != nil {
		lead = w.SizedBox{Width: 24, Height: 24, Child: c.Avatar}
	} else if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: pick(st.IconColor, s.OnSurfaceVariant)}
	}
	if c.OnDeleted != nil {
		fg := s.OnSurfaceVariant
		if c.Selected {
			fg = s.OnSecondaryContainer
		}
		fg = pick(st.DeleteIconColor, fg)
		trail = InkSurface{OnTap: c.OnDeleted, ContentColor: fg, Radius: CornerFull, NoFocus: true,
			Child: w.SizedBox{Width: 18, Height: 18, Child: w.Icon{Icon: iconClose, Size: 18, Color: fg}}}
	}
	tap := c.OnPressed
	if tap == nil && c.OnDeleted != nil {
		tap = func() {}
	}
	return chip(th, st, c.Label, lead, trail, c.Selected, false, tap, s.OnSurfaceVariant)
}

// SuggestionChip offers a dynamically generated suggestion ("Sounds good").
type SuggestionChip struct {
	Label     string
	Icon      *vector.Icon
	Elevated  bool
	OnPressed func()
	Style     ChipTheme // overrides Theme.Chip
}

func (c SuggestionChip) Build(ctx w.BuildContext) w.Widget {
	th, st := chipLook(ctx, c.Style)
	s := th.Scheme
	var lead w.Widget
	if c.Icon != nil {
		lead = w.Icon{Icon: c.Icon, Size: 18, Color: pick(st.IconColor, s.Primary)}
	}
	return chip(th, st, c.Label, lead, nil, false, c.Elevated, c.OnPressed, s.OnSurfaceVariant)
}
