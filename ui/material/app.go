package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// App is the root of a Material app: theme, overlay, navigator and
// snackbar messenger. Put your first page in Home.
type App struct {
	Theme     Theme  // zero value = baseline light theme
	DarkTheme *Theme // used when Dark is set; nil = Theme.WithDark(true)
	Dark      bool
	Home      w.Widget
}

func (a App) Build(w.BuildContext) w.Widget {
	th := a.Theme
	if th.Scheme == (ColorScheme{}) {
		th = th.WithDark(false) // baseline scheme, component themes kept
	}
	if a.Dark {
		if a.DarkTheme != nil {
			th = *a.DarkTheme
		} else {
			th = th.WithDark(true) // keeps the component themes
		}
	}
	home := a.Home
	return ThemeScope{Theme: th, Child: w.DecoratedBox{Color: pick(th.ScaffoldBackground, th.Scheme.Surface), Child: w.Overlay{
		Child: messenger{Child: w.Navigator{Home: home}},
	}}}
}

// Pop closes the top route (dialog, sheet, page) with a result.
func Pop(ctx w.BuildContext, result any) {
	if nav := w.NavigatorOf(ctx); nav != nil {
		nav.Pop(result)
	}
}

// Push shows a new full-screen page with the M3 fade-through transition.
func Push(ctx w.BuildContext, page func(ctx w.BuildContext) w.Widget) {
	nav := w.NavigatorOf(ctx)
	if nav == nil {
		return
	}
	nav.Push(&w.Route{Opaque: true, Duration: 300 * time.Millisecond, Builder: func(ctx w.BuildContext) w.Widget {
		th := ThemeOf(ctx)
		return w.DecoratedBox{Color: pick(th.ScaffoldBackground, th.Scheme.Surface), Child: page(ctx)}
	}, Transition: func(child w.Widget, t float32) w.Widget {
		if t >= 1 {
			return child
		}
		return w.Opacity{Opacity: t, Child: w.Translate{Offset: geom.Pt(0, (1-t)*32), Child: child}}
	}})
}

// ---------------------------------------------------------------------------
// Snackbar messenger

// SnackBar is a brief message at the bottom of the screen.
type SnackBar struct {
	Message     string
	ActionLabel string
	OnAction    func()
	ShowClose   bool
	Duration    time.Duration // default 4s
}

// ShowSnackBar displays sb (replacing the current one).
func ShowSnackBar(ctx w.BuildContext, sb SnackBar) {
	if sc, ok := w.Find[messengerScope](ctx); ok {
		sc.state.show(sb)
	}
}

type messenger struct{ Child w.Widget }

func (messenger) CreateState() w.State { return &messengerState{} }

type messengerState struct {
	w.StateBase
	current *SnackBar
	entry   *w.OverlayEntry
	anim    *w.AnimationController
	gen     int
}

func (s *messengerState) InitState() {
	s.anim = w.NewAnimationController(s, 250*time.Millisecond)
	s.anim.Curve = w.EmphasizedDecelerate
	s.anim.AddListener(func() {
		if s.entry != nil {
			s.entry.MarkNeedsBuild()
		}
	})
	s.anim.OnStatus = func(st w.AnimationStatus) {
		if st == w.Dismissed && s.entry != nil {
			s.entry.Remove()
			s.entry, s.current = nil, nil
		}
	}
}

func (s *messengerState) show(sb SnackBar) {
	if sb.Duration == 0 {
		sb.Duration = 4 * time.Second
	}
	s.current = &sb
	s.gen++
	gen := s.gen
	if s.entry == nil {
		ov := w.OverlayOf(s.Context())
		if ov == nil {
			return
		}
		s.entry = &w.OverlayEntry{Builder: s.buildBar}
		ov.Insert(s.entry)
	} else {
		s.entry.MarkNeedsBuild()
	}
	s.anim.Forward()
	time.AfterFunc(sb.Duration, func() {
		s.Post(func() {
			if s.gen == gen {
				s.hide()
			}
		})
	})
}

func (s *messengerState) hide() { s.anim.Reverse() }

func (s *messengerState) buildBar(ctx w.BuildContext) w.Widget {
	sb := s.current
	if sb == nil {
		return w.SizedBox{}
	}
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := th.SnackBar
	action := pick(st.ActionTextColor, sc.InversePrimary)
	kids := []w.Widget{w.Expanded{Child: w.Padding{Padding: geom.InsetsHV(0, 14),
		Child: w.Text{Text: sb.Message, Style: pickTC(st.ContentTextStyle, th.Text.BodyMedium, sc.InverseOnSurface)}}}}
	if sb.ActionLabel != "" {
		act := sb.OnAction
		kids = append(kids, InkSurface{OnTap: func() {
			if act != nil {
				act()
			}
			s.hide()
		}, ContentColor: action, Radius: CornerFull, Child: w.Padding{Padding: geom.InsetsHV(12, 10),
			Child: w.Text{Text: sb.ActionLabel, Style: Styled(th.Text.LabelLarge, action)}}})
	}
	if sb.ShowClose {
		kids = append(kids, IconButton{Icon: iconClose, Color: pick(st.CloseIconColor, sc.InverseOnSurface), OnPressed: s.hide})
	}
	t := s.anim.Value()
	bar := Surface{Color: pick(st.BackgroundColor, sc.InverseSurface), Radius: pickF(st.Radius, CornerExtraSmall), Elevation: pickI(st.Elevation, 3),
		Child: w.Padding{Padding: geom.InsetsLTRB(16, 0, 8, 0), Child: w.Row{Cross: w.CrossCenter, Spacing: 8, Children: kids}}}
	return w.Stack{Expand: true, Children: []w.Widget{
		w.Positioned{Left: w.At(16), Right: w.At(16), Bottom: w.At(16), Child: w.Align{Alignment: geom.BottomCenter,
			Child: w.ConstrainedBox{Constraints: geom.Constraints{MaxW: pickF(st.MaxWidth, 640), MaxH: geom.Inf},
				Child: w.Opacity{Opacity: t, Child: w.Translate{Offset: geom.Pt(0, (1-t)*24), Child: bar}}}}},
	}}
}

func (s *messengerState) Build(w.BuildContext) w.Widget {
	return messengerScope{state: s, child: w.WidgetOf[messenger](s).Child}
}

type messengerScope struct {
	state *messengerState
	child w.Widget
}

func (m messengerScope) ChildWidget() w.Widget { return m.child }
func (m messengerScope) UpdateShouldNotify(old w.Widget) bool {
	return old.(messengerScope).state != m.state
}
