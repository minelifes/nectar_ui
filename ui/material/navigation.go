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
// App bars

// AppBarVariant selects the top app bar size.
type AppBarVariant uint8

const (
	AppBarSmall AppBarVariant = iota
	AppBarCenterAligned
	AppBarMedium
	AppBarLarge
)

// AppBar is the M3 top app bar.
type AppBar struct {
	Title   string
	Variant AppBarVariant
	Leading w.Widget // nil = back button when this page isn't the first route
	Actions []w.Widget
	// ScrolledUnder tints the bar (use when content scrolls beneath it).
	ScrolledUnder bool
	// NoAutoBack disables the automatic back button.
	NoAutoBack bool
	// Style overrides Theme.AppBar.
	Style AppBarTheme
}

func (a AppBar) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	st := merge(th.AppBar, a.Style)
	bg := pick(st.BackgroundColor, s.Surface)
	if a.ScrolledUnder {
		bg = pick(st.ScrolledUnderColor, s.SurfaceContainer)
	}
	fg := pick(st.ForegroundColor, s.OnSurface)
	lead := a.Leading
	if lead == nil && !a.NoAutoBack {
		if nav := w.NavigatorOf(ctx); nav != nil && w.CanPopRoute(ctx) {
			lead = IconButton{Icon: iconArrowBack, Color: fg, OnPressed: func() { nav.Pop(nil) }}
		}
	}
	titleSt := pickTC(st.TitleTextStyle, th.Text.TitleLarge, fg)
	variant := a.Variant
	if variant == AppBarSmall && pickB(st.CenterTitle, false) {
		variant = AppBarCenterAligned // an explicit Variant always wins
	}
	row := []w.Widget{}
	if lead != nil {
		row = append(row, w.IconTheme{Size: 24, Color: fg, Child: lead})
	} else {
		row = append(row, w.SizedBox{Width: 12})
	}
	small := variant == AppBarSmall || variant == AppBarCenterAligned
	if small {
		ta := alignStart
		if variant == AppBarCenterAligned {
			ta = alignCenter
		}
		row = append(row, w.Expanded{Child: w.Padding{Padding: geom.InsetsHV(4, 0),
			Child: w.Text{Text: a.Title, Style: titleSt, MaxLines: 1, Ellipsis: true, Align: ta}}})
	} else {
		row = append(row, w.Spacer{})
	}
	row = append(row, w.IconTheme{Size: 24, Color: pick(st.ActionsIconColor, s.OnSurfaceVariant), Child: w.Row{ShrinkMain: true, Cross: w.CrossCenter, Children: a.Actions}})
	row = append(row, w.SizedBox{Width: 4})
	barH := pickF(st.Height, 64)
	bar := w.SizedBox{Height: barH, Child: w.Padding{Padding: geom.InsetsHV(4, 0), Child: w.Row{Cross: w.CrossCenter, Children: row}}}
	elev := pickI(st.Elevation, 0)
	box := func(child w.Widget) w.Widget {
		if elev > 0 {
			return Surface{Color: bg, Elevation: elev, ShadowColor: st.ShadowColor, Child: child}
		}
		return w.DecoratedBox{Color: bg, Child: child}
	}
	if small {
		return box(bar)
	}
	big := pickTC(st.TitleTextStyle, th.Text.HeadlineSmall, fg)
	h, pb := barH+48, float32(24)
	if variant == AppBarLarge {
		big, h, pb = pickTC(st.TitleTextStyle, th.Text.HeadlineMedium, fg), barH+88, 28
	}
	return box(w.SizedBox{Height: h, Child: w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		bar,
		w.Expanded{Child: w.Align{Alignment: geom.BottomLeft, Child: w.Padding{Padding: geom.InsetsLTRB(16, 0, 16, pb),
			Child: w.Text{Text: a.Title, Style: big, MaxLines: 1, Ellipsis: true}}}},
	}}})
}

