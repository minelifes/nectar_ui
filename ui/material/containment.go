package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// ---------------------------------------------------------------------------
// Card

// CardVariant selects the card style.
type CardVariant uint8

const (
	CardElevated CardVariant = iota
	CardFilled
	CardOutlined
)

// Card groups related content on a surface. With OnTap it's interactive.
type Card struct {
	Variant CardVariant
	OnTap   func()
	Padding geom.EdgeInsets // default none
	Child   w.Widget
	// Style overrides Theme.Card.
	Style CardTheme
}

func (c Card) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	var bg, border geom.Color
	var bw float32
	elev := 0
	switch c.Variant {
	case CardElevated:
		bg, elev = s.SurfaceContainerLow, 1
	case CardFilled:
		bg = s.SurfaceContainerHighest
	case CardOutlined:
		bg, border, bw = s.Surface, s.OutlineVariant, 1
	}
	st := merge(th.Card, c.Style)
	bg = pick(st.Color, bg)
	border, bw = side(st.BorderColor, st.BorderWidth, s.OutlineVariant, bw)
	elev = pickI(st.Elevation, elev)
	radius := pickF(st.Radius, CornerMedium)
	child := c.Child
	if !c.Padding.IsZero() {
		child = w.Padding{Padding: c.Padding, Child: child}
	}
	var card w.Widget
	if c.OnTap == nil {
		card = Surface{Color: bg, Radius: radius, BorderColor: border, BorderWidth: bw, Elevation: elev, ShadowColor: st.ShadowColor, Child: child}
	} else {
		card = InkSurface{OnTap: c.OnTap, Color: bg, ContentColor: pick(st.OverlayColor, s.OnSurface), BorderColor: border, BorderWidth: bw,
			Radius: radius, Elevation: elev, ShadowColor: st.ShadowColor, RaiseOnHover: c.Variant == CardElevated && elev > 0, Child: child}
	}
	if st.Margin != nil {
		card = w.Padding{Padding: *st.Margin, Child: card}
	}
	return card
}

// ---------------------------------------------------------------------------
// Dividers

// Divider is a thin horizontal line. Styled by Theme.Divider and Style.
type Divider struct {
	Indent, EndIndent float32
	Height            float32 // total space; default Style / 1
	Style             DividerTheme
}

func dividerLook(ctx w.BuildContext, own DividerTheme, indent, end, space float32) (col geom.Color, thick, in, out, sp float32) {
	th := ThemeOf(ctx)
	st := merge(th.Divider, own)
	thick = max(pickF(st.Thickness, 1), 0)
	in, out = indent, end
	if in == 0 {
		in = pickF(st.Indent, 0)
	}
	if out == 0 {
		out = pickF(st.EndIndent, 0)
	}
	sp = space
	if sp == 0 {
		sp = pickF(st.Space, thick)
	}
	return pick(st.Color, th.Scheme.OutlineVariant), thick, in, out, max(sp, thick)
}

func (d Divider) Build(ctx w.BuildContext) w.Widget {
	col, thick, in, out, h := dividerLook(ctx, d.Style, d.Indent, d.EndIndent, d.Height)
	return w.CustomPaint{Size: geom.Sz(geom.Inf, h), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		c.FillRect(geom.Rect{X: o.X + in, Y: o.Y + (s.H-thick)/2, W: s.W - in - out, H: thick}, col)
	}}
}

// VerticalDivider is a thin vertical line. Styled by Theme.Divider and
// Style.
type VerticalDivider struct {
	Indent, EndIndent float32
	Width             float32
	Style             DividerTheme
}

func (d VerticalDivider) Build(ctx w.BuildContext) w.Widget {
	col, thick, in, out, wd := dividerLook(ctx, d.Style, d.Indent, d.EndIndent, d.Width)
	return w.CustomPaint{Size: geom.Sz(wd, geom.Inf), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		c.FillRect(geom.Rect{X: o.X + (s.W-thick)/2, Y: o.Y + in, W: thick, H: s.H - in - out}, col)
	}}
}

