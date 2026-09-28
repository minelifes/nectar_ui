package widgets

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// ScrollController reads and drives the offset of a scroll view.
type ScrollController struct {
	vp        *render.RenderViewport
	offset    float32
	listeners []func()

	owner              *BuildOwner
	anim               *Ticker
	animFrom, animTo   float32
	animDur            time.Duration
	lastExtent, lastCt float32
}

// NewScrollController creates a controller starting at offset 0.
func NewScrollController() *ScrollController { return &ScrollController{} }

// Offset returns the scroll position.
func (c *ScrollController) Offset() float32 {
	if c.vp != nil {
		return c.vp.Offset()
	}
	return c.offset
}

// MaxOffset returns the largest valid offset (0 before the first layout).
func (c *ScrollController) MaxOffset() float32 {
	if c.vp != nil {
		return c.vp.MaxOffset()
	}
	return 0
}

// ViewportExtent returns the visible size along the scroll axis.
func (c *ScrollController) ViewportExtent() float32 {
	if c.vp != nil {
		return c.vp.ViewportExtent()
	}
	return 0
}

// AddListener runs fn whenever the offset or the content size changes.
func (c *ScrollController) AddListener(fn func()) { c.listeners = append(c.listeners, fn) }

func (c *ScrollController) notify() {
	for _, fn := range c.listeners {
		fn()
	}
}

// JumpTo scrolls immediately.
func (c *ScrollController) JumpTo(v float32) {
	c.stopAnim()
	c.set(v)
}

func (c *ScrollController) set(v float32) bool {
	old := c.Offset()
	if c.vp != nil {
		c.offset = c.vp.SetScrollOffset(v)
	} else {
		c.offset = max(0, v)
	}
	if c.offset != old {
		c.notify()
		return true
	}
	return false
}

// ScrollBy scrolls by delta; returns whether the offset changed.
func (c *ScrollController) ScrollBy(delta float32) bool {
	c.stopAnim()
	return c.set(c.Offset() + delta)
}

// AnimateTo scrolls smoothly to v.
func (c *ScrollController) AnimateTo(v float32, d time.Duration) {
	if c.owner == nil || d <= 0 {
		c.JumpTo(v)
		return
	}
	c.stopAnim()
	c.animFrom, c.animTo, c.animDur = c.Offset(), v, d
	c.anim = c.owner.NewTicker(func(el time.Duration) {
		t := min(float32(el)/float32(c.animDur), 1)
		c.set(Lerp(c.animFrom, c.animTo, Emphasized(t)))
		if t >= 1 {
			c.stopAnim()
		}
	})
	c.anim.Start()
}

func (c *ScrollController) stopAnim() {
	if c.anim != nil {
		c.anim.Stop()
		c.anim = nil
	}
}

func (c *ScrollController) attach(vp *render.RenderViewport, owner *BuildOwner) {
	c.vp, c.owner = vp, owner
	vp.SetScrollOffset(c.offset)
	vp.OnMetrics = func(extent, content float32) {
		if extent != c.lastExtent || content != c.lastCt {
			c.lastExtent, c.lastCt = extent, content
			c.offset = vp.Offset()
			c.notify()
		}
	}
}

// ---------------------------------------------------------------------------

// ScrollView makes its child scrollable along Axis (vertical by default):
// mouse wheel / trackpad and drag. It must get bounded constraints on the
// scroll axis (e.g. inside Expanded).
type ScrollView struct {
	Horizontal bool // scroll sideways instead of vertically
	Controller *ScrollController
	Padding    geom.EdgeInsets
	ThumbColor geom.Color // scrollbar; zero = none
	Child      Widget
}

func (ScrollView) CreateState() State { return &scrollState{} }

type scrollState struct {
	StateBase
	ctrl *ScrollController
}

func (s *scrollState) InitState() {
	s.ctrl = WidgetOf[ScrollView](s).Controller
	if s.ctrl == nil {
		s.ctrl = NewScrollController()
	}
}

func (s *scrollState) DidUpdateWidget(Widget) {
	if c := WidgetOf[ScrollView](s).Controller; c != nil {
		s.ctrl = c
	}
}

