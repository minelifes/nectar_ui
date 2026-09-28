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
}

func (c Card) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
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
	child := c.Child
	if !c.Padding.IsZero() {
		child = w.Padding{Padding: c.Padding, Child: child}
	}
	if c.OnTap == nil {
		return Surface{Color: bg, Radius: CornerMedium, BorderColor: border, BorderWidth: bw, Elevation: elev, Child: child}
	}
	return InkSurface{OnTap: c.OnTap, Color: bg, ContentColor: s.OnSurface, BorderColor: border, BorderWidth: bw,
		Radius: CornerMedium, Elevation: elev, RaiseOnHover: c.Variant == CardElevated, Child: child}
}

// ---------------------------------------------------------------------------
// Dividers

// Divider is a thin horizontal line.
type Divider struct {
	Indent, EndIndent float32
	Height            float32 // total space; default 1 (line is always 1px)
}

func (d Divider) Build(ctx w.BuildContext) w.Widget {
	col := ThemeOf(ctx).Scheme.OutlineVariant
	h := max(d.Height, 1)
	return w.CustomPaint{Size: geom.Sz(geom.Inf, h), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		c.FillRect(geom.Rect{X: o.X + d.Indent, Y: o.Y + (s.H-1)/2, W: s.W - d.Indent - d.EndIndent, H: 1}, col)
	}}
}

// VerticalDivider is a thin vertical line.
type VerticalDivider struct {
	Indent, EndIndent float32
	Width             float32
}

func (d VerticalDivider) Build(ctx w.BuildContext) w.Widget {
	col := ThemeOf(ctx).Scheme.OutlineVariant
	wd := max(d.Width, 1)
	return w.CustomPaint{Size: geom.Sz(wd, geom.Inf), Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		c.FillRect(geom.Rect{X: o.X + (s.W-1)/2, Y: o.Y + d.Indent, W: 1, H: s.H - d.Indent - d.EndIndent}, col)
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
}

func (t ListTile) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	titleC, subC, iconC := s.OnSurface, s.OnSurfaceVariant, s.OnSurfaceVariant
	bg := geom.Transparent
	if t.Selected {
		bg, titleC, iconC = s.SecondaryContainer, s.OnSecondaryContainer, s.OnSecondaryContainer
	}
	if t.Disabled {
		dc, _ := disabledColors(s)
		titleC, subC, iconC = dc, dc, dc
	}
	lines := []w.Widget{}
	if t.Overline != "" {
		lines = append(lines, w.Text{Text: t.Overline, Style: Styled(th.Text.LabelSmall, subC), MaxLines: 1, Ellipsis: true})
	}
	if t.TitleWidget != nil {
		lines = append(lines, t.TitleWidget)
	} else if t.Title != "" {
		st := th.Text.BodyLarge
		if t.Dense {
			st = th.Text.BodyMedium
		}
		lines = append(lines, w.Text{Text: t.Title, Style: Styled(st, titleC), MaxLines: 1, Ellipsis: true})
	}
	if t.Subtitle != "" {
		ml := 1
		if t.ThreeLine {
			ml = 2
		}
		lines = append(lines, w.Text{Text: t.Subtitle, Style: Styled(th.Text.BodyMedium, subC), MaxLines: ml, Ellipsis: true})
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
	kids := []w.Widget{}
	if t.Leading != nil {
		kids = append(kids, w.IconTheme{Size: 24, Color: iconC, Child: t.Leading})
	}
	kids = append(kids, w.Expanded{Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Main: w.MainCenter, Children: lines}})
	if t.Trailing != nil {
		kids = append(kids, w.DefaultTextStyle{Style: Styled(th.Text.LabelSmall, subC), Child: w.IconTheme{Size: 24, Color: iconC, Child: t.Trailing}})
	}
	cross := w.CrossCenter
	if t.ThreeLine {
		cross = w.CrossStart
	}
	body := w.ConstrainedBox{Constraints: geom.Constraints{MinW: 0, MaxW: geom.Inf, MinH: minH, MaxH: geom.Inf},
		Child: w.Padding{Padding: geom.InsetsLTRB(16, 8, 24, 8), Child: w.Row{Cross: cross, Spacing: 16, Children: kids}}}
	if t.OnTap == nil || t.Disabled {
		return w.DecoratedBox{Color: bg, Child: body}
	}
	return InkSurface{OnTap: t.OnTap, Color: bg, ContentColor: titleC, Child: body}
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
	sc := ThemeOf(ctx).Scheme
	if s.open {
		s.f.Set(1)
	} else {
		s.f.Set(0)
	}
	arrow := iconExpandMore
	col := sc.OnSurfaceVariant
	if s.open {
		arrow, col = iconExpandLess, sc.Primary
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		ListTile{Title: e.Title, Subtitle: e.Subtitle, Leading: e.Leading,
			Trailing: w.Icon{Icon: arrow, Color: col},
			OnTap: func() {
				s.SetState(func() { s.open = !s.open })
				if e.OnExpansionChanged != nil {
					e.OnExpansionChanged(s.open)
				}
			}},
		w.SizeTransition{Factor: s.f.Value(), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: e.Children}},
	}}
}