// ---------------------------------------------------------------------------
// ListTile

// ListTile is one row of a list: leading, title, subtitle, trailing.
type ListTile struct {
	Leading  w.Widget // icon (24) or avatar (40)
	Title    string
	Subtitle string
	Overline string
	Trailing w.Widget // icon, text or control
	// TitleWidget replaces Title (for custom content).
	TitleWidget w.Widget
	ThreeLine   bool // subtitle can wrap to two lines
	Selected    bool
	Disabled    bool
	OnTap       func()
	Dense       bool
	// Style overrides Theme.ListTile.
	Style ListTileTheme
}

func (t ListTile) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	st := merge(th.ListTile, t.Style)
	titleC, subC, iconC := pick(st.TextColor, s.OnSurface), pick(st.SubtitleColor, s.OnSurfaceVariant), pick(st.IconColor, s.OnSurfaceVariant)
	bg := pick(st.TileColor, geom.Transparent)
	if t.Selected {
		sel := pick(st.SelectedColor, s.OnSecondaryContainer)
		bg, titleC, iconC = pick(st.SelectedTileColor, s.SecondaryContainer), sel, sel
	}
	if t.Disabled {
		dc, _ := disabledColors(s)
		titleC, subC, iconC = dc, dc, dc
	}
	lines := []w.Widget{}
	if t.Overline != "" {
		lines = append(lines, w.Text{Text: t.Overline, Style: pickTC(st.SubtitleTextStyle, th.Text.LabelSmall, subC), MaxLines: 1, Ellipsis: true})
	}
	if t.TitleWidget != nil {
		lines = append(lines, t.TitleWidget)
	} else if t.Title != "" {
		ts, tileStyle := th.Text.BodyLarge, st
		if t.Dense {
			ts = th.Text.BodyMedium
		}
		lines = append(lines, w.Text{Text: t.Title, Style: pickTC(tileStyle.TitleTextStyle, ts, titleC), MaxLines: 1, Ellipsis: true})
	}
	if t.Subtitle != "" {
		ml := 1
		if t.ThreeLine {
			ml = 2
		}
		lines = append(lines, w.Text{Text: t.Subtitle, Style: pickTC(st.SubtitleTextStyle, th.Text.BodyMedium, subC), MaxLines: ml, Ellipsis: true})
	}
	minH := float32(56)
	switch {
	case t.ThreeLine:
		minH = 88
	case t.Subtitle != "" || t.Overline != "":
		minH = 72
	}
	if t.Dense {
		minH -= 8
	}
	minH = pickF(st.MinHeight, minH)
	kids := []w.Widget{}
	if t.Leading != nil {
		kids = append(kids, w.IconTheme{Size: 24, Color: iconC, Child: t.Leading})
	}
	kids = append(kids, w.Expanded{Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Main: w.MainCenter, Children: lines}})
	if t.Trailing != nil {
		kids = append(kids, w.DefaultTextStyle{Style: pickTC(st.TrailingTextStyle, th.Text.LabelSmall, subC), Child: w.IconTheme{Size: 24, Color: iconC, Child: t.Trailing}})
	}
	cross := w.CrossCenter
	if t.ThreeLine {
		cross = w.CrossStart
	}
	body := w.ConstrainedBox{Constraints: geom.Constraints{MinW: 0, MaxW: geom.Inf, MinH: minH, MaxH: geom.Inf},
		Child: w.Padding{Padding: pickE(st.ContentPadding, geom.InsetsLTRB(16, 8, 24, 8)), Child: w.Row{Cross: cross, Spacing: pickF(st.HorizontalGap, 16), Children: kids}}}
	radius := pickF(st.Radius, 0)
	if t.OnTap == nil || t.Disabled {
		if radius > 0 {
			return Surface{Color: bg, Radius: radius, Child: body}
		}
		return w.DecoratedBox{Color: bg, Child: body}
	}
	return InkSurface{OnTap: t.OnTap, Color: bg, ContentColor: pick(st.OverlayColor, titleC), Radius: radius, Child: body}
}

