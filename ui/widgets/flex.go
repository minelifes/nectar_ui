package widgets

import "github.com/minelifes/nectar_ui/ui/render"

// Re-exported so users only need to import widgets.
type (
	MainAxisAlignment  = render.MainAxisAlignment
	CrossAxisAlignment = render.CrossAxisAlignment
)

const (
	MainStart        = render.MainStart
	MainCenter       = render.MainCenter
	MainEnd          = render.MainEnd
	MainSpaceBetween = render.MainSpaceBetween
	MainSpaceAround  = render.MainSpaceAround
	MainSpaceEvenly  = render.MainSpaceEvenly

	CrossStart   = render.CrossStart
	CrossCenter  = render.CrossCenter
	CrossEnd     = render.CrossEnd
	CrossStretch = render.CrossStretch
)

// Flex lays out children along an axis. Prefer Row / Column.
type Flex struct {
	Direction  render.Axis
	Main       MainAxisAlignment
	Cross      CrossAxisAlignment
	Spacing    float32
	ShrinkMain bool // size to children instead of filling the main axis
	Children   []Widget
}

func (w Flex) ChildWidgets() []Widget { return w.Children }

func (w Flex) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderFlex{}
	w.UpdateRenderObject(nil, r)
	return r
}

func (Flex) MarksOwnPaint() {}
func (w Flex) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderFlex)
	if r.Direction != w.Direction || r.Main != w.Main || r.Cross != w.Cross || r.Spacing != w.Spacing || r.ShrinkMain != w.ShrinkMain {
		r.Direction, r.Main, r.Cross, r.Spacing, r.ShrinkMain = w.Direction, w.Main, w.Cross, w.Spacing, w.ShrinkMain
		render.MarkNeedsLayout(r)
	}
}

// Row lays children out horizontally.
type Row struct {
	Main       MainAxisAlignment
	Cross      CrossAxisAlignment
	Spacing    float32
	ShrinkMain bool
	Children   []Widget
}

func (w Row) Build(BuildContext) Widget {
	return Flex{Direction: render.Horizontal, Main: w.Main, Cross: w.Cross, Spacing: w.Spacing, ShrinkMain: w.ShrinkMain, Children: w.Children}
}

// Column lays children out vertically.
type Column struct {
	Main       MainAxisAlignment
	Cross      CrossAxisAlignment
	Spacing    float32
	ShrinkMain bool
	Children   []Widget
}

func (w Column) Build(BuildContext) Widget {
	return Flex{Direction: render.Vertical, Main: w.Main, Cross: w.Cross, Spacing: w.Spacing, ShrinkMain: w.ShrinkMain, Children: w.Children}
}

// Expanded makes its child fill a share of the free space in a Row/Column.
type Expanded struct {
	Flex  int // default 1
	Child Widget
}

func (w Expanded) ChildWidget() Widget { return w.Child }
func (w Expanded) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderFlexible{Flex: max(1, w.Flex), Fit: true}
}
func (Expanded) MarksOwnPaint() {}
func (w Expanded) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	updateFlexible(ro, max(1, w.Flex), true)
}

// Flexible lets its child use up to a share of the free space.
type Flexible struct {
	Flex  int // default 1
	Child Widget
}

func (w Flexible) ChildWidget() Widget { return w.Child }
func (w Flexible) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderFlexible{Flex: max(1, w.Flex)}
}
func (Flexible) MarksOwnPaint() {}
func (w Flexible) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	updateFlexible(ro, max(1, w.Flex), false)
}

// Spacer takes up free space in a Row/Column.
type Spacer struct{ Flex int }

func (w Spacer) Build(BuildContext) Widget { return Expanded{Flex: w.Flex, Child: SizedBox{}} }

func updateFlexible(ro render.RenderObject, flex int, fit bool) {
	r := ro.(*render.RenderFlexible)
	if r.Flex != flex || r.Fit != fit {
		r.Flex, r.Fit = flex, fit
		render.MarkNeedsLayout(r)
	}
}
