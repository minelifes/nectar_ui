package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/fuzzy"
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// QuickPickItem is one choice of a QuickPick.
type QuickPickItem struct {
	Label  string
	Detail string // dimmer text after the label (a path, a description)
	Hint   string // right-aligned (a shortcut)
	Icon   *vector.Icon
	Value  any
}

// QuickPick is a search box over a list: type to fuzzy-filter, ↑/↓ to
// move, Enter to pick, Escape to cancel. ShowQuickPick opens one as a
// popup at the top of the window; QuickPickPanel embeds one.
type QuickPick struct {
	Placeholder string
	Items       []QuickPickItem
	// Search, when set, supplies the items for each query instead of
	// fuzzy-filtering Items (a file index, a symbol search). It runs on
	// the UI goroutine at every keystroke.
	Search func(query string) []QuickPickItem
	// Query is the initial text.
	Query    string
	OnPick   func(item QuickPickItem)
	OnCancel func()
	// MaxVisible is how many items show before the list scrolls (8).
	MaxVisible int
	// Style overrides Theme.QuickPick.
	Style QuickPickTheme
}

// ShowQuickPick opens qp at the top center of the window. It closes when
// an item is picked, on Escape, or on a click outside.
func ShowQuickPick(ctx w.BuildContext, qp QuickPick) {
	ov := w.OverlayOf(ctx)
	if ov == nil {
		return
	}
	var entry *w.OverlayEntry
	done := false
	closeWith := func(fn func()) {
		if done {
			return
		}
		done = true
		entry.Remove()
		if fn != nil {
			fn()
		}
	}
	pick, cancel := qp.OnPick, qp.OnCancel
	qp.OnPick = func(it QuickPickItem) {
		closeWith(func() {
			if pick != nil {
				pick(it)
			}
		})
	}
	qp.OnCancel = func() { closeWith(cancel) }
	entry = &w.OverlayEntry{Builder: func(ctx w.BuildContext) w.Widget {
		st := merge(ThemeOf(ctx).QuickPick, qp.Style)
		width := pickF(st.Width, 600)
		return w.Stack{Expand: true, Children: []w.Widget{
			w.PositionedFill(w.GestureDetector{OnTapDown: func(w.TapDetails) { qp.OnCancel() }, Child: w.AbsorbPointer{}}),
			w.Positioned{Top: w.At(56), Left: w.At(0), Right: w.At(0), Child: w.Align{Alignment: geom.TopCenter,
				Child: w.SizedBox{Width: width, Child: fadeIn{child: QuickPickPanel{QuickPick: qp, Autofocus: true}}}}},
		}}
	}}
	ov.Insert(entry)
}

// QuickPickPanel shows a QuickPick inline.
type QuickPickPanel struct {
	QuickPick
	Autofocus bool
}

func (QuickPickPanel) CreateState() w.State { return &quickPickState{} }

type quickPickState struct {
	w.StateBase
	ctrl     *w.TextEditingController
	scroll   *w.ScrollController
	query    string
	items    []QuickPickItem
	matches  [][]int // matched byte offsets of each shown item's label
	selected int
}

func (s *quickPickState) qp() QuickPickPanel { return w.WidgetOf[QuickPickPanel](s) }

func (s *quickPickState) InitState() {
	qp := s.qp()
	s.ctrl = w.NewTextController(qp.Query)
	s.scroll = w.NewScrollController()
	s.filter(qp.Query)
}

// DidUpdateWidget refilters (the items may have changed) but keeps the
// highlighted item.
func (s *quickPickState) DidUpdateWidget(w.Widget) {
	sel := s.selected
	off := s.scroll.Offset()
	s.filter(s.query)
	if sel < len(s.items) {
		s.selected = sel
		s.scroll.JumpTo(off)
	}
}

// filter recomputes the shown items for query.
func (s *quickPickState) filter(query string) {
	qp := s.qp()
	s.query = query
	if qp.Search != nil {
		s.items = qp.Search(query)
		s.matches = make([][]int, len(s.items))
		for i, it := range s.items {
			if _, pos, ok := fuzzy.Score(query, it.Label); ok {
				s.matches[i] = pos
			}
		}
	} else {
		keys := make([]string, len(qp.Items))
		for i, it := range qp.Items {
			keys[i] = it.Label
			if it.Detail != "" {
				keys[i] += " " + it.Detail
			}
		}
		ms := fuzzy.Filter(query, keys)
		s.items = make([]QuickPickItem, len(ms))
		s.matches = make([][]int, len(ms))
		for i, m := range ms {
			s.items[i] = qp.Items[m.Index]
			for _, p := range m.Positions {
				if p < len(s.items[i].Label) {
					s.matches[i] = append(s.matches[i], p)
				}
			}
		}
	}
	s.selected = 0
	s.scroll.JumpTo(0)
}