// ---------------------------------------------------------------------------
// Badge

// Badge shows a small dot, or a count/label, on the top-right of Child.
type Badge struct {
	Label string // empty = small 6px dot
	Show  *bool  // nil = shown
	Child w.Widget
}

func (b Badge) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	if b.Show != nil && !*b.Show {
		return b.Child
	}
	var dot w.Widget
	if b.Label == "" {
		dot = w.Positioned{Right: w.At(-3), Top: w.At(-3), Child: w.Container{Width: 6, Height: 6, Color: s.Error, Border: &geom.Border{Radius: 3}}}
	} else {
		dot = w.Positioned{Left: w.At(12), Top: w.At(-4), Child: w.Container{Color: s.Error, Border: &geom.Border{Radius: 8},
			Padding: geom.InsetsHV(4, 0),
			Child: w.ConstrainedBox{Constraints: geom.Constraints{MinW: 8, MaxW: geom.Inf, MinH: 16, MaxH: 16},
				Child: w.Center{Child: w.Text{Text: b.Label, Style: Styled(th.Text.LabelSmall, s.OnError)}}}}}
	}
	return w.Stack{Children: []w.Widget{b.Child, dot}}
}

// ---------------------------------------------------------------------------
// CircleAvatar

// CircleAvatar is a round avatar with initials or an icon.
type CircleAvatar struct {
	Text   string
	Icon   *vector.Icon
	Radius float32 // default 20
	Color  geom.Color
}

func (a CircleAvatar) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	r := a.Radius
	if r == 0 {
		r = 20
	}
	bg, fg := s.PrimaryContainer, s.OnPrimaryContainer
	if a.Color.A > 0 {
		bg = a.Color
	}
	var child w.Widget
	if a.Icon != nil {
		child = w.Icon{Icon: a.Icon, Size: r, Color: fg}
	} else {
		st := Styled(th.Text.TitleMedium, fg)
		st.Size = r * 0.8
		child = w.Text{Text: a.Text, Style: st, MaxLines: 1}
	}
	return w.Container{Width: 2 * r, Height: 2 * r, Color: bg, Border: &geom.Border{Radius: r}, Alignment: w.Ptr(geom.Center), Child: child}
}

// ---------------------------------------------------------------------------
// Tooltip

// Tooltip shows Message in a small label after hovering Child.
type Tooltip struct {
	Message string
	Child   w.Widget
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
	origin := render.GlobalOrigin(ro)
	size := ro.Base().Size()
	msg := w.WidgetOf[Tooltip](s).Message
	p := text.Layout(msg, th.Text.BodySmall, text.Options{})
	tw := p.Width + 16
	x := origin.X + size.W/2 - tw/2
	win := ov.Context().RenderObject().Base().Size()
	x = max(4, min(x, win.W-tw-4))
	y := origin.Y + size.H + 4
	if y+28 > win.H {
		y = origin.Y - 28
	}
	s.entry = &w.OverlayEntry{Builder: func(w.BuildContext) w.Widget {
		return w.IgnorePointer{Ignoring: true, Child: w.Stack{Expand: true, Children: []w.Widget{
			w.Positioned{Left: w.At(x), Top: w.At(y), Child: w.Container{Color: sc.InverseSurface, Border: &geom.Border{Radius: CornerExtraSmall},
				Padding: geom.InsetsHV(8, 4), Child: w.Text{Text: msg, Style: Styled(th.Text.BodySmall, sc.InverseOnSurface)}}},
		}}}
	}}
	ov.Insert(s.entry)
}

func (s *tooltipState) Build(ctx w.BuildContext) w.Widget {
	return w.MouseRegion{
		OnEnter: func(w.PointerEvent) {
			if s.ticker == nil {
				s.ticker = ctx.Owner().NewTicker(func(el time.Duration) {
					if el >= 500*time.Millisecond {
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
}

func (b MaterialBanner) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	top := []w.Widget{}
	if b.Icon != nil {
		top = append(top, CircleAvatar{Icon: b.Icon, Color: s.Primary, Radius: 20})
	}
	top = append(top, w.Expanded{Child: w.Text{Text: b.Content, Style: Styled(th.Text.BodyMedium, s.OnSurface)}})
	return w.Container{Color: s.SurfaceContainerLow, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(16, 16, 16, 8), Child: w.Row{Spacing: 16, Cross: w.CrossCenter, Children: top}},
		w.Padding{Padding: geom.InsetsLTRB(8, 0, 8, 8), Child: w.Row{Main: w.MainEnd, Spacing: 8, Children: b.Actions}},
		Divider{},
	}}}
}
