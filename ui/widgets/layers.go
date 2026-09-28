package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
)

// Edge is an optional distance for Positioned; build one with At(v).
type Edge = render.Edge

// At returns a set edge distance: Positioned{Left: At(16)}.
func At(v float32) Edge { return render.At(v) }

// Stack paints children on top of each other (first = bottom).
type Stack struct {
	Alignment *geom.Alignment // for non-positioned children; nil = top-left
	Expand    bool            // fill the constraints instead of sizing to children
	Children  []Widget
}

func (w Stack) ChildWidgets() []Widget { return w.Children }
func (w Stack) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderStack{Alignment: w.align(), Expand: w.Expand}
}
func (w Stack) align() geom.Alignment {
	if w.Alignment == nil {
		return geom.TopLeft
	}
	return *w.Alignment
}
func (w Stack) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderStack)
	if r.Alignment != w.align() || r.Expand != w.Expand {
		r.Alignment, r.Expand = w.align(), w.Expand
		render.MarkNeedsLayout(r)
	}
}

// Positioned places its child inside a Stack by edges and/or size.
type Positioned struct {
	Left, Top, Right, Bottom Edge
	Width, Height            Edge
	Child                    Widget
}

// PositionedFill stretches its child over the whole Stack.
func PositionedFill(child Widget) Positioned {
	return Positioned{Left: At(0), Top: At(0), Right: At(0), Bottom: At(0), Child: child}
}

func (w Positioned) ChildWidget() Widget { return w.Child }
func (w Positioned) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderPositioned{}
	w.UpdateRenderObject(nil, r)
	return r
}
func (w Positioned) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderPositioned)
	if r.Left != w.Left || r.Top != w.Top || r.Right != w.Right || r.Bottom != w.Bottom || r.Width != w.Width || r.Height != w.Height {
		r.Left, r.Top, r.Right, r.Bottom, r.Width, r.Height = w.Left, w.Top, w.Right, w.Bottom, w.Width, w.Height
		render.MarkNeedsLayout(r)
	}
}

// Opacity fades its child (0 = invisible, 1 = opaque).
type Opacity struct {
	Opacity float32
	Child   Widget
}

func (w Opacity) ChildWidget() Widget { return w.Child }
func (w Opacity) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderOpacity{Opacity: w.Opacity}
}
func (w Opacity) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderOpacity)
	if r.Opacity != w.Opacity {
		r.Opacity = w.Opacity
		render.MarkNeedsPaint(r)
	}
}

// Translate shifts its child by Offset plus Fraction × child size, without
// changing layout (for slide animations).
type Translate struct {
	Offset   geom.Offset
	Fraction geom.Offset
	Child    Widget
}

func (w Translate) ChildWidget() Widget { return w.Child }
func (w Translate) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderTranslate{Offset: w.Offset, Fraction: w.Fraction}
}
func (w Translate) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderTranslate).SetTranslation(w.Offset, w.Fraction)
}

// IgnorePointer hides its subtree from pointer events.
type IgnorePointer struct {
	Ignoring bool
	Child    Widget
}

func (w IgnorePointer) ChildWidget() Widget { return w.Child }
func (w IgnorePointer) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderIgnorePointer{Ignoring: w.Ignoring}
}
func (w IgnorePointer) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderIgnorePointer).Ignoring = w.Ignoring
}

// AbsorbPointer swallows pointer hits over its area (modal barriers).
type AbsorbPointer struct{ Child Widget }

func (w AbsorbPointer) ChildWidget() Widget { return w.Child }
func (w AbsorbPointer) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderAbsorbPointer{}
}
func (w AbsorbPointer) UpdateRenderObject(BuildContext, render.RenderObject) {}

// Wrap flows children into runs (like text), e.g. for chip groups.
type Wrap struct {
	Spacing    float32
	RunSpacing float32
	Alignment  MainAxisAlignment
	Children   []Widget
}

