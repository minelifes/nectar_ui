package material

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// TextField is an M3 text field (filled by default, or Outlined) with a
// floating label, hint, helper/error text, icons and a character counter.
type TextField struct {
	Controller  *w.TextEditingController // nil = internal
	Label       string
	Hint        string
	Helper      string
	Error       string // non-empty shows the error state
	Prefix      *vector.Icon
	Suffix      *vector.Icon
	OnSuffix    func() // makes the suffix icon a button
	Outlined    bool
	Obscure     bool
	Multiline   bool
	MinLines    int
	MaxLength   int // >0 shows a counter
	Disabled    bool
	Autofocus   bool
	Width       float32 // 0 = fill available width
	OnChanged   func(string)
	OnSubmitted func(string)
	// Style overrides Theme.Input. Style.Outlined wins over Outlined.
	Style InputDecorationTheme
}

func (TextField) CreateState() w.State { return &textFieldState{} }

type textFieldState struct {
	w.StateBase
	ctrl    *w.TextEditingController
	node    *w.FocusNode
	focused bool
	hovered bool
	float   *w.Animated
}

func (s *textFieldState) InitState() {
	tf := w.WidgetOf[TextField](s)
	s.ctrl = tf.Controller
	if s.ctrl == nil {
		s.ctrl = w.NewTextController("")
	}
	s.ctrl.AddListener(func() {
		if s.Mounted() {
			s.SetState(nil)
		}
	})
	s.node = &w.FocusNode{}
	v := float32(0)
	if s.ctrl.Text() != "" {
		v = 1
	}
	s.float = w.NewAnimated(s, 150*time.Millisecond, w.Standard, v)
}