// ---------------------------------------------------------------------------
// Navigation bar / rail / drawer

// NavigationDestination is one destination of a NavigationBar / Rail /
// Drawer.
type NavigationDestination struct {
	Icon         *vector.Icon
	SelectedIcon *vector.Icon
	Label        string
	Badge        string // "" = none; "•" = small dot
}

func (d NavigationDestination) icon(sel bool) *vector.Icon {
	if sel && d.SelectedIcon != nil {
		return d.SelectedIcon
	}
	return d.Icon
}

func badged(ic w.Widget, badge string) w.Widget {
	switch badge {
	case "":
		return ic
	case "•":
		return Badge{Child: ic}
	}
	return Badge{Label: badge, Child: ic}
}

// indicator paints the selection pill, animated by t (0..1).
func indicator(c *render.Canvas, ctr geom.Offset, wdt, h, t float32, col geom.Color) {
	if t <= 0 {
		return
	}
	cw := wdt * (0.5 + 0.5*t)
	c.FillRoundRect(geom.Rect{X: ctr.X - cw/2, Y: ctr.Y - h/2, W: cw, H: h}, h/2, col.WithAlpha(col.A*t))
}

// NavigationBar is the bottom bar for 3–5 top-level destinations.
type NavigationBar struct {
	Destinations []NavigationDestination
	Selected     int
	OnSelected   func(int)
	HideLabels   bool
	// Style overrides Theme.NavigationBar.
	Style NavigationBarTheme
}

func (NavigationBar) CreateState() w.State { return &navBarState{} }

type navBarState struct {
	w.StateBase
	anims []*w.Animated
}

func (s *navBarState) sync(n, sel int) {
	for len(s.anims) < n {
		v := float32(0)
		if len(s.anims) == sel {
			v = 1
		}
		s.anims = append(s.anims, w.NewAnimated(s, 200*time.Millisecond, w.Emphasized, v))
	}
	for i, a := range s.anims {
		if i == sel {
			a.Set(1)
		} else {
			a.Set(0)
		}
	}
}

func (s *navBarState) Build(ctx w.BuildContext) w.Widget {
	nb := w.WidgetOf[NavigationBar](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.NavigationBar, nb.Style)
	s.sync(len(nb.Destinations), nb.Selected)
	items := make([]w.Widget, len(nb.Destinations))
	for i, d := range nb.Destinations {
		i, d := i, d
		sel := i == nb.Selected
		ic, lc := pick(st.IconColor, sc.OnSurfaceVariant), pick(st.LabelColor, sc.OnSurfaceVariant)
		if sel {
			ic, lc = pick(st.SelectedIconColor, sc.OnSecondaryContainer), pick(st.SelectedLabelColor, sc.OnSurface)
		}
		a := s.anims[i]
		col := pick(st.IndicatorColor, sc.SecondaryContainer)
		iconW := w.CustomPaint{Size: geom.Sz(64, 32),
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				indicator(c, geom.Pt(o.X+size.W/2, o.Y+size.H/2), 64, 32, a.Value(), col)
			},
			Child: w.Center{Child: badged(w.Icon{Icon: d.icon(sel), Size: 24, Color: ic}, d.Badge)}}
		kids := []w.Widget{iconW}
		if !nb.HideLabels {
			kids = append(kids, w.SizedBox{Height: 4}, w.Text{Text: d.Label, Style: pickTC(st.LabelTextStyle, th.Text.LabelMedium, lc), MaxLines: 1})
		}
		var tap func()
		if nb.OnSelected != nil {
			tap = func() { nb.OnSelected(i) }
		}
		items[i] = w.Expanded{Child: InkSurface{OnTap: tap, ContentColor: pick(st.OverlayColor, sc.OnSurface), NoFocus: false,
			Child: w.Column{Main: w.MainCenter, Cross: w.CrossCenter, Children: kids}}}
	}
	return w.Container{Color: pick(st.BackgroundColor, sc.SurfaceContainer), Height: pickF(st.Height, 80),
		Child: w.Padding{Padding: geom.InsetsHV(8, 0), Child: w.Row{Cross: w.CrossStretch, Spacing: 8, Children: items}}}
}