func (w Wrap) ChildWidgets() []Widget { return w.Children }
func (w Wrap) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderWrap{Spacing: w.Spacing, RunSpacing: w.RunSpacing, Alignment: w.Alignment}
}
func (w Wrap) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderWrap)
	if r.Spacing != w.Spacing || r.RunSpacing != w.RunSpacing || r.Alignment != w.Alignment {
		r.Spacing, r.RunSpacing, r.Alignment = w.Spacing, w.RunSpacing, w.Alignment
		render.MarkNeedsLayout(r)
	}
}

// Painter draws with the canvas API; see render.Canvas.
type Painter = render.Painter

// CustomPaint draws with Painter behind its child (and ForegroundPainter in
// front). Without a child it's Size big (clamped to constraints); use
// geom.Inf for an axis that should fill the available space.
type CustomPaint struct {
	Painter           Painter
	ForegroundPainter Painter
	Size              geom.Size
	Child             Widget
}

func (w CustomPaint) ChildWidget() Widget { return w.Child }
func (w CustomPaint) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderCustomPaint{}
	w.UpdateRenderObject(nil, r)
	return r
}
func (w CustomPaint) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderCustomPaint)
	r.Painter, r.ForegroundPainter = w.Painter, w.ForegroundPainter
	if r.PreferredSize != w.Size {
		r.PreferredSize = w.Size
		render.MarkNeedsLayout(r)
	}
	render.MarkNeedsPaint(r)
}

// FractionallySizedBox sizes its child to a fraction of the available space.
type FractionallySizedBox struct {
	WidthFactor, HeightFactor float32
	Child                     Widget
}

func (w FractionallySizedBox) ChildWidget() Widget { return w.Child }
func (w FractionallySizedBox) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderFractionallySized{WidthFactor: w.WidthFactor, HeightFactor: w.HeightFactor}
}
func (w FractionallySizedBox) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderFractionallySized)
	if r.WidthFactor != w.WidthFactor || r.HeightFactor != w.HeightFactor {
		r.WidthFactor, r.HeightFactor = w.WidthFactor, w.HeightFactor
		render.MarkNeedsLayout(r)
	}
}

// ---------------------------------------------------------------------------
// Icons

// IconTheme sets the default size and color of Icons below it.
type IconTheme struct {
	Size  float32
	Color geom.Color
	Child Widget
}

func (w IconTheme) ChildWidget() Widget { return w.Child }
func (w IconTheme) UpdateShouldNotify(old Widget) bool {
	o := old.(IconTheme)
	return o.Size != w.Size || o.Color != w.Color
}

// Icon draws a vector icon. Zero Size/Color come from the nearest IconTheme
// (default 24px, black).
type Icon struct {
	Icon  *vector.Icon
	Size  float32
	Color geom.Color
}

func (w Icon) Build(ctx BuildContext) Widget {
	size, color := w.Size, w.Color
	if th, ok := DependOn[IconTheme](ctx); ok {
		if size == 0 {
			size = th.Size
		}
		if color == (geom.Color{}) {
			color = th.Color
		}
	}
	if size == 0 {
		size = 24
	}
	if color == (geom.Color{}) {
		color = geom.Black
	}
	ic := w.Icon
	return CustomPaint{Size: geom.Size{W: size, H: size}, Painter: func(c *render.Canvas, o geom.Offset, s geom.Size) {
		side := min(s.W, s.H)
		c.DrawIcon(ic, geom.Rect{X: o.X + (s.W-side)/2, Y: o.Y + (s.H-side)/2, W: side, H: side}, color)
	}}
}

// SizeTransition shows Factor (0..1) of its child's natural height (or
// width), clipping the rest. Animate Factor to expand/collapse.
type SizeTransition struct {
	Factor     float32
	Horizontal bool
	Child      Widget
}

func (w SizeTransition) ChildWidget() Widget { return w.Child }
func (w SizeTransition) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderSizeFactor{Factor: w.Factor, Horizontal: w.Horizontal}
}
func (w SizeTransition) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderSizeFactor)
	if r.Factor != w.Factor || r.Horizontal != w.Horizontal {
		r.Factor, r.Horizontal = w.Factor, w.Horizontal
		render.MarkNeedsLayout(r)
	}
}