func (s *textFieldState) Build(ctx w.BuildContext) w.Widget {
	tf := w.WidgetOf[TextField](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Input, tf.Style)
	tf.Outlined = pickB(st.Outlined, tf.Outlined)
	radius := pickF(st.Radius, CornerExtraSmall)
	hasText := s.ctrl.Text() != ""
	if s.focused || hasText {
		s.float.Set(1)
	} else {
		s.float.Set(0)
	}
	isErr := tf.Error != ""
	accent, labelIdle, content := pick(st.FocusColor, sc.Primary), pick(st.LabelStyle.Color, sc.OnSurfaceVariant), pick(st.TextStyle.Color, sc.OnSurface)
	indicator := sc.OnSurfaceVariant
	if tf.Outlined {
		indicator = sc.Outline
	}
	indicator = pick(st.BorderColor, indicator)
	if s.hovered && !s.focused {
		indicator = pick(st.HoverBorderColor, sc.OnSurface)
	}
	errC := pick(st.ErrorColor, sc.Error)
	if isErr {
		accent, indicator, labelIdle = errC, errC, errC
	}
	if tf.Disabled {
		dc, _ := disabledColors(sc)
		accent, labelIdle, content, indicator = dc, dc, dc, dc
	}
	labelColor := labelIdle
	if s.focused {
		labelColor = accent
		indicator = accent
	}
	t := s.float.Value()
	if tf.Label == "" {
		t = 1
	}

	// Field content: [prefix] editable [suffix]
	lineStyle := pickTC(st.TextStyle, th.Text.BodyLarge, content)
	edit := w.EditableText{
		Controller: s.ctrl, Focus: s.node, Style: lineStyle,
		CursorColor: accent, SelectionColor: accent.WithAlpha(0.3),
		Multiline: tf.Multiline, MinLines: tf.MinLines, Obscure: tf.Obscure,
		ReadOnly: tf.Disabled, Autofocus: tf.Autofocus,
		OnChanged: func(v string) {
			if tf.MaxLength > 0 && utf8.RuneCountInString(v) > tf.MaxLength {
				r := []rune(v)
				s.ctrl.SetText(string(r[:tf.MaxLength]))
				v = s.ctrl.Text()
			}
			if tf.OnChanged != nil {
				tf.OnChanged(v)
			}
		},
		OnSubmitted:   tf.OnSubmitted,
		OnFocusChange: func(f bool) { s.SetState(func() { s.focused = f }) },
	}
	var editArea w.Widget = edit
	if tf.Disabled {
		editArea = w.IgnorePointer{Ignoring: true, Child: edit}
	}
	// Hint shows when the label has floated (or there's no label) and empty.
	var hint w.Widget
	if tf.Hint != "" && !hasText && (t >= 1 || tf.Label == "") && (s.focused || tf.Label == "") {
		hint = w.IgnorePointer{Ignoring: true, Child: w.Text{Text: tf.Hint, Style: pickTC(st.HintStyle, th.Text.BodyLarge, sc.OnSurfaceVariant), MaxLines: 1}}
	}
	top := float32(16)
	if tf.Label != "" && !tf.Outlined {
		top = 24
	}
	stackKids := []w.Widget{w.Padding{Padding: geom.InsetsLTRB(0, top, 0, 8), Child: editArea}}
	if hint != nil {
		stackKids = append(stackKids, w.Positioned{Left: w.At(0), Top: w.At(top), Child: hint})
	}
	// prefixW is how far the text starts right of the no-icon position:
	// 12 padding + 24 icon (the 16 text padding is the same either way).
	const prefixW = 12 + 24
	// Floating label: resting = BodyLarge centered, floating = BodySmall on top.
	if tf.Label != "" {
		size := widgetsLerp(16, 12, t)
		y := widgetsLerp(16, 8, t)
		x := float32(0)
		if tf.Outlined {
			// The content stack starts 4px below the field top; the floated
			// label sits centered on the border line.
			y = widgetsLerp(14, -11.5, t)
			// With a leading icon the resting label sits after the icon, but
			// the floated one moves to the border start (16px from the edge,
			// inside the notch), sliding over the icon (M3).
			if tf.Prefix != nil {
				x = widgetsLerp(0, -prefixW, t)
			}
		}
		ls := pickT(st.LabelStyle, Styled(th.Text.BodyLarge, labelColor))
		ls.Color = labelColor
		ls.Size, ls.LineHeight = size, 1.25
		stackKids = append(stackKids, w.Positioned{Left: w.At(x), Top: w.At(y),
			Child: w.IgnorePointer{Ignoring: true, Child: w.Text{Text: tf.Label, Style: ls, MaxLines: 1}}})
	}
	row := []w.Widget{}
	iconCol := pick(st.IconColor, sc.OnSurfaceVariant)
	if tf.Disabled {
		iconCol = content
	}
	if tf.Prefix != nil {
		row = append(row, w.Padding{Padding: geom.InsetsLTRB(12, 0, 0, 0), Child: w.Icon{Icon: tf.Prefix, Size: 24, Color: iconCol}})
	}
	padL, padR := float32(16), float32(16)
	if tf.Prefix != nil {
		padL = 16
	}
	row = append(row, w.Expanded{Child: w.Padding{Padding: geom.InsetsLTRB(padL, 0, padR, 0), Child: w.Stack{Children: stackKids}}})
	suffix := tf.Suffix
	if isErr && suffix == nil {
		suffix = iconError
	}
	if suffix != nil {
		col := iconCol
		if isErr {
			col = errC
		}
		var sw w.Widget = w.Icon{Icon: suffix, Size: 24, Color: col}
		if tf.OnSuffix != nil && !tf.Disabled {
			sw = IconButton{Icon: suffix, OnPressed: tf.OnSuffix, Color: col}
		} else {
			sw = w.Padding{Padding: geom.InsetsLTRB(0, 0, 12, 0), Child: sw}
		}
		row = append(row, sw)
	}

	outlined := tf.Outlined
	fill := pick(st.FillColor, sc.SurfaceContainerHighest)
	if tf.Disabled {
		fill = sc.OnSurface.WithAlpha(0.04)
	}
	labelW := float32(0)
	if tf.Label != "" && outlined {
		labelW = measure(tf.Label, pickT(st.LabelStyle, th.Text.BodySmall)) + 8
	}
	bgForNotch := pick(th.ScaffoldBackground, sc.Surface)
	focusedNow := s.focused
	container := w.CustomPaint{
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			r := geom.RectFrom(o, size)
			if outlined {
				bw := float32(1)
				if focusedNow {
					bw = 2
				}
				c.StrokeRoundRect(r, radius, bw, indicator)
				// Notch: erase the border behind the floated label (the
				// label itself is painted afterwards, on top).
				if labelW > 0 && s.float.Value() > 0 {
					c.FillRect(geom.Rect{X: o.X + 12, Y: o.Y - 2, W: labelW * s.float.Value(), H: 4}, bgForNotch)
				}
				return
			}
			// Filled: top corners rounded, bottom square, active indicator.
			c.FillRoundRect(r, radius, fill)
			c.FillRect(geom.Rect{X: r.X, Y: r.Bottom() - radius, W: r.W, H: min(radius, r.H)}, fill)
			if s.hovered && !tf.Disabled {
				c.FillRoundRect(r, radius, sc.OnSurface.WithAlpha(HoverOpacity))
			}
			h := float32(1)
			if focusedNow {
				h = 2
			}
			c.FillRect(geom.Rect{X: r.X, Y: r.Bottom() - h, W: r.W, H: h}, indicator)
		},
		Child: w.ConstrainedBox{Constraints: geom.Constraints{MinW: 0, MaxW: geom.Inf, MinH: 56, MaxH: geom.Inf},
			Child: w.Row{Cross: w.CrossCenter, Children: row}},
	}
	var field w.Widget = container
	field = w.MouseRegion{Cursor: w.CursorText,
		OnEnter: func(w.PointerEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(w.PointerEvent) { s.SetState(func() { s.hovered = false }) },
		Child:   w.GestureDetector{OnTap: func() { s.node.RequestFocus() }, Child: field}}

	col := []w.Widget{field}
	support := tf.Helper
	supportColor := pick(st.HelperStyle.Color, sc.OnSurfaceVariant)
	if isErr {
		support, supportColor = tf.Error, errC
	}
	if tf.Disabled {
		supportColor = content
	}
	if support != "" || tf.MaxLength > 0 {
		hs := pickT(st.HelperStyle, th.Text.BodySmall)
		hs.Color = supportColor
		kids := []w.Widget{w.Expanded{Child: w.Text{Text: support, Style: hs}}}
		if tf.MaxLength > 0 {
			kids = append(kids, w.Text{Text: fmt.Sprintf("%d/%d", utf8.RuneCountInString(s.ctrl.Text()), tf.MaxLength), Style: hs})
		}
		col = append(col, w.Padding{Padding: geom.InsetsLTRB(16, 4, 16, 0), Child: w.Row{Children: kids}})
	}
	var out w.Widget = w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: col}
	if tf.Width > 0 {
		out = w.SizedBox{Width: tf.Width, Child: out}
	}
	return out
}