// NavigationRail is the side navigation for medium screens.
type NavigationRail struct {
	Destinations []NavigationDestination
	Selected     int
	OnSelected   func(int)
	Leading      w.Widget // e.g. a FAB or menu button
	Trailing     w.Widget
	// Style overrides Theme.NavigationRail.
	Style NavigationRailTheme
}

func (NavigationRail) CreateState() w.State { return &navRailState{} }

type navRailState struct{ navBarState }

func (s *navRailState) Build(ctx w.BuildContext) w.Widget {
	nr := w.WidgetOf[NavigationRail](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.NavigationRail, nr.Style)
	width := pickF(st.Width, 80)
	s.sync(len(nr.Destinations), nr.Selected)
	kids := []w.Widget{w.SizedBox{Height: 12}}
	if nr.Leading != nil {
		kids = append(kids, nr.Leading, w.SizedBox{Height: 28})
	}
	for i, d := range nr.Destinations {
		i, d := i, d
		sel := i == nr.Selected
		ic, lc := pick(st.IconColor, sc.OnSurfaceVariant), pick(st.LabelColor, sc.OnSurfaceVariant)
		if sel {
			ic, lc = pick(st.SelectedIconColor, sc.OnSecondaryContainer), pick(st.SelectedLabelColor, sc.OnSurface)
		}
		a := s.anims[i]
		col := pick(st.IndicatorColor, sc.SecondaryContainer)
		var tap func()
		if nr.OnSelected != nil {
			tap = func() { nr.OnSelected(i) }
		}
		kids = append(kids, InkSurface{OnTap: tap, ContentColor: pick(st.OverlayColor, sc.OnSurface), Radius: CornerLarge,
			Child: w.SizedBox{Width: width, Height: 56, Child: w.Column{Main: w.MainCenter, Cross: w.CrossCenter, Children: []w.Widget{
				w.CustomPaint{Size: geom.Sz(56, 32),
					Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
						indicator(c, geom.Pt(o.X+size.W/2, o.Y+size.H/2), 56, 32, a.Value(), col)
					},
					Child: w.Center{Child: badged(w.Icon{Icon: d.icon(sel), Size: 24, Color: ic}, d.Badge)}},
				w.SizedBox{Height: 4},
				w.Text{Text: d.Label, Style: pickTC(st.LabelTextStyle, th.Text.LabelMedium, lc), MaxLines: 1},
			}}}}, w.SizedBox{Height: 12})
	}
	if nr.Trailing != nil {
		kids = append(kids, w.Spacer{}, nr.Trailing, w.SizedBox{Height: 16})
	}
	return w.Container{Color: pick(st.BackgroundColor, sc.Surface), Width: width, Child: w.Column{Cross: w.CrossCenter, Children: kids}}
}

// NavigationDrawer lists destinations in a side sheet. Headlines (Label
// with nil Icon) render as section titles.
type NavigationDrawer struct {
	Header       w.Widget
	Destinations []NavigationDestination
	Selected     int
	OnSelected   func(int)
	// Style overrides Theme.NavigationDrawer.
	Style NavigationDrawerTheme
}

