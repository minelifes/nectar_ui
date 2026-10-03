package widgets

import (
	"sort"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// ScrollController reads and drives the offset of a scroll view.
type ScrollController struct {
	vp        render.ScrollAxis
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
	c.attachAxis(vp, owner)
	vp.OnMetrics = func(extent, content float32) { c.metrics(extent, content) }
}

// attachAxis connects the controller to a scrollable axis; the owner of the
// axis calls metrics after each layout.
func (c *ScrollController) attachAxis(vp render.ScrollAxis, owner *BuildOwner) {
	c.vp, c.owner = vp, owner
	vp.SetScrollOffset(c.offset)
}

func (c *ScrollController) metrics(extent, content float32) {
	if extent != c.lastExtent || content != c.lastCt {
		c.lastExtent, c.lastCt = extent, content
		c.offset = c.vp.Offset()
		c.notify()
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

	// strictAxis: a horizontal view ignores vertical wheel deltas (by
	// default a plain mouse wheel scrolls horizontal lists too).
	strictAxis bool
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
	// Scrolling only moves the content: replay its display list instead of
	// repainting it.
	child = RepaintBoundary{Child: child}
	return Listener{
		OnEvent: func(e PointerEvent) {
			// An inner scroll view that already moved has claimed the event.
			if e.Kind != render.PointerScroll || e.Winner() != nil {
				return
			}
			d := e.Scroll.Y
			if w.Horizontal {
				d = e.Scroll.X
				if d == 0 && (!w.strictAxis || Modifiers(e.Mods).Shift()) {
					d = e.Scroll.Y
				}
			}
			if ctrl.ScrollBy(d) && ctrl.vp != nil {
				e.Claim(ctrl.vp.Viewport())
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
func (viewport) MarksOwnPaint() {}
func (w viewport) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderViewport)
	if r.Axis != w.axis {
		r.Axis = w.axis
		render.MarkNeedsLayout(r)
	}
	if r.ThumbColor != w.thumb {
		r.ThumbColor = w.thumb
		render.MarkNeedsPaint(r)
	}
	if w.ctrl.vp != render.ScrollAxis(r) {
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

// ListViewBuilder lazily builds only the visible rows of a long list. Rows
// are ItemExtent tall (wide, if horizontal), or ItemExtentOf(i) when that's
// set (rows of different, known sizes: section headers, wrapped lines).
type ListViewBuilder struct {
	Horizontal bool
	Controller *ScrollController
	ItemCount  int
	ItemExtent float32
	// ItemExtentOf gives each row its own extent (overrides ItemExtent).
	// It's called for every row on each build, so keep it cheap.
	ItemExtentOf func(index int) float32
	Builder      func(ctx BuildContext, index int) Widget
	// IsHeader marks section headers (vertical lists): the header of the
	// section at the top stays pinned there while its rows scroll under
	// it, and the next header pushes it away.
	IsHeader   func(index int) bool
	ThumbColor geom.Color
}

func (ListViewBuilder) CreateState() State { return &lazyListState{} }

type lazyListState struct {
	StateBase
	ctrl       *ScrollController
	first, end int
	pinned     int     // pinned header row (-1 = none)
	pinY       float32 // its position (≤ 0 when pushed up)
	starts     []float32
}

func (s *lazyListState) InitState() {
	s.ctrl = WidgetOf[ListViewBuilder](s).Controller
	if s.ctrl == nil {
		s.ctrl = NewScrollController()
	}
	s.pinned = -1
	s.ctrl.AddListener(func() {
		s.measure()
		f, e := s.visible()
		p, py := s.pinnedHeader(f)
		if f != s.first || e != s.end || p != s.pinned || py != s.pinY {
			s.SetState(nil)
		}
	})
}

// measure computes the start of every row (variable extents only).
func (s *lazyListState) measure() {
	w := WidgetOf[ListViewBuilder](s)
	if w.ItemExtentOf == nil {
		s.starts = nil
		return
	}
	n := max(w.ItemCount, 0)
	if cap(s.starts) < n+1 {
		s.starts = make([]float32, n+1)
	}
	s.starts = s.starts[:n+1]
	var y float32
	for i := 0; i < n; i++ {
		s.starts[i] = y
		y += max(w.ItemExtentOf(i), 0)
	}
	s.starts[n] = y
}

// start returns the offset of row i.
func (s *lazyListState) start(i int) float32 {
	if s.starts != nil {
		return s.starts[min(max(i, 0), len(s.starts)-1)]
	}
	return float32(i) * WidgetOf[ListViewBuilder](s).ItemExtent
}

func (s *lazyListState) total() float32 {
	return s.start(max(WidgetOf[ListViewBuilder](s).ItemCount, 0))
}

// rowAt returns the row at offset v (clamped to [0, count]).
func (s *lazyListState) rowAt(v float32) int {
	w := WidgetOf[ListViewBuilder](s)
	count := max(w.ItemCount, 0)
	if !(v > 0) { // also catches NaN
		return 0
	}
	if s.starts == nil {
		ext := max(w.ItemExtent, 1)
		// Clamp in float before converting: int(±Inf) and int(NaN) differ
		// between CPUs (and would overflow).
		return int(min(v/ext, float32(count)))
	}
	return min(sort.Search(count, func(i int) bool { return s.starts[i+1] > v }), count)
}

// visible returns the range of rows to build (with one screen of slack).
func (s *lazyListState) visible() (int, int) {
	w := WidgetOf[ListViewBuilder](s)
	view := s.ctrl.ViewportExtent()
	if view <= 0 {
		view = 1000
	}
	count := max(w.ItemCount, 0)
	// The offset can be stale: the list may just have shrunk (a folder
	// collapsed) while scrolled far down, and the viewport only clamps it
	// at the next layout. Clamp it to the new length here, so the rows the
	// viewport will show are the ones that get built.
	off := min(s.ctrl.Offset(), s.total()-view)
	if !(off > 0) {
		off = 0
	}
	first := s.rowAt(off - view/2)
	end := max(first, min(s.rowAt(off+view*1.5)+1, count))
	return first, end
}

// pinnedHeader returns the header to pin at the top and its y in the view.
func (s *lazyListState) pinnedHeader(int) (int, float32) {
	w := WidgetOf[ListViewBuilder](s)
	if w.IsHeader == nil || w.Horizontal || w.ItemCount <= 0 {
		return -1, 0
	}
	off := max(s.ctrl.Offset(), 0)
	top := s.rowAt(off)
	h := -1
	for i := min(top, w.ItemCount-1); i >= 0; i-- {
		if w.IsHeader(i) {
			h = i
			break
		}
	}
	if h < 0 || (s.start(h) >= off && h == top) {
		return -1, 0 // the header is in place on its own
	}
	ext := s.start(h+1) - s.start(h)
	y := float32(0)
	for i := top + 1; i < w.ItemCount && s.start(i) < off+ext; i++ {
		if w.IsHeader(i) {
			y = min(0, s.start(i)-off-ext)
			break
		}
	}
	return h, y
}

func (s *lazyListState) Build(ctx BuildContext) Widget {
	w := WidgetOf[ListViewBuilder](s)
	s.measure()
	s.first, s.end = s.visible()
	extent := func(i int) float32 { return s.start(i+1) - s.start(i) }
	spacer := func(v float32) Widget {
		if w.Horizontal {
			return SizedBox{Width: v}
		}
		return SizedBox{Height: v}
	}
	sized := func(i int, item Widget) Widget {
		if w.Horizontal {
			return SizedBox{Width: extent(i), Child: item}
		}
		return SizedBox{Height: extent(i), Child: item}
	}
	kids := make([]Widget, 0, s.end-s.first+2)
	if s.first > 0 {
		kids = append(kids, KeyedSubtree{ID: "lead", Child: spacer(s.start(s.first))})
	}
	for i := s.first; i < s.end; i++ {
		kids = append(kids, KeyedSubtree{ID: i, Child: sized(i, w.Builder(ctx, i))})
	}
	if s.end < w.ItemCount {
		kids = append(kids, KeyedSubtree{ID: "tail", Child: spacer(s.total() - s.start(s.end))})
	}
	list := ScrollView{Horizontal: w.Horizontal, Controller: s.ctrl, ThumbColor: w.ThumbColor,
		Child: Flex{Direction: axisOf(w.Horizontal), Cross: CrossStretch, ShrinkMain: true, Children: kids}}
	if w.IsHeader == nil || w.Horizontal {
		return list
	}
	// Always a Stack (even with nothing pinned), so the list keeps its
	// place in the tree when a header gets pinned.
	s.pinned, s.pinY = s.pinnedHeader(s.first)
	layers := []Widget{KeyedSubtree{ID: "list", Child: list}}
	if s.pinned >= 0 {
		layers = append(layers, KeyedSubtree{ID: "pinned", Child: Positioned{Top: At(s.pinY), Left: At(0), Right: At(0),
			Child: ClipRect{Child: sized(s.pinned, w.Builder(ctx, s.pinned))}}})
	}
	return Stack{Children: layers}
}

func axisOf(horizontal bool) render.Axis {
	if horizontal {
		return render.Horizontal
	}
	return render.Vertical
}