func (s *quickPickState) move(d int) {
	if len(s.items) == 0 {
		return
	}
	s.SetState(func() {
		s.selected = min(max(s.selected+d, 0), len(s.items)-1)
	})
	s.reveal()
}

func (s *quickPickState) reveal() {
	h := s.itemHeight()
	top, view := float32(s.selected)*h, s.scroll.ViewportExtent()
	if off := s.scroll.Offset(); top < off {
		s.scroll.JumpTo(top)
	} else if view > 0 && top+h > off+view {
		s.scroll.JumpTo(top + h - view)
	}
}

func (s *quickPickState) itemHeight() float32 {
	st := merge(ThemeOf(s.Context()).QuickPick, s.qp().Style)
	return pickF(st.ItemHeight, 36)
}

func (s *quickPickState) pick() {
	qp := s.qp()
	if s.selected < len(s.items) && qp.OnPick != nil {
		qp.OnPick(s.items[s.selected])
	}
}

func (s *quickPickState) onKey(e w.KeyEvent) bool {
	switch e.Key {
	case w.KeyUp:
		s.move(-1)
	case w.KeyDown:
		s.move(1)
	case w.KeyPageUp:
		s.move(-s.visible())
	case w.KeyPageDown:
		s.move(s.visible())
	case w.KeyEscape:
		if c := s.qp().OnCancel; c != nil {
			c()
		}
	default:
		return false
	}
	return true
}

func (s *quickPickState) visible() int {
	if n := s.qp().MaxVisible; n > 0 {
		return n
	}
	return 8
}

func (s *quickPickState) Build(ctx w.BuildContext) w.Widget {
	qp := s.qp()
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.QuickPick, qp.Style)
	itemH := pickF(st.ItemHeight, 36)
	labelSt := pickTC(st.TextStyle, th.Text.BodyMedium, sc.OnSurface)
	detailSt := pickTC(st.DetailStyle, th.Text.BodySmall, sc.OnSurfaceVariant)
	hintSt := pickTC(st.HintStyle, th.Text.LabelMedium, sc.OnSurfaceVariant)
	inputSt := pickTC(st.InputStyle, th.Text.BodyLarge, sc.OnSurface)
	match := pick(st.MatchColor, sc.Primary)
	selBg := pick(st.SelectedColor, sc.SecondaryContainer)

	input := w.Stack{Children: []w.Widget{
		w.EditableText{Controller: s.ctrl, Style: inputSt, Autofocus: qp.Autofocus, CursorColor: sc.Primary,
			OnChanged:   func(q string) { s.SetState(func() { s.filter(q) }) },
			OnSubmitted: func(string) { s.pick() }},
	}}
	if s.ctrl.Text() == "" && qp.Placeholder != "" {
		input.Children = append(input.Children, w.IgnorePointer{Child: w.Text{Text: qp.Placeholder,
			Style: pickTC(st.InputStyle, th.Text.BodyLarge, sc.OnSurfaceVariant), MaxLines: 1}})
	}
	rows := len(s.items)
	listH := float32(min(max(rows, 1), s.visible())) * itemH
	var list w.Widget
	if rows == 0 {
		list = w.SizedBox{Height: itemH, Child: w.Padding{Padding: geom.InsetsHV(16, 0), Child: w.Align{Alignment: geom.CenterLeft,
			Child: w.Text{Text: "No matching results", Style: detailSt}}}}
	} else {
		list = w.SizedBox{Height: listH, Child: w.ListViewBuilder{Controller: s.scroll, ItemCount: rows, ItemExtent: itemH,
			Builder: func(ctx w.BuildContext, i int) w.Widget {
				return s.row(i, labelSt, detailSt, hintSt, match, selBg)
			}}}
	}
	return w.Focus{OnKey: s.onKey, Child: w.AbsorbPointer{Child: Surface{
		Color: pick(st.BackgroundColor, sc.SurfaceContainerHigh), Radius: pickF(st.Radius, CornerMedium),
		Elevation: pickI(st.Elevation, 3), ShadowColor: st.ShadowColor,
		Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
			w.Padding{Padding: geom.InsetsLTRB(14, 14, 14, 10), Child: input},
			Divider{},
			w.Padding{Padding: geom.InsetsHV(0, 6), Child: list},
		}}}}}
}