// CheckboxListTile is a ListTile with a trailing checkbox; tapping the row toggles.
type CheckboxListTile struct {
	Title, Subtitle string
	Leading         w.Widget
	Value           bool
	OnChanged       func(bool)
}

func (t CheckboxListTile) Build(w.BuildContext) w.Widget {
	var tap func()
	if t.OnChanged != nil {
		tap = func() { t.OnChanged(!t.Value) }
	}
	return ListTile{Title: t.Title, Subtitle: t.Subtitle, Leading: t.Leading, OnTap: tap, Disabled: t.OnChanged == nil,
		Trailing: Checkbox{Value: t.Value, OnChanged: t.OnChanged}}
}

// RadioListTile is a ListTile with a leading radio button.
type RadioListTile[T comparable] struct {
	Title, Subtitle   string
	Value, GroupValue T
	OnChanged         func(T)
}

func (t RadioListTile[T]) Build(w.BuildContext) w.Widget {
	var tap func()
	if t.OnChanged != nil {
		tap = func() { t.OnChanged(t.Value) }
	}
	return ListTile{Title: t.Title, Subtitle: t.Subtitle, OnTap: tap, Disabled: t.OnChanged == nil,
		Leading: Radio[T]{Value: t.Value, GroupValue: t.GroupValue, OnChanged: t.OnChanged}}
}

// SwitchListTile is a ListTile with a trailing switch.
type SwitchListTile struct {
	Title, Subtitle string
	Leading         w.Widget
	Value           bool
	OnChanged       func(bool)
}

func (t SwitchListTile) Build(w.BuildContext) w.Widget {
	var tap func()
	if t.OnChanged != nil {
		tap = func() { t.OnChanged(!t.Value) }
	}
	return ListTile{Title: t.Title, Subtitle: t.Subtitle, Leading: t.Leading, OnTap: tap, Disabled: t.OnChanged == nil,
		Trailing: Switch{Value: t.Value, OnChanged: t.OnChanged}}
}

// ---------------------------------------------------------------------------
// ExpansionTile

// ExpansionTile is a ListTile that expands to reveal Children.
type ExpansionTile struct {
	Title, Subtitle    string
	Leading            w.Widget
	Children           []w.Widget
	InitiallyExpanded  bool
	OnExpansionChanged func(bool)
	// Style overrides Theme.ExpansionTile.
	Style ExpansionTileTheme
}

func (ExpansionTile) CreateState() w.State { return &expansionState{} }

type expansionState struct {
	w.StateBase
	open bool
	f    *w.Animated
}

func (s *expansionState) InitState() {
	s.open = w.WidgetOf[ExpansionTile](s).InitiallyExpanded
	v := float32(0)
	if s.open {
		v = 1
	}
	s.f = w.NewAnimated(s, 200*time.Millisecond, w.Emphasized, v)
}

func (s *expansionState) Build(ctx w.BuildContext) w.Widget {
	e := w.WidgetOf[ExpansionTile](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.ExpansionTile, e.Style)
	if s.open {
		s.f.Set(1)
	} else {
		s.f.Set(0)
	}
	arrow := iconExpandMore
	col, bg, text := pick(st.CollapsedIconColor, sc.OnSurfaceVariant), st.CollapsedBackgroundColor, st.CollapsedTextColor
	if s.open {
		arrow, col, bg, text = iconExpandLess, pick(st.IconColor, sc.Primary), st.BackgroundColor, st.TextColor
	}
	var children w.Widget = w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: e.Children}
	if st.ChildrenPadding != nil {
		children = w.Padding{Padding: *st.ChildrenPadding, Child: children}
	}
	return w.DecoratedBox{Color: pick(bg, geom.Transparent), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		ListTile{Title: e.Title, Subtitle: e.Subtitle, Leading: e.Leading, Style: ListTileTheme{TextColor: text},
			Trailing: w.Icon{Icon: arrow, Color: col},
			OnTap: func() {
				s.SetState(func() { s.open = !s.open })
				if e.OnExpansionChanged != nil {
					e.OnExpansionChanged(s.open)
				}
			}},
		w.SizeTransition{Factor: s.f.Value(), Child: children},
	}}}
}

