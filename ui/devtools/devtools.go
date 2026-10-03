// Package devtools helps while building an app: an Inspector that shows
// which widget drew what (hover to highlight, click to see its type, size,
// ancestors and fields), a PerformanceOverlay with frame times, and text
// dumps of the widget and render trees for logs and tests.
//
//	app := devtools.Inspector{Enabled: inspecting, OnExit: stop,
//	    Child: devtools.PerformanceOverlay{Enabled: showFPS, Child: page}}
package devtools

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// TypeName returns a widget's type without its package path ("Padding",
// "material.TextButton"); internal widgets keep their lower-case names.
func TypeName(v any) string {
	t := reflect.TypeOf(v)
	if t == nil {
		return "nil"
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.String()
	if strings.HasPrefix(name, "widgets.") {
		name = strings.TrimPrefix(name, "widgets.")
	}
	return name
}

// DumpWidgets returns the widget tree under ctx, one widget per line,
// indented by depth (with keys where set).
func DumpWidgets(ctx w.BuildContext) string {
	var b strings.Builder
	var walk func(c w.BuildContext, depth int)
	walk = func(c w.BuildContext, depth int) {
		fmt.Fprintf(&b, "%s%s", strings.Repeat("  ", depth), TypeName(c.Widget()))
		if k, ok := c.Widget().(w.Keyed); ok {
			fmt.Fprintf(&b, " #%v", k.Key())
		}
		if t, ok := c.Widget().(w.Text); ok {
			fmt.Fprintf(&b, " %q", t.Text)
		}
		b.WriteByte('\n')
		w.VisitChildContexts(c, func(ch w.BuildContext) { walk(ch, depth+1) })
	}
	walk(ctx, 0)
	return b.String()
}

// DumpRender returns the render tree under ro with each node's position in
// the window and size.
func DumpRender(ro render.RenderObject) string {
	var b strings.Builder
	var walk func(r render.RenderObject, depth int)
	walk = func(r render.RenderObject, depth int) {
		o, s := render.GlobalOrigin(r), r.Base().Size()
		fmt.Fprintf(&b, "%s%s (%g,%g %gx%g)\n", strings.Repeat("  ", depth), strings.TrimPrefix(TypeName(r), "render."), o.X, o.Y, s.W, s.H)
		r.VisitChildren(func(c render.RenderObject) { walk(c, depth+1) })
	}
	if ro != nil {
		walk(ro, 0)
	}
	return b.String()
}

// RenderAt returns the topmost, deepest render object under pos (window
// coordinates) below root, skipping hidden subtrees.
func RenderAt(root render.RenderObject, pos geom.Offset) render.RenderObject {
	var found render.RenderObject
	var walk func(r render.RenderObject) bool
	walk = func(r render.RenderObject) bool {
		if off, ok := r.(*render.RenderOffstage); ok && off.Offstage {
			return false
		}
		if op, ok := r.(*render.RenderOpacity); ok && op.Opacity <= 0 {
			return false
		}
		if !geom.RectFrom(render.GlobalOrigin(r), r.Base().Size()).Contains(pos) {
			return false
		}
		var kids []render.RenderObject
		r.VisitChildren(func(c render.RenderObject) { kids = append(kids, c) })
		for i := len(kids) - 1; i >= 0; i-- {
			if walk(kids[i]) {
				return true
			}
		}
		found = r
		return true
	}
	if root != nil {
		walk(root)
	}
	return found
}

// ---------------------------------------------------------------------------
// Inspector

// Inspector, while Enabled, takes over the pointer: hovering outlines the
// widget under it, clicking selects it and shows its details in a panel.
// Escape calls OnExit.
type Inspector struct {
	Enabled bool
	OnExit  func()
	Child   w.Widget
}

func (Inspector) CreateState() w.State { return &inspectorState{} }

type inspectorState struct {
	w.StateBase
	hover, selected render.RenderObject
	childCtx        w.BuildContext
}

var (
	panelBg   = geom.HexA(0x1e1f22f0)
	panelText = geom.Hex(0xdfe1e5)
	panelDim  = geom.Hex(0x8c8f96)
	hoverCol  = geom.Hex(0x4c8dff)
	selectCol = geom.Hex(0xff9a3c)
)

func (s *inspectorState) childRoot() render.RenderObject {
	if s.childCtx == nil {
		return nil
	}
	return s.childCtx.RenderObject()
}

func (s *inspectorState) Build(ctx w.BuildContext) w.Widget {
	in := w.WidgetOf[Inspector](s)
	child := w.KeyedSubtree{ID: "child", Child: w.Builder{Builder: func(c w.BuildContext) w.Widget {
		s.childCtx = c
		return in.Child
	}}}
	if !in.Enabled {
		s.hover, s.selected = nil, nil
		return w.Stack{Expand: true, Children: []w.Widget{child}}
	}
	pick := func(p geom.Offset) render.RenderObject { return RenderAt(s.childRoot(), p) }
	origin := func() geom.Offset {
		if ro := ctx.RenderObject(); ro != nil {
			return render.GlobalOrigin(ro)
		}
		return geom.Offset{}
	}
	layers := []w.Widget{
		child,
		w.KeyedSubtree{ID: "input", Child: w.PositionedFill(w.Focus{Autofocus: true, OnKey: func(e w.KeyEvent) bool {
			if e.Key == w.KeyEscape && in.OnExit != nil {
				in.OnExit()
				return true
			}
			return false
		}, Child: w.MouseRegion{Cursor: w.CursorCrosshair,
			OnHover: func(e w.PointerEvent) {
				if ro := pick(e.Position); ro != s.hover {
					s.SetState(func() { s.hover = ro })
				}
			},
			OnExit: func(w.PointerEvent) { s.SetState(func() { s.hover = nil }) },
			Child: w.GestureDetector{OnTapDown: func(d w.TapDetails) {
				s.SetState(func() { s.selected = pick(d.Global) })
			}, Child: w.AbsorbPointer{}}}})},
		w.KeyedSubtree{ID: "outline", Child: w.PositionedFill(w.IgnorePointer{Ignoring: true, Child: w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, _ geom.Size) {
				base := origin()
				outline := func(ro render.RenderObject, col geom.Color) {
					if ro == nil || ro.Base().Owner() == nil {
						return
					}
					r := geom.RectFrom(render.GlobalOrigin(ro).Sub(base).Add(o), ro.Base().Size())
					c.FillRect(r, col.WithAlpha(0.12))
					c.StrokeRoundRect(r, 0, 1.5, col)
				}
				outline(s.hover, hoverCol)
				outline(s.selected, selectCol)
			}}})},
	}
	if s.selected != nil {
		layers = append(layers, w.KeyedSubtree{ID: "panel", Child: w.Positioned{Right: w.At(8), Top: w.At(8), Bottom: w.At(8), Width: w.At(340),
			Child: s.panel(ctx)}})
	} else if s.hover != nil {
		layers = append(layers, w.KeyedSubtree{ID: "tip", Child: w.Positioned{Left: w.At(8), Bottom: w.At(8),
			Child: w.IgnorePointer{Ignoring: true, Child: label(s.describeShort(s.hover))}}})
	}
	return w.Stack{Expand: true, Children: layers}
}

