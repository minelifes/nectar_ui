package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// RichTooltip is the card a HoverCard shows: a title, supporting text (or
// any Content) and actions. Use it directly to show one inline.
type RichTooltip struct {
	Title   string
	Text    string
	Content w.Widget // replaces Text
	Actions []ToastAction
	// Style overrides Theme.HoverCard.
	Style HoverCardTheme
}

func (r RichTooltip) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.HoverCard, r.Style)
	kids := []w.Widget{}
	if r.Title != "" {
		kids = append(kids, w.Text{Text: r.Title, Style: pickTC(st.TitleStyle, th.Text.TitleSmall, sc.OnSurfaceVariant)})
	}
	switch {
	case r.Content != nil:
		kids = append(kids, r.Content)
	case r.Text != "":
		kids = append(kids, w.Text{Text: r.Text, Style: pickTC(st.TextStyle, th.Text.BodyMedium, sc.OnSurfaceVariant)})
	}
	if len(r.Actions) > 0 {
		btns := make([]w.Widget, len(r.Actions))
		for i, a := range r.Actions {
			btns[i] = TextButton{Label: a.Label, OnPressed: a.OnPressed, Style: ButtonStyle{ForegroundColor: pick(st.ActionColor, sc.Primary)}}
		}
		kids = append(kids, w.Row{Spacing: 4, Children: btns})
	}
	return w.ConstrainedBox{Constraints: geom.Constraints{MaxW: pickF(st.MaxWidth, 320), MaxH: geom.Inf},
		Child: w.AbsorbPointer{Child: Surface{Color: pick(st.BackgroundColor, sc.SurfaceContainer), Radius: pickF(st.Radius, CornerMedium),
			Elevation: pickI(st.Elevation, 2), ShadowColor: st.ShadowColor,
			Child: w.Padding{Padding: pickE(st.Padding, geom.InsetsLTRB(16, 12, 16, 8)),
				Child: w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 4, Children: kids}}}}}
}

// HoverCard shows a RichTooltip (or any Card) near Child after the pointer
// rests on it. The card stays while the pointer is over Child or the card
// itself (so its actions can be clicked) and goes away shortly after it
// leaves both.
type HoverCard struct {
	Title   string
	Text    string
	Content w.Widget
	Actions []ToastAction
	// Card replaces the RichTooltip built from the fields above.
	Card  func(ctx w.BuildContext) w.Widget
	Child w.Widget
	// Style overrides Theme.HoverCard.
	Style HoverCardTheme
}

func (HoverCard) CreateState() w.State { return &hoverCardState{} }

type hoverCardState struct {
	w.StateBase
	ticker      *w.Ticker
	entry       *w.OverlayEntry
	overChild   bool
	overCard    bool
	leftAt      time.Duration // ticker time when the pointer left both
	waitingShow bool
}

const hoverCardGrace = 200 * time.Millisecond

func (s *hoverCardState) Dispose() { s.hide() }

func (s *hoverCardState) hide() {
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	if s.entry != nil {
		s.entry.Remove()
		s.entry = nil
	}
}

// tick drives showing (after the wait) and hiding (after the grace time).
func (s *hoverCardState) tick(el time.Duration) {
	hc := w.WidgetOf[HoverCard](s)
	wait := merge(ThemeOf(s.Context()).HoverCard, hc.Style).WaitDuration
	if wait == 0 {
		wait = 500 * time.Millisecond
	}
	switch {
	case s.waitingShow:
		if !s.overChild {
			s.hide()
			return
		}
		if el >= wait {
			s.waitingShow = false
			s.show()
		}
	case s.entry != nil:
		if s.overChild || s.overCard {
			s.leftAt = -1
			return
		}
		if s.leftAt < 0 {
			s.leftAt = el
		}
		if el-s.leftAt >= hoverCardGrace {
			s.hide()
		}
	}
}

func (s *hoverCardState) enter() {
	s.overChild = true
	if s.ticker == nil {
		s.waitingShow = s.entry == nil
		s.leftAt = -1
		s.ticker = s.Context().Owner().NewTicker(s.tick)
		s.ticker.Start()
	}
}

func (s *hoverCardState) show() {
	ctx := s.Context()
	ov := w.OverlayOf(ctx)
	ro := ctx.RenderObject()
	if ov == nil || ro == nil || s.entry != nil {
		return
	}
	hc := w.WidgetOf[HoverCard](s)
	origin, size := render.GlobalOrigin(ro), ro.Base().Size()
	win := ov.Context().RenderObject().Base().Size()
	maxW := pickF(merge(ThemeOf(ctx).HoverCard, hc.Style).MaxWidth, 320)
	x := min(max(origin.X, 8), max(8, win.W-maxW-8))
	below := origin.Y+size.H+4 < win.H*0.6
	s.leftAt = -1
	s.entry = &w.OverlayEntry{Builder: func(ctx w.BuildContext) w.Widget {
		var card w.Widget
		if hc.Card != nil {
			card = hc.Card(ctx)
		} else {
			card = RichTooltip{Title: hc.Title, Text: hc.Text, Content: hc.Content, Actions: hc.Actions, Style: hc.Style}
		}
		card = w.MouseRegion{
			OnEnter: func(w.PointerEvent) { s.overCard = true },
			OnExit:  func(w.PointerEvent) { s.overCard = false },
			Child:   card,
		}
		pos := w.Positioned{Left: w.At(x), Top: w.At(origin.Y + size.H + 4), Child: card}
		if !below {
			pos = w.Positioned{Left: w.At(x), Bottom: w.At(win.H - origin.Y + 4), Child: card}
		}
		return w.Stack{Expand: true, Children: []w.Widget{pos}}
	}}
	ov.Insert(s.entry)
}

func (s *hoverCardState) Build(ctx w.BuildContext) w.Widget {
	return w.MouseRegion{
		OnEnter: func(w.PointerEvent) { s.enter() },
		OnExit:  func(w.PointerEvent) { s.overChild = false },
		Child: w.Listener{OnEvent: func(e w.PointerEvent) {
			if e.Kind == w.PointerDown && s.entry == nil {
				s.hide() // a click cancels a pending card
			}
		}, Child: w.WidgetOf[HoverCard](s).Child},
	}
}