// ---------------------------------------------------------------------------
// Badge

// Badge shows a small dot, or a count/label, on the top-right of Child.
type Badge struct {
	Label string // empty = small 6px dot
	Show  *bool  // nil = shown
	Child w.Widget
	// Style overrides Theme.Badge.
	Style BadgeTheme
}

func (b Badge) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	if b.Show != nil && !*b.Show {
		return b.Child
	}
	st := merge(th.Badge, b.Style)
	bg, fg := pick(st.BackgroundColor, s.Error), pick(st.TextColor, s.OnError)
	var dot w.Widget
	if b.Label == "" {
		d := pickF(st.SmallSize, 6)
		dot = w.Positioned{Right: w.At(-d / 2), Top: w.At(-d / 2), Child: w.Container{Width: d, Height: d, Color: bg, Border: &geom.Border{Radius: d / 2}}}
	} else {
		h := pickF(st.LargeSize, 16)
		dot = w.Positioned{Left: w.At(12), Top: w.At(-h / 4), Child: w.Container{Color: bg, Border: &geom.Border{Radius: h / 2},
			Padding: pickE(st.Padding, geom.InsetsHV(4, 0)),
			Child: w.ConstrainedBox{Constraints: geom.Constraints{MinW: h / 2, MaxW: geom.Inf, MinH: h, MaxH: h},
				Child: w.Center{Child: w.Text{Text: b.Label, Style: pickTC(st.TextStyle, th.Text.LabelSmall, fg)}}}}}
	}
	return w.Stack{Children: []w.Widget{b.Child, dot}}
}

// ---------------------------------------------------------------------------
// CircleAvatar

// CircleAvatar is a round avatar with initials or an icon.
type CircleAvatar struct {
	Text   string
	Icon   *vector.Icon
	Radius float32 // default Style / 20
	Color  geom.Color
	// Style overrides Theme.Avatar (Radius and Color above win over it).
	Style CircleAvatarTheme
}

func (a CircleAvatar) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	st := merge(th.Avatar, a.Style)
	r := a.Radius
	if r == 0 {
		r = pickF(st.Radius, 20)
	}
	bg, fg := pick(st.BackgroundColor, s.PrimaryContainer), pick(st.ForegroundColor, s.OnPrimaryContainer)
	if a.Color.A > 0 {
		bg = a.Color
	}
	var child w.Widget
	if a.Icon != nil {
		child = w.Icon{Icon: a.Icon, Size: r, Color: fg}
	} else {
		ts := Styled(th.Text.TitleMedium, fg)
		ts.Size = r * 0.8
		child = w.Text{Text: a.Text, Style: pickT(st.TextStyle, ts), MaxLines: 1}
	}
	return w.Container{Width: 2 * r, Height: 2 * r, Color: bg, Border: &geom.Border{Radius: r}, Alignment: w.Ptr(geom.Center), Child: child}
}

// ---------------------------------------------------------------------------
// Tooltip

// Tooltip shows Message in a small label after hovering Child.
type Tooltip struct {
	Message string
	Child   w.Widget
	// Style overrides Theme.Tooltip.
	Style TooltipTheme
}

func (Tooltip) CreateState() w.State { return &tooltipState{} }

type tooltipState struct {
	w.StateBase
	ticker *w.Ticker
	entry  *w.OverlayEntry
}

func (s *tooltipState) Dispose() { s.hide() }

func (s *tooltipState) hide() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.entry != nil {
		s.entry.Remove()
		s.entry = nil
	}
}