// fadeIn fades its child in when it appears (popups).
type fadeIn struct{ child w.Widget }

func (fadeIn) CreateState() w.State { return &fadeInState{} }

type fadeInState struct {
	w.StateBase
	t *w.Animated
}

func (s *fadeInState) InitState() {
	s.t = w.NewAnimated(s, 120*time.Millisecond, w.EmphasizedDecelerate, 0)
	s.t.Set(1)
}

func (s *fadeInState) Build(w.BuildContext) w.Widget {
	t := s.t.Value()
	if t >= 1 {
		return w.Opacity{Opacity: 1, Child: w.WidgetOf[fadeIn](s).child}
	}
	return w.Opacity{Opacity: t, Child: w.Translate{Offset: geom.Pt(0, (1-t)*-8), Child: w.WidgetOf[fadeIn](s).child}}
}

func (s *quickPickState) row(i int, labelSt, detailSt, hintSt text.Style, match, selBg geom.Color) w.Widget {
	it := s.items[i]
	spans := highlightSpans(it.Label, s.matches[i], match)
	kids := []w.Widget{}
	if it.Icon != nil {
		kids = append(kids, w.Icon{Icon: it.Icon, Size: 18, Color: hintSt.Color})
	}
	label := []w.Widget{w.RichText{Spans: spans, Style: labelSt, MaxLines: 1, Ellipsis: true}}
	if it.Detail != "" {
		label = append(label, w.Flexible{Child: w.Text{Text: it.Detail, Style: detailSt, MaxLines: 1, Ellipsis: true}})
	}
	kids = append(kids, w.Expanded{Child: w.Row{Cross: w.CrossCenter, Spacing: 8, Children: label}})
	if it.Hint != "" {
		kids = append(kids, w.Text{Text: it.Hint, Style: hintSt})
	}
	bg := geom.Transparent
	if i == s.selected {
		bg = selBg
	}
	return w.MouseRegion{OnHover: func(w.PointerEvent) {
		if s.selected != i {
			s.SetState(func() { s.selected = i })
		}
	}, Child: w.GestureDetector{OnTap: func() { s.selected = i; s.pick() },
		Child: w.Padding{Padding: geom.InsetsHV(6, 0), Child: w.DecoratedBox{Color: bg, Border: &geom.Border{Radius: CornerSmall},
			Child: w.Padding{Padding: geom.InsetsHV(10, 0), Child: w.Row{Cross: w.CrossCenter, Spacing: 10, Children: kids}}}}}}
}

// highlightSpans splits s into spans with the matched bytes in color.
func highlightSpans(s string, pos []int, color geom.Color) []text.Span {
	if len(pos) == 0 {
		return []text.Span{{Text: s}}
	}
	var out []text.Span
	at := 0
	for _, r := range fuzzy.Ranges(s, pos) {
		if r[0] > at {
			out = append(out, text.Span{Text: s[at:r[0]]})
		}
		out = append(out, text.Span{Text: s[r[0]:r[1]], Style: text.Style{Color: color, Font: MediumFont()}})
		at = r[1]
	}
	if at < len(s) {
		out = append(out, text.Span{Text: s[at:]})
	}
	return out
}

// ShowCommandPalette opens a QuickPick over the commands available where
// the focus is (see commands.Host.Available), with their shortcuts; the
// picked command runs after the focus returns to where it was. Recently
// run commands come first.
func ShowCommandPalette(ctx w.BuildContext) {
	host := commands.HostOf(ctx)
	if host == nil {
		return
	}
	snap := host.Snapshot()
	var items []QuickPickItem
	for _, c := range snap.Commands() {
		if c.Hidden || !c.IsEnabled() {
			continue
		}
		hint := ""
		if ks := host.Keymap.KeysFor(c); len(ks) > 0 {
			hint = ks[0].String()
		}
		items = append(items, QuickPickItem{Label: c.Label(), Hint: hint, Value: c.ID})
	}
	items = recentFirst(items, host.Recent())
	ShowQuickPick(ctx, QuickPick{Placeholder: "Type a command", Items: items,
		OnPick: func(it QuickPickItem) { snap.Run(it.Value.(string)) }})
}

func recentFirst(items []QuickPickItem, recent []string) []QuickPickItem {
	out := make([]QuickPickItem, 0, len(items))
	used := map[int]bool{}
	for _, id := range recent {
		for i, it := range items {
			if !used[i] && it.Value == id {
				out = append(out, it)
				used[i] = true
			}
		}
	}
	for i, it := range items {
		if !used[i] {
			out = append(out, it)
		}
	}
	return out
}