func (d NavigationDrawer) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.NavigationDrawer, d.Style)
	radius, itemH := pickF(st.Radius, CornerLarge), pickF(st.ItemHeight, 56)
	kids := []w.Widget{w.SizedBox{Height: 12}}
	if d.Header != nil {
		kids = append(kids, w.Padding{Padding: geom.InsetsLTRB(16, 16, 16, 12), Child: d.Header})
	}
	idx := 0
	for _, dest := range d.Destinations {
		if dest.Icon == nil {
			kids = append(kids, w.Padding{Padding: geom.InsetsLTRB(28, 18, 16, 18),
				Child: w.Text{Text: dest.Label, Style: Styled(th.Text.TitleSmall, pick(st.HeadlineColor, sc.OnSurfaceVariant))}})
			continue
		}
		i := idx
		idx++
		sel := i == d.Selected
		bg, fg, ic := geom.Transparent, pick(st.LabelColor, sc.OnSurfaceVariant), pick(st.IconColor, sc.OnSurfaceVariant)
		if sel {
			bg, fg, ic = pick(st.IndicatorColor, sc.SecondaryContainer), pick(st.SelectedLabelColor, sc.OnSecondaryContainer), pick(st.SelectedIconColor, sc.OnSecondaryContainer)
		}
		var tap func()
		if d.OnSelected != nil {
			tap = func() { d.OnSelected(i) }
		}
		lst := pickTC(st.LabelTextStyle, th.Text.LabelLarge, fg)
		row := []w.Widget{w.Icon{Icon: dest.icon(sel), Size: 24, Color: ic},
			w.Expanded{Child: w.Text{Text: dest.Label, Style: lst, MaxLines: 1, Ellipsis: true}}}
		if dest.Badge != "" {
			row = append(row, w.Text{Text: dest.Badge, Style: lst})
		}
		kids = append(kids, w.Padding{Padding: geom.InsetsHV(12, 0), Child: InkSurface{OnTap: tap, Color: bg, ContentColor: fg, Radius: CornerFull,
			Child: w.SizedBox{Height: itemH, Child: w.Padding{Padding: geom.InsetsLTRB(16, 0, 24, 0),
				Child: w.Row{Cross: w.CrossCenter, Spacing: 12, Children: row}}}}})
	}
	return w.CustomPaint{
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			// Rounded on the trailing edge only: extend the shape leftwards.
			c.FillRoundRect(geom.Rect{X: o.X - radius, Y: o.Y, W: size.W + radius, H: size.H}, radius, pick(st.BackgroundColor, sc.SurfaceContainerLow))
		},
		Child: w.SizedBox{Width: pickF(st.Width, 360), Child: w.ListView{Children: kids}},
	}
}

// ---------------------------------------------------------------------------
// Tabs

// Tab is one tab of a TabBar.
type Tab struct {
	Text string
	Icon *vector.Icon
}

// TabBar switches between views. Secondary tabs use a full-width, thinner
// indicator.
type TabBar struct {
	Tabs      []Tab
	Selected  int
	OnChanged func(int)
	Secondary bool
	// Style overrides Theme.TabBar.
	Style TabBarTheme
}

func (TabBar) CreateState() w.State { return &tabBarState{} }

type tabBarState struct {
	w.StateBase
	pos   *w.Animated
	width float32
}

func (s *tabBarState) InitState() {
	s.pos = w.NewAnimated(s, 250*time.Millisecond, w.Emphasized, float32(w.WidgetOf[TabBar](s).Selected))
}