func (s *tooltipState) show() {
	ctx := s.Context()
	ov := w.OverlayOf(ctx)
	ro := ctx.RenderObject()
	if ov == nil || ro == nil || s.entry != nil {
		return
	}
	th := ThemeOf(ctx)
	sc := th.Scheme
	tip := w.WidgetOf[Tooltip](s)
	st := merge(th.Tooltip, tip.Style)
	origin := render.GlobalOrigin(ro)
	size := ro.Base().Size()
	msg := tip.Message
	pad := pickE(st.Padding, geom.InsetsHV(8, 4))
	ts := pickTC(st.TextStyle, th.Text.BodySmall, sc.InverseOnSurface)
	p := text.Layout(msg, ts, text.Options{})
	tw := p.Width + pad.Left + pad.Right
	th2 := p.Height + pad.Top + pad.Bottom
	x := origin.X + size.W/2 - tw/2
	win := ov.Context().RenderObject().Base().Size()
	x = max(4, min(x, win.W-tw-4))
	y := origin.Y + size.H + 4
	if y+th2+4 > win.H {
		y = origin.Y - th2 - 4
	}
	s.entry = &w.OverlayEntry{Builder: func(w.BuildContext) w.Widget {
		return w.IgnorePointer{Ignoring: true, Child: w.Stack{Expand: true, Children: []w.Widget{
			w.Positioned{Left: w.At(x), Top: w.At(y), Child: w.Container{Color: pick(st.Color, sc.InverseSurface), Border: &geom.Border{Radius: pickF(st.Radius, CornerExtraSmall)},
				Padding: pad, Child: w.Text{Text: msg, Style: ts}}},
		}}}
	}}
	ov.Insert(s.entry)
}

func (s *tooltipState) Build(ctx w.BuildContext) w.Widget {
	return w.MouseRegion{
		OnEnter: func(w.PointerEvent) {
			wait := merge(ThemeOf(ctx).Tooltip, w.WidgetOf[Tooltip](s).Style).WaitDuration
			if wait == 0 {
				wait = 500 * time.Millisecond
			}
			if s.ticker == nil {
				s.ticker = ctx.Owner().NewTicker(func(el time.Duration) {
					if el >= wait {
						s.ticker.Stop()
						s.show()
					}
				})
			}
			s.ticker.Start()
		},
		OnExit: func(w.PointerEvent) { s.hide() },
		Child: w.Listener{OnEvent: func(e w.PointerEvent) {
			if e.Kind == render.PointerDown {
				s.hide()
			}
		}, Child: w.WidgetOf[Tooltip](s).Child},
	}
}

// ---------------------------------------------------------------------------
// MaterialBanner

// MaterialBanner is a prominent message at the top of a screen with actions.
type MaterialBanner struct {
	Icon    *vector.Icon
	Content string
	Actions []w.Widget
	// Style overrides Theme.Banner.
	Style BannerTheme
}

func (b MaterialBanner) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	st := merge(th.Banner, b.Style)
	top := []w.Widget{}
	if b.Icon != nil {
		top = append(top, CircleAvatar{Icon: b.Icon, Color: pick(st.LeadingColor, s.Primary), Radius: 20})
	}
	top = append(top, w.Expanded{Child: w.Text{Text: b.Content, Style: pickTC(st.ContentTextStyle, th.Text.BodyMedium, s.OnSurface)}})
	return w.Container{Color: pick(st.BackgroundColor, s.SurfaceContainerLow), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		w.Padding{Padding: pickE(st.Padding, geom.InsetsLTRB(16, 16, 16, 8)), Child: w.Row{Spacing: 16, Cross: w.CrossCenter, Children: top}},
		w.Padding{Padding: geom.InsetsLTRB(8, 0, 8, 8), Child: w.Row{Main: w.MainEnd, Spacing: 8, Children: b.Actions}},
		Divider{Style: DividerTheme{Color: st.DividerColor}},
	}}}
}
