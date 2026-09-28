package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// ---------------------------------------------------------------------------
// Text

// DefaultTextStyle provides the base text style for Text widgets below it.
type DefaultTextStyle struct {
	Style text.Style
	Child Widget
}

func (w DefaultTextStyle) ChildWidget() Widget { return w.Child }
func (w DefaultTextStyle) UpdateShouldNotify(old Widget) bool {
	return old.(DefaultTextStyle).Style != w.Style
}

// Text displays a string. Zero fields in Style inherit from the nearest
// DefaultTextStyle.
type Text struct {
	Text     string
	Style    text.Style
	Align    text.Align
	MaxLines int  // 0 = unlimited
	Ellipsis bool // add "…" when MaxLines cuts the text
}

func (w Text) Build(ctx BuildContext) Widget {
	style := w.Style
	if def, ok := DependOn[DefaultTextStyle](ctx); ok {
		style = mergeStyle(def.Style, style)
	}
	ell := ""
	if w.Ellipsis {
		ell = "…"
	}
	return paragraph{text: w.Text, style: style, align: w.Align, maxLines: w.MaxLines, ellipsis: ell}
}

func mergeStyle(base, s text.Style) text.Style {
	if s.Font == nil {
		s.Font = base.Font
	}
	if s.Size == 0 {
		s.Size = base.Size
	}
	if s.Color == (geom.Color{}) {
		s.Color = base.Color
	}
	if s.LineHeight == 0 {
		s.LineHeight = base.LineHeight
	}
	if s.LetterSpacing == 0 {
		s.LetterSpacing = base.LetterSpacing
	}
	return s
}

// paragraph is the render-object widget behind Text.
type paragraph struct {
	text     string
	style    text.Style
	align    text.Align
	maxLines int
	ellipsis string
}

func (w paragraph) CreateRenderObject(BuildContext) render.RenderObject {
	p := render.NewParagraph("", text.Style{}, 0)
	w.UpdateRenderObject(nil, p)
	return p
}

func (w paragraph) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	ro.(*render.RenderParagraph).Update(w.text, w.style, w.align, w.maxLines, w.ellipsis)
}

// ---------------------------------------------------------------------------
// Decoration & layout primitives

// DecoratedBox paints a background (optionally rounded / bordered).
type DecoratedBox struct {
	Color  geom.Color
	Border *geom.Border
	Child  Widget
}

func (w DecoratedBox) ChildWidget() Widget { return w.Child }
func (w DecoratedBox) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderDecoratedBox{Color: w.Color, Border: w.Border}
}
func (w DecoratedBox) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderDecoratedBox)
	if r.Color != w.Color || r.Border != w.Border {
		r.Color, r.Border = w.Color, w.Border
		render.MarkNeedsPaint(r)
	}
}

// Padding insets its child.
type Padding struct {
	Padding geom.EdgeInsets
	Child   Widget
}

func (w Padding) ChildWidget() Widget { return w.Child }
func (w Padding) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderPadding{Padding: w.Padding}
}
func (w Padding) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderPadding)
	if r.Padding != w.Padding {
		r.Padding = w.Padding
		render.MarkNeedsLayout(r)
	}
}

// SizedBox forces its size. A zero Width/Height leaves that axis
// unconstrained (use ConstrainedBox for an exact zero).
type SizedBox struct {
	Width, Height float32
	Child         Widget
}

func (w SizedBox) ChildWidget() Widget { return w.Child }
func (w SizedBox) constraints() geom.Constraints {
	c := geom.Unbounded()
	if w.Width > 0 {
		c = c.WithTightWidth(w.Width)
	}
	if w.Height > 0 {
		c = c.WithTightHeight(w.Height)
	}
	return c
}
func (w SizedBox) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderConstrainedBox{Additional: w.constraints()}
}
func (w SizedBox) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderConstrainedBox)
	if c := w.constraints(); r.Additional != c {
		r.Additional = c
		render.MarkNeedsLayout(r)
	}
}

// ConstrainedBox applies extra constraints to its child.
type ConstrainedBox struct {
	Constraints geom.Constraints
	Child       Widget
}

func (w ConstrainedBox) ChildWidget() Widget { return w.Child }
func (w ConstrainedBox) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderConstrainedBox{Additional: w.Constraints}
}
func (w ConstrainedBox) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderConstrainedBox)
	if r.Additional != w.Constraints {
		r.Additional = w.Constraints
		render.MarkNeedsLayout(r)
	}
}

// Align positions its child within itself.
type Align struct {
	Alignment geom.Alignment
	Child     Widget
}

func (w Align) ChildWidget() Widget { return w.Child }
func (w Align) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderAlign{Alignment: w.Alignment}
}
func (w Align) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderAlign)
	if r.Alignment != w.Alignment {
		r.Alignment = w.Alignment
		render.MarkNeedsLayout(r)
	}
}

// Center centers its child.
type Center struct{ Child Widget }

func (w Center) Build(BuildContext) Widget { return Align{Alignment: geom.Center, Child: w.Child} }

// ClipRect clips its child to its bounds.
type ClipRect struct{ Child Widget }

func (w ClipRect) ChildWidget() Widget { return w.Child }
func (w ClipRect) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderClipRect{}
}
func (w ClipRect) UpdateRenderObject(BuildContext, render.RenderObject) {}

// ---------------------------------------------------------------------------
// Container: the convenience widget composed from the primitives above.

// Container combines margin, size, decoration, padding and alignment.
type Container struct {
	Margin    geom.EdgeInsets
	Padding   geom.EdgeInsets
	Width     float32 // 0 = unconstrained
	Height    float32 // 0 = unconstrained
	Color     geom.Color
	Border    *geom.Border
	Alignment *geom.Alignment // nil = child keeps its own size
	Child     Widget
}

func (w Container) Build(BuildContext) Widget {
	child := w.Child
	if w.Alignment != nil {
		child = Align{Alignment: *w.Alignment, Child: child}
	}
	if !w.Padding.IsZero() {
		child = Padding{Padding: w.Padding, Child: child}
	}
	if w.Color.A > 0 || w.Border != nil || child == nil {
		child = DecoratedBox{Color: w.Color, Border: w.Border, Child: child}
	}
	if w.Width > 0 || w.Height > 0 {
		child = SizedBox{Width: w.Width, Height: w.Height, Child: child}
	}
	if !w.Margin.IsZero() {
		child = Padding{Padding: w.Margin, Child: child}
	}
	return child
}

// Ptr is a helper for optional fields: Alignment: widgets.Ptr(geom.Center).
func Ptr[T any](v T) *T { return &v }

// KeyedSubtree attaches a key to any widget so its element (and State) is
// matched by key instead of by position when a child list changes.
type KeyedSubtree struct {
	ID    any // must be comparable
	Child Widget
}

func (w KeyedSubtree) Key() any                  { return w.ID }
func (w KeyedSubtree) Build(BuildContext) Widget { return w.Child }
