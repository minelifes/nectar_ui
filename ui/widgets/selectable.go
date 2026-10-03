package widgets

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// SelectableText is read-only text the user can select and copy: drag to
// select, double-click for a word, triple-click for everything, Shift+arrows
// to extend, Ctrl/⌘+A and Ctrl/⌘+C. Set Spans for rich text (Text is then
// ignored).
type SelectableText struct {
	Text     string
	Spans    []text.Span
	Style    text.Style
	Align    text.Align
	MaxLines int
	// SelectionColor highlights the selection; zero = the text color at
	// 30% opacity.
	SelectionColor geom.Color
	// OnSelectionChanged reports the selected text ("" when cleared).
	OnSelectionChanged func(selected string)
}

func (SelectableText) CreateState() State { return &selectableState{} }

type selectableState struct {
	StateBase
	node         *FocusNode
	ro           *render.RenderParagraph
	base, extent int
	lastTap      time.Time
	lastPos      geom.Offset
	taps         int
}

func (s *selectableState) w() SelectableText { return WidgetOf[SelectableText](s) }

func (s *selectableState) source() string {
	w := s.w()
	if w.Spans == nil {
		return w.Text
	}
	n := 0
	for _, sp := range w.Spans {
		n += len(sp.Text)
	}
	b := make([]byte, 0, n)
	for _, sp := range w.Spans {
		b = append(b, sp.Text...)
	}
	return string(b)
}

func (s *selectableState) InitState() {
	s.node = &FocusNode{
		UnfocusOnTapOutside: true,
		SkipTraversal:       true,
		OnKey:               s.onKey,
		OnFocusChange: func(f bool) {
			if !f {
				s.select_(s.extent, s.extent)
			}
		},
	}
}

func (s *selectableState) DidUpdateWidget(Widget) {
	n := len(s.source())
	if s.base > n || s.extent > n {
		s.base, s.extent = 0, 0
	}
}

// select_ sets the selection and reports it.
func (s *selectableState) select_(a, b int) {
	if a == s.base && b == s.extent {
		return
	}
	s.SetState(func() { s.base, s.extent = a, b })
	if cb := s.w().OnSelectionChanged; cb != nil {
		cb(s.selected())
	}
}

func (s *selectableState) selected() string {
	lo, hi := min(s.base, s.extent), max(s.base, s.extent)
	src := s.source()
	if hi > len(src) {
		return ""
	}
	return src[lo:hi]
}

func (s *selectableState) onKey(e KeyEvent) bool {
	src := s.source()
	switch {
	case e.Mods.Shortcut() && e.Key == KeyA:
		s.select_(0, len(src))
	case e.Mods.Shortcut() && e.Key == KeyC:
		if t := s.selected(); t != "" {
			if cb := s.Context().Owner().Clipboard; cb != nil {
				_ = cb.WriteText(t)
			}
		}
	case e.Key == KeyEscape && s.base != s.extent:
		s.select_(s.extent, s.extent)
	case e.Mods.Shift() && e.Key == KeyLeft:
		s.select_(s.base, text.PrevRune(src, s.extent))
	case e.Mods.Shift() && e.Key == KeyRight:
		s.select_(s.base, text.NextRune(src, s.extent))
	case e.Mods.Shift() && e.Key == KeyHome:
		s.select_(s.base, 0)
	case e.Mods.Shift() && e.Key == KeyEnd:
		s.select_(s.base, len(src))
	default:
		return false
	}
	return true
}

func (s *selectableState) offsetAt(local geom.Offset) int {
	if s.ro == nil {
		return 0
	}
	return s.ro.OffsetAt(local)
}

func (s *selectableState) Build(ctx BuildContext) Widget {
	w := s.w()
	style := w.Style
	if def, ok := DependOn[DefaultTextStyle](ctx); ok {
		style = mergeStyle(def.Style, style)
	}
	sel := w.SelectionColor
	if sel == (geom.Color{}) {
		sel = style.Resolved().Color.WithAlpha(0.3)
	}
	p := selectableParagraph{
		paragraph: paragraph{text: w.Text, spans: w.Spans, rich: w.Spans != nil, style: style, align: w.Align, maxLines: w.MaxLines},
		a:         s.base, b: s.extent, color: sel, state: s,
	}
	return Focus{Node: s.node, Child: MouseRegion{Cursor: CursorText, Child: GestureDetector{
		OnTapDown: func(d TapDetails) {
			s.node.RequestFocus()
			now := s.Context().Owner().now()
			near := d.Local.Sub(s.lastPos)
			if now.Sub(s.lastTap) < 400*time.Millisecond && near.X*near.X+near.Y*near.Y < 25 {
				s.taps++
			} else {
				s.taps = 1
			}
			s.lastTap, s.lastPos = now, d.Local
			i := s.offsetAt(d.Local)
			switch s.taps {
			case 1:
				s.select_(i, i)
			case 2:
				a, b := text.WordAt(s.source(), i)
				s.select_(a, b)
			default:
				s.select_(0, len(s.source()))
			}
		},
		OnPanStart: func(d DragDetails) {
			s.node.RequestFocus()
			s.taps = 0
		},
		OnPanUpdate: func(d DragDetails) {
			s.select_(s.base, s.offsetAt(d.Local))
		},
		Child: p,
	}}}
}

// selectableParagraph is a paragraph that paints a selection.
type selectableParagraph struct {
	paragraph
	a, b  int
	color geom.Color
	state *selectableState
}

func (w selectableParagraph) CreateRenderObject(ctx BuildContext) render.RenderObject {
	p := render.NewParagraph("", text.Style{}, 0)
	w.UpdateRenderObject(ctx, p)
	return p
}

func (selectableParagraph) MarksOwnPaint() {}

func (w selectableParagraph) UpdateRenderObject(ctx BuildContext, ro render.RenderObject) {
	w.paragraph.UpdateRenderObject(ctx, ro)
	r := ro.(*render.RenderParagraph)
	r.SetSelection(w.a, w.b, w.color)
	w.state.ro = r
}