// measure returns the laid-out width of s in style st.
func measure(s string, st text.Style) float32 {
	return text.Layout(s, st, text.Options{}).Width
}

// ---------------------------------------------------------------------------

// SearchBar is a pill-shaped search input.
type SearchBar struct {
	Controller  *w.TextEditingController
	Hint        string
	Leading     w.Widget   // default: search icon
	Trailing    []w.Widget // e.g. an avatar or mic IconButton
	OnChanged   func(string)
	OnSubmitted func(string)
	Autofocus   bool
	// Style overrides Theme.SearchBar.
	Style SearchBarTheme
}

func (SearchBar) CreateState() w.State { return &searchBarState{} }

type searchBarState struct {
	w.StateBase
	ctrl *w.TextEditingController
	node *w.FocusNode
}

func (s *searchBarState) InitState() {
	sb := w.WidgetOf[SearchBar](s)
	s.ctrl = sb.Controller
	if s.ctrl == nil {
		s.ctrl = w.NewTextController("")
	}
	s.ctrl.AddListener(func() {
		if s.Mounted() {
			s.SetState(nil)
		}
	})
	s.node = &w.FocusNode{}
}

func (s *searchBarState) Build(ctx w.BuildContext) w.Widget {
	sb := w.WidgetOf[SearchBar](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.SearchBar, sb.Style)
	lead := sb.Leading
	if lead == nil {
		lead = w.Padding{Padding: geom.InsetsLTRB(16, 0, 0, 0), Child: w.Icon{Icon: iconSearch, Size: 24, Color: pick(st.IconColor, sc.OnSurface)}}
	}
	kids := []w.Widget{w.Expanded{Child: w.Padding{Padding: geom.InsetsHV(16, 0), Child: w.Stack{Children: s.content(sb, th, st)}}}}
	kids = append([]w.Widget{lead}, kids...)
	if len(sb.Trailing) > 0 {
		kids = append(kids, w.Padding{Padding: geom.InsetsLTRB(0, 0, 8, 0), Child: w.Row{ShrinkMain: true, Cross: w.CrossCenter, Children: sb.Trailing}})
	}
	return InkSurface{OnTap: func() { s.node.RequestFocus() }, NoFocus: true, Color: pick(st.BackgroundColor, sc.SurfaceContainerHigh),
		ContentColor: pick(st.OverlayColor, sc.OnSurface), Radius: pickF(st.Radius, CornerFull), Elevation: pickI(st.Elevation, 1),
		Child: w.SizedBox{Height: pickF(st.Height, 56), Child: w.Row{Cross: w.CrossCenter, Children: kids}}}
}

func (s *searchBarState) content(sb SearchBar, th Theme, st SearchBarTheme) []w.Widget {
	sc := th.Scheme
	kids := []w.Widget{w.EditableText{
		Controller: s.ctrl, Focus: s.node, Style: pickTC(st.TextStyle, th.Text.BodyLarge, sc.OnSurface),
		CursorColor: sc.Primary, SelectionColor: sc.Primary.WithAlpha(0.3), Autofocus: sb.Autofocus,
		OnChanged: sb.OnChanged, OnSubmitted: sb.OnSubmitted,
	}}
	if s.ctrl.Text() == "" && sb.Hint != "" {
		kids = append(kids, w.IgnorePointer{Ignoring: true, Child: w.Text{Text: sb.Hint, Style: pickTC(st.HintStyle, th.Text.BodyLarge, sc.OnSurfaceVariant), MaxLines: 1}})
	}
	return kids
}