func label(s string) w.Widget {
	return w.DecoratedBox{Color: panelBg, Border: &geom.Border{Radius: 4}, Child: w.Padding{Padding: geom.InsetsHV(8, 4),
		Child: w.Text{Text: s, Style: text.Style{Size: 12, Color: panelText}}}}
}

// public returns the nearest ancestor-or-self of el whose widget is an
// exported type (Text, not the internal paragraph it builds).
func public(el w.BuildContext) w.BuildContext {
	for p := el; p != nil; p = p.Parent() {
		name := TypeName(p.Widget())
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		if name != "" && name[0] >= 'A' && name[0] <= 'Z' {
			return p
		}
	}
	return el
}

func (s *inspectorState) describeShort(ro render.RenderObject) string {
	sz := ro.Base().Size()
	name := strings.TrimPrefix(TypeName(ro), "render.")
	if el := w.ElementOf(s.childCtx, ro); el != nil {
		name = TypeName(public(el).Widget())
	}
	return fmt.Sprintf("%s  %g×%g", name, sz.W, sz.H)
}

// panel lists the selected widget's details.
func (s *inspectorState) panel(ctx w.BuildContext) w.Widget {
	ro := s.selected
	el := w.ElementOf(s.childCtx, ro)
	st := text.Style{Size: 12, Color: panelText}
	dim := text.Style{Size: 11, Color: panelDim}
	head := text.Style{Size: 13, Color: selectCol, Font: text.DefaultBoldFont()}
	o, sz := render.GlobalOrigin(ro), ro.Base().Size()
	rows := []w.Widget{}
	add := func(t string, s text.Style) { rows = append(rows, w.Text{Text: t, Style: s}) }
	if el != nil {
		pub := public(el)
		if pub != el {
			add(fmt.Sprintf("%s (%s)", TypeName(pub.Widget()), TypeName(el.Widget())), head)
			el = pub
		} else {
			add(TypeName(el.Widget()), head)
		}
	}
	add(fmt.Sprintf("render: %s", strings.TrimPrefix(TypeName(ro), "render.")), st)
	add(fmt.Sprintf("size %g × %g  at (%g, %g)", sz.W, sz.H, o.X, o.Y), st)
	add(fmt.Sprintf("constraints %v", ro.Base().Constraints()), dim)
	if el != nil {
		add("ancestors:", head)
		var path []string
		for p := el.Parent(); p != nil && len(path) < 24; p = p.Parent() {
			path = append(path, TypeName(p.Widget()))
		}
		for _, p := range path {
			add("  "+p, dim)
		}
		add("widget:", head)
		fields := fmt.Sprintf("%+v", el.Widget())
		if len(fields) > 1500 {
			fields = fields[:1500] + "…"
		}
		add(fields, dim)
	}
	return w.DecoratedBox{Color: panelBg, Border: &geom.Border{Radius: 6}, Child: w.Padding{Padding: geom.Insets(10),
		Child: w.ListView{Spacing: 3, Children: rows}}}
}