func (s *tabBarState) Build(ctx w.BuildContext) w.Widget {
	tb := w.WidgetOf[TabBar](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.TabBar, tb.Style)
	s.pos.Set(float32(tb.Selected))
	hasIcon := false
	for _, t := range tb.Tabs {
		if t.Icon != nil {
			hasIcon = true
		}
	}
	h := float32(48)
	if hasIcon && !tb.Secondary {
		h = 64
	}
	h = pickF(st.Height, h)
	labelW := make([]float32, len(tb.Tabs))
	tabs := make([]w.Widget, len(tb.Tabs))
	for i, t := range tb.Tabs {
		i := i
		sel := i == tb.Selected
		fg := pick(st.UnselectedLabelColor, sc.OnSurfaceVariant)
		if sel {
			fg = sc.Primary
			if tb.Secondary {
				fg = sc.OnSurface
			}
			fg = pick(st.LabelColor, fg)
		}
		ls := pickTC(st.LabelStyle, th.Text.TitleSmall, fg)
		labelW[i] = text.Layout(t.Text, ls, text.Options{}).Width
		kids := []w.Widget{}
		if t.Icon != nil {
			kids = append(kids, w.Icon{Icon: t.Icon, Size: 24, Color: fg})
		}
		if t.Text != "" {
			kids = append(kids, w.Text{Text: t.Text, Style: ls, MaxLines: 1})
		}
		var content w.Widget = w.Column{Main: w.MainCenter, Cross: w.CrossCenter, Spacing: 2, Children: kids}
		if tb.Secondary || !hasIcon {
			content = w.Row{Main: w.MainCenter, Cross: w.CrossCenter, Spacing: 8, Children: kids}
		}
		var tap func()
		if tb.OnChanged != nil {
			tap = func() { tb.OnChanged(i) }
		}
		tabs[i] = w.Expanded{Child: InkSurface{OnTap: tap, ContentColor: pick(st.OverlayColor, fg), Child: w.SizedBox{Height: h, Child: content}}}
	}
	n := len(tb.Tabs)
	secondary := tb.Secondary
	line, prim := pick(st.DividerColor, sc.SurfaceVariant), pick(st.IndicatorColor, sc.Primary)
	ih := pickF(st.IndicatorHeight, 0)
	return w.CustomPaint{
		ForegroundPainter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			c.FillRect(geom.Rect{X: o.X, Y: o.Y + size.H - 1, W: size.W, H: 1}, line)
			if n == 0 {
				return
			}
			tw := size.W / float32(n)
			p := s.pos.Value()
			i0 := min(int(p), n-1)
			i1 := min(i0+1, n-1)
			f := p - float32(i0)
			if secondary {
				hh := ih
				if hh == 0 {
					hh = 2
				}
				c.FillRect(geom.Rect{X: o.X + tw*p, Y: o.Y + size.H - hh, W: tw, H: hh}, prim)
				return
			}
			// Primary indicator: 3px, rounded on top, as wide as the label.
			lw := max(widgetsLerp(labelW[i0], labelW[i1], f), 24)
			cx := o.X + tw*p + tw/2
			c.PushClip(geom.RectFrom(o, size))
			hh := ih
			if hh == 0 {
				hh = 3
			}
			c.FillRoundRect(geom.Rect{X: cx - lw/2, Y: o.Y + size.H - hh, W: lw, H: 2 * hh}, hh, prim)
			c.PopClip()
		},
		Child: w.ClipRect{Child: w.Row{Children: tabs}},
	}
}

// TabBarView shows the child for the selected tab.
type TabBarView struct {
	Selected int
	Children []w.Widget
}

func (v TabBarView) Build(w.BuildContext) w.Widget {
	if v.Selected < 0 || v.Selected >= len(v.Children) {
		return w.SizedBox{}
	}
	return w.KeyedSubtree{ID: v.Selected, Child: v.Children[v.Selected]}
}

// ---------------------------------------------------------------------------
// Bottom app bar

// BottomAppBar holds actions at the bottom, with an optional FAB at the end.
type BottomAppBar struct {
	Actions []w.Widget
	FAB     w.Widget
	// Style overrides Theme.BottomAppBar.
	Style BottomAppBarTheme
}

func (b BottomAppBar) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.BottomAppBar, b.Style)
	kids := []w.Widget{w.Row{ShrinkMain: true, Cross: w.CrossCenter, Spacing: 4, Children: b.Actions}, w.Spacer{}}
	if b.FAB != nil {
		kids = append(kids, b.FAB)
	}
	return w.Container{Color: pick(st.Color, sc.SurfaceContainer), Height: pickF(st.Height, 80), Padding: pickE(st.Padding, geom.InsetsLTRB(4, 12, 16, 12)),
		Child: w.IconTheme{Size: 24, Color: pick(st.IconColor, sc.OnSurfaceVariant), Child: w.Row{Cross: w.CrossCenter, Children: kids}}}
}