func (s *scrollState) Build(ctx BuildContext) Widget {
	w := WidgetOf[ScrollView](s)
	axis := axisOf(w.Horizontal)
	ctrl := s.ctrl
	child := w.Child
	if !w.Padding.IsZero() {
		child = Padding{Padding: w.Padding, Child: child}
	}
	return Listener{
		OnEvent: func(e PointerEvent) {
			// An inner scroll view that already moved has claimed the event.
			if e.Kind != render.PointerScroll || e.Winner() != nil {
				return
			}
			d := e.Scroll.Y
			if w.Horizontal {
				d = e.Scroll.X
				if d == 0 {
					d = e.Scroll.Y
				}
			}
			if ctrl.ScrollBy(d) && ctrl.vp != nil {
				e.Claim(ctrl.vp)
			}
		},
		Child: GestureDetector{
			OnPanUpdate: func(d DragDetails) {
				if !w.Horizontal {
					ctrl.ScrollBy(-d.Delta.Y)
				} else {
					ctrl.ScrollBy(-d.Delta.X)
				}
			},
			Child: viewport{axis: axis, ctrl: ctrl, thumb: w.ThumbColor, owner: ctx.Owner(), child: child},
		},
	}
}

type viewport struct {
	axis  render.Axis
	ctrl  *ScrollController
	thumb geom.Color
	owner *BuildOwner
	child Widget
}

func (w viewport) ChildWidget() Widget { return w.child }
func (w viewport) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderViewport{Axis: w.axis, ThumbColor: w.thumb}
	w.ctrl.attach(r, w.owner)
	return r
}
func (w viewport) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderViewport)
	if r.Axis != w.axis {
		r.Axis = w.axis
		render.MarkNeedsLayout(r)
	}
	r.ThumbColor = w.thumb
	if w.ctrl.vp != r {
		w.ctrl.attach(r, w.owner)
	}
}

// ListView is a scrolling Column (or Row, with Axis: Horizontal).
type ListView struct {
	Horizontal bool
	Controller *ScrollController
	Padding    geom.EdgeInsets
	Spacing    float32
	ThumbColor geom.Color
	Children   []Widget
}

func (w ListView) Build(BuildContext) Widget {
	return ScrollView{Horizontal: w.Horizontal, Controller: w.Controller, Padding: w.Padding, ThumbColor: w.ThumbColor,
		Child: Flex{Direction: axisOf(w.Horizontal), Cross: CrossStretch, Spacing: w.Spacing, ShrinkMain: true, Children: w.Children}}
}

// ListViewBuilder lazily builds only the visible rows of a long list. All
// rows are ItemExtent tall (wide, if horizontal).
type ListViewBuilder struct {
	Horizontal bool
	Controller *ScrollController
	ItemCount  int
	ItemExtent float32
	Builder    func(ctx BuildContext, index int) Widget
	ThumbColor geom.Color
}

func (ListViewBuilder) CreateState() State { return &lazyListState{} }

type lazyListState struct {
	StateBase
	ctrl       *ScrollController
	first, end int
}

func (s *lazyListState) InitState() {
	s.ctrl = WidgetOf[ListViewBuilder](s).Controller
	if s.ctrl == nil {
		s.ctrl = NewScrollController()
	}
	s.ctrl.AddListener(func() {
		if f, e := s.visible(); f != s.first || e != s.end {
			s.SetState(nil)
		}
	})
}

// visible returns the range of rows to build (with one screen of slack).
func (s *lazyListState) visible() (int, int) {
	w := WidgetOf[ListViewBuilder](s)
	ext := max(w.ItemExtent, 1)
	view := s.ctrl.ViewportExtent()
	if view <= 0 {
		view = 1000
	}
	off := s.ctrl.Offset()
	first := max(0, int((off-view/2)/ext))
	end := min(w.ItemCount, int((off+view*1.5)/ext)+1)
	return first, end
}

func (s *lazyListState) Build(ctx BuildContext) Widget {
	w := WidgetOf[ListViewBuilder](s)
	s.first, s.end = s.visible()
	ext := w.ItemExtent
	spacer := func(n int) Widget {
		if w.Horizontal {
			return SizedBox{Width: float32(n) * ext}
		}
		return SizedBox{Height: float32(n) * ext}
	}
	kids := make([]Widget, 0, s.end-s.first+2)
	if s.first > 0 {
		kids = append(kids, KeyedSubtree{ID: "lead", Child: spacer(s.first)})
	}
	for i := s.first; i < s.end; i++ {
		item := w.Builder(ctx, i)
		if w.Horizontal {
			item = SizedBox{Width: ext, Child: item}
		} else {
			item = SizedBox{Height: ext, Child: item}
		}
		kids = append(kids, KeyedSubtree{ID: i, Child: item})
	}
	if rest := w.ItemCount - s.end; rest > 0 {
		kids = append(kids, KeyedSubtree{ID: "tail", Child: spacer(rest)})
	}
	return ScrollView{Horizontal: w.Horizontal, Controller: s.ctrl, ThumbColor: w.ThumbColor,
		Child: Flex{Direction: axisOf(w.Horizontal), Cross: CrossStretch, ShrinkMain: true, Children: kids}}
}

func axisOf(horizontal bool) render.Axis {
	if horizontal {
		return render.Horizontal
	}
	return render.Vertical
}