// ---------------------------------------------------------------------------
// PerformanceOverlay

// PerformanceOverlay, while Enabled, shows frame statistics in a corner:
// frames per second, build / layout / paint times, rebuilt elements and
// display-list size, with a graph of recent frame times. It keeps frames
// coming while shown (it's a development tool).
type PerformanceOverlay struct {
	Enabled   bool
	Alignment geom.Alignment // zero = top right
	Child     w.Widget
}

func (PerformanceOverlay) CreateState() w.State { return &perfState{} }

type perfState struct {
	w.StateBase
	ticker *w.Ticker
	stats  w.FrameStats
	recent []time.Duration // frame intervals, newest last
}

func (s *perfState) sync() {
	on := w.WidgetOf[PerformanceOverlay](s).Enabled
	switch {
	case on && s.ticker == nil:
		s.ticker = s.Context().Owner().NewTicker(func(time.Duration) {
			st := s.Context().Owner().Stats()
			if st.Frames == s.stats.Frames {
				return
			}
			s.SetState(func() {
				s.stats = st
				s.recent = append(s.recent, st.Interval)
				if len(s.recent) > 90 {
					s.recent = s.recent[len(s.recent)-90:]
				}
			})
		})
		s.ticker.Start()
	case !on && s.ticker != nil:
		s.ticker.Stop()
		s.ticker = nil
	}
}

func (s *perfState) InitState()               { s.sync() }
func (s *perfState) DidUpdateWidget(w.Widget) { s.sync() }
func (s *perfState) Dispose() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

// FPS returns the frame rate over the recent frames.
func (s *perfState) fps() float64 {
	var sum time.Duration
	n := 0
	for _, d := range s.recent {
		if d > 0 {
			sum += d
			n++
		}
	}
	if n == 0 || sum <= 0 {
		return 0
	}
	return float64(n) / sum.Seconds()
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond)) }

func (s *perfState) Build(w.BuildContext) w.Widget {
	po := w.WidgetOf[PerformanceOverlay](s)
	child := w.KeyedSubtree{ID: "child", Child: po.Child}
	if !po.Enabled {
		return w.Stack{Expand: true, Children: []w.Widget{child}}
	}
	st := s.stats
	line1 := fmt.Sprintf("%.0f fps   build %s  layout %s  paint %s", s.fps(), ms(st.Build), ms(st.Layout), ms(st.Paint))
	line2 := fmt.Sprintf("rebuilt %d   commands %d   frame #%d", st.Rebuilds, st.Commands, st.Frames)
	recent := append([]time.Duration(nil), s.recent...)
	graph := w.SizedBox{Width: 280, Height: 36, Child: w.CustomPaint{Painter: func(c *render.Canvas, o geom.Offset, sz geom.Size) {
		c.FillRect(geom.RectFrom(o, sz), geom.HexA(0x00000040))
		budget := 1000.0 / 60 // ms per frame at 60 fps
		y60 := o.Y + sz.H - sz.H/2
		c.FillRect(geom.Rect{X: o.X, Y: y60, W: sz.W, H: 1}, geom.HexA(0xffffff40))
		bw := sz.W / 90
		for i, d := range recent {
			v := float32(float64(d)/float64(time.Millisecond)/budget) * sz.H / 2
			v = min(v, sz.H)
			col := geom.Hex(0x5bd75b)
			if float64(d)/float64(time.Millisecond) > budget*1.5 {
				col = geom.Hex(0xff6b5b)
			}
			c.FillRect(geom.Rect{X: o.X + float32(i)*bw, Y: o.Y + sz.H - v, W: max(bw-1, 1), H: v}, col)
		}
	}}}
	al := po.Alignment
	if al == (geom.Alignment{}) {
		al = geom.TopRight
	}
	panel := w.IgnorePointer{Ignoring: true, Child: w.DecoratedBox{Color: panelBg, Border: &geom.Border{Radius: 6},
		Child: w.Padding{Padding: geom.Insets(8), Child: w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 4, Children: []w.Widget{
			w.Text{Text: line1, Style: text.Style{Size: 11, Color: panelText}},
			w.Text{Text: line2, Style: text.Style{Size: 11, Color: panelDim}},
			graph,
		}}}}}
	return w.Stack{Expand: true, Children: []w.Widget{
		child,
		w.KeyedSubtree{ID: "perf", Child: w.PositionedFill(w.IgnorePointer{Ignoring: true, Child: w.Padding{Padding: geom.Insets(8),
			Child: w.Align{Alignment: al, Child: panel}}})},
	}}
}