// ---------------------------------------------------------------------------
// Scaffold

// Scaffold lays out a screen: app bar, body, FAB, bottom bar, side rail
// and a modal drawer (open it with ScaffoldOf(ctx).OpenDrawer()).
type Scaffold struct {
	AppBar     w.Widget
	Body       w.Widget
	FAB        w.Widget
	BottomBar  w.Widget // NavigationBar or BottomAppBar
	Rail       w.Widget // NavigationRail, left of the body
	Drawer     w.Widget // NavigationDrawer, shown modally
	Background *geom.Color
}

func (Scaffold) CreateState() w.State { return &ScaffoldState{} }

// ScaffoldState controls a Scaffold (drawer).
type ScaffoldState struct {
	w.StateBase
	open *w.Animated
}

func (s *ScaffoldState) InitState() {
	s.open = w.NewAnimated(s, 250*time.Millisecond, w.Emphasized, 0)
}

// OpenDrawer slides the drawer in.
func (s *ScaffoldState) OpenDrawer() { s.SetState(func() { s.open.Set(1) }) }

// CloseDrawer slides it out.
func (s *ScaffoldState) CloseDrawer() { s.SetState(func() { s.open.Set(0) }) }

// ScaffoldOf returns the nearest Scaffold's state (nil if none).
func ScaffoldOf(ctx w.BuildContext) *ScaffoldState {
	if sc, ok := w.Find[scaffoldScope](ctx); ok {
		return sc.state
	}
	return nil
}

type scaffoldScope struct {
	state *ScaffoldState
	child w.Widget
}

func (s scaffoldScope) ChildWidget() w.Widget { return s.child }
func (s scaffoldScope) UpdateShouldNotify(old w.Widget) bool {
	return old.(scaffoldScope).state != s.state
}

func (s *ScaffoldState) Build(ctx w.BuildContext) w.Widget {
	sf := w.WidgetOf[Scaffold](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	bg := pick(th.ScaffoldBackground, sc.Surface)
	if sf.Background != nil {
		bg = *sf.Background
	}
	col := []w.Widget{}
	if sf.AppBar != nil {
		col = append(col, sf.AppBar)
	}
	body := sf.Body
	if body == nil {
		body = w.SizedBox{}
	}
	if sf.Rail != nil {
		body = w.Row{Cross: w.CrossStretch, Children: []w.Widget{sf.Rail, w.Expanded{Child: body}}}
	}
	col = append(col, w.Expanded{Child: body})
	if sf.BottomBar != nil {
		col = append(col, sf.BottomBar)
	}
	layers := []w.Widget{w.PositionedFill(w.DecoratedBox{Color: bg, Child: w.Column{Cross: w.CrossStretch, Children: col}})}
	if sf.FAB != nil {
		bottom := float32(16)
		if sf.BottomBar != nil {
			bottom += 80
		}
		layers = append(layers, w.Positioned{Right: w.At(16), Bottom: w.At(bottom), Child: sf.FAB})
	}
	if sf.Drawer != nil {
		t := s.open.Value()
		if t > 0 {
			scrim := sc.Scrim.WithAlpha(0.32 * t)
			layers = append(layers,
				w.PositionedFill(w.GestureDetector{OnTap: s.CloseDrawer, Child: w.AbsorbPointer{Child: w.DecoratedBox{Color: scrim}}}),
				w.Positioned{Left: w.At(0), Top: w.At(0), Bottom: w.At(0),
					Child: w.Translate{Fraction: geom.Pt(t-1, 0), Child: w.AbsorbPointer{Child: w.Focus{Autofocus: true, OnKey: func(e w.KeyEvent) bool {
						if e.Key == w.KeyEscape {
							s.CloseDrawer()
							return true
						}
						return false
					}, Child: sf.Drawer}}}})
		}
	}
	return scaffoldScope{state: s, child: w.Stack{Expand: true, Children: layers}}
}
