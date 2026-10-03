package material

import (
	"math"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// CarouselLayout selects one of the M3 carousel layouts.
type CarouselLayout uint8

const (
	// CarouselMultiBrowse shows a large, a medium and a small item; items
	// shrink as they scroll towards the edges.
	CarouselMultiBrowse CarouselLayout = iota
	// CarouselUncontained shows items at one size, cut off by the edge.
	CarouselUncontained
	// CarouselHero shows one large item and a peek of the next; snaps.
	CarouselHero
	// CarouselFullScreen shows one item filling the carousel; snaps.
	CarouselFullScreen
)

// Carousel is a horizontally scrolling set of items (M3 carousel). Items are
// masked to rounded rectangles; their content keeps its full width and is
// centered in the mask, giving the M3 "parallax" reveal.
//
// Scroll with drag, a horizontal trackpad swipe, or the arrow keys when
// focused. Vertical wheel scrolling passes through to the page unless
// WheelScroll is set.
// Item content is visual only: handle taps with OnTap (the index of the
// tapped item).
type Carousel struct {
	Layout CarouselLayout
	Height float32 // default 200 (full screen: fills the height it's given)
	// ItemExtent is the large item width for MultiBrowse / Hero and the item
	// width for Uncontained. 0 = derived from the carousel width.
	ItemExtent float32
	Items      []w.Widget
	// Colors are item background fills (cycled); nil = surface container
	// tones from the theme.
	Colors []geom.Color
	OnTap  func(index int)
	// Snap aligns to items after a drag or wheel gesture. Always on for
	// Hero and FullScreen.
	Snap bool
	// WheelScroll lets the vertical mouse wheel scroll the carousel too
	// (down = forward). While the pointer is over it, the carousel takes
	// the wheel and the page around it stays put; once it reaches the last
	// item (scrolling down) or the first (scrolling up), wheel events pass
	// through and the page continues scrolling. Horizontal scrolling works
	// either way and releases at the ends the same way.
	WheelScroll bool
}

func (Carousel) CreateState() w.State { return &carouselState{} }

type carouselState struct {
	w.StateBase
	pos       float32 // scroll position in item units
	ro        *renderCarousel
	node      *w.FocusNode
	snapAnim  *w.Ticker
	wheelTick *w.Ticker
	wheelFrom float32 // position when the current wheel gesture started
	snapFrom  float32
	snapTo    float32
}

func (s *carouselState) c() Carousel { return w.WidgetOf[Carousel](s) }

func (s *carouselState) snaps() bool {
	c := s.c()
	return c.Snap || c.Layout == CarouselHero || c.Layout == CarouselFullScreen
}

func (s *carouselState) InitState() {
	s.node = &w.FocusNode{OnKey: func(e w.KeyEvent) bool {
		switch e.Key {
		case w.KeyLeft:
			s.animateTo(float32(math.Round(float64(s.pos))) - 1)
		case w.KeyRight:
			s.animateTo(float32(math.Round(float64(s.pos))) + 1)
		default:
			return false
		}
		return true
	}}
}

func (s *carouselState) Dispose() {
	s.stopSnap()
	if s.wheelTick != nil {
		s.wheelTick.Stop()
	}
}

func (s *carouselState) maxPos() float32 {
	if s.ro == nil {
		return float32(max(len(s.c().Items)-1, 0))
	}
	return s.ro.maxPos()
}

func (s *carouselState) setPos(p float32) {
	p = min(max(p, 0), s.maxPos())
	if p != s.pos {
		s.SetState(func() { s.pos = p })
	}
}

func (s *carouselState) stopSnap() {
	if s.snapAnim != nil {
		s.snapAnim.Stop()
		s.snapAnim = nil
	}
}

// animateTo scrolls smoothly to an item position.
func (s *carouselState) animateTo(target float32) {
	target = min(max(target, 0), s.maxPos())
	s.stopSnap()
	s.snapFrom, s.snapTo = s.pos, target
	if s.snapFrom == s.snapTo {
		return
	}
	d := 300 * time.Millisecond
	s.snapAnim = s.Context().Owner().NewTicker(func(el time.Duration) {
		t := min(float32(el)/float32(d), 1)
		s.setPos(w.Lerp(s.snapFrom, s.snapTo, w.Emphasized(t)))
		if t >= 1 {
			s.stopSnap()
		}
	})
	s.snapAnim.Start()
}

func (s *carouselState) snap() {
	if s.snaps() {
		s.animateTo(float32(math.Round(float64(s.pos))))
	}
}

// unit is the scroll distance of one item in pixels.
func (s *carouselState) unit() float32 {
	if s.ro == nil || s.ro.unit <= 0 {
		return 1
	}
	return s.ro.unit
}

func (s *carouselState) onWheel(e w.PointerEvent) {
	if e.Kind != render.PointerScroll || e.Winner() != nil {
		return
	}
	// Horizontal scroll (trackpad swipe, tilt wheel) always moves the
	// carousel; the vertical wheel only with WheelScroll, otherwise it goes
	// to the page around it.
	d := e.Scroll.X
	if d == 0 && s.c().WheelScroll {
		d = e.Scroll.Y
	}
	if d == 0 {
		return
	}
	old := s.pos
	s.stopSnap()
	s.setPos(s.pos + d/s.unit())
	// Claim the event only if the carousel actually moved: at the first or
	// last item it is released, so an outer scroll view takes over.
	if s.pos != old && s.ro != nil {
		e.Claim(s.ro)
	}
	if !s.snaps() {
		return
	}
	// Snap shortly after the wheel / trackpad gesture ends: every event
	// restarts the ticker, whose clock then measures the idle time. The
	// snap goes to the next item in the gesture's direction, so even one
	// notch of a mouse wheel pages by an item instead of springing back.
	if s.wheelTick == nil {
		s.wheelTick = s.Context().Owner().NewTicker(func(idle time.Duration) {
			if idle > 150*time.Millisecond {
				s.wheelTick.Stop()
				switch p := float64(s.pos); {
				case s.pos > s.wheelFrom:
					s.animateTo(float32(math.Ceil(p - 1e-3)))
				case s.pos < s.wheelFrom:
					s.animateTo(float32(math.Floor(p + 1e-3)))
				default:
					s.snap()
				}
			}
		})
	}
	if !s.wheelTick.Active() && s.pos != old {
		s.wheelFrom = old // start of a new wheel gesture
	}
	s.wheelTick.Stop()
	s.wheelTick.Start()
}

func (s *carouselState) Build(ctx w.BuildContext) w.Widget {
	c := s.c()
	th := ThemeOf(ctx)
	sc := th.Scheme
	ct := th.Carousel
	colors := c.Colors
	if len(colors) == 0 {
		colors = []geom.Color{pick(ct.ItemColors[0], sc.SurfaceContainerHighest), pick(ct.ItemColors[1], sc.SurfaceContainerHigh), pick(ct.ItemColors[2], sc.SurfaceContainer)}
	}
	s.pos = min(max(s.pos, 0), s.maxPos())
	items := make([]w.Widget, len(c.Items))
	for i, it := range c.Items {
		items[i] = w.KeyedSubtree{ID: i, Child: w.IgnorePointer{Ignoring: true, Child: it}}
	}
	body := carouselView{state: s, layout: c.Layout, extent: c.ItemExtent, height: c.Height, pos: s.pos, colors: colors, radius: pickF(ct.Radius, CornerExtraLarge), items: items}
	return w.Focus{Node: s.node, Child: w.Listener{OnEvent: s.onWheel, Child: w.GestureDetector{
		OnPanStart: func(w.DragDetails) { s.stopSnap() },
		OnPanUpdate: func(d w.DragDetails) {
			s.setPos(s.pos - d.Delta.X/s.unit())
		},
		OnPanEnd: func(d w.DragDetails) { s.snap() },
		OnTapUp: func(d w.TapDetails) {
			s.node.RequestFocus()
			if c.OnTap != nil && s.ro != nil {
				if i := s.ro.itemAt(d.Local.X); i >= 0 {
					c.OnTap(i)
				}
			}
		},
		Child: body,
	}}}
}

// carouselView is the render-object widget behind Carousel.
type carouselView struct {
	state  *carouselState
	layout CarouselLayout
	extent float32
	height float32
	pos    float32
	colors []geom.Color
	radius float32
	items  []w.Widget
}

func (v carouselView) ChildWidgets() []w.Widget { return v.items }

func (v carouselView) CreateRenderObject(w.BuildContext) render.RenderObject {
	r := &renderCarousel{}
	v.UpdateRenderObject(nil, r)
	return r
}

func (carouselView) MarksOwnPaint() {}
func (v carouselView) UpdateRenderObject(_ w.BuildContext, ro render.RenderObject) {
	r := ro.(*renderCarousel)
	v.state.ro = r
	if r.layout != v.layout || r.extent != v.extent || r.height != v.height {
		r.layout, r.extent, r.height = v.layout, v.extent, v.height
		render.MarkNeedsLayout(r)
	}
	if r.pos != v.pos {
		r.pos = v.pos
		render.MarkNeedsLayout(r) // positions and masks depend on pos
	}
	r.colors, r.radius = v.colors, v.radius
	render.MarkNeedsPaint(r)
}

// renderCarousel lays items out along keylines.
type renderCarousel struct {
	render.Box
	render.MultiChild
	layout CarouselLayout
	extent float32
	height float32
	pos    float32
	colors []geom.Color
	radius float32

	unit  float32     // scroll distance per item (large size + gap)
	masks []geom.Rect // per child, local coordinates
	full  float32     // width each child is laid out at
	count int
}

const carouselGap = 8

// keyline maps a slot position (item units, 0 = the first visible slot) to
// a mask x and width.
type keyline struct{ x, w float32 }

func (r *renderCarousel) keylines(width float32) (kls []keyline, full float32) {
	small := float32(56)
	switch r.layout {
	case CarouselFullScreen:
		return nil, width
	case CarouselUncontained:
		l := r.extent
		if l <= 0 {
			l = min(max(width*0.6, 120), 320)
		}
		return nil, l
	case CarouselHero:
		l := r.extent
		if l <= 0 || l > width-small-carouselGap {
			l = max(width-small-carouselGap, 1)
		}
		return []keyline{
			{-small - carouselGap, small},
			{0, l},
			{l + carouselGap, small},
			{l + small + 2*carouselGap, small},
		}, l
	}
	// Multi-browse: large + medium + small fill the width.
	avail := width - small - 2*carouselGap
	l := r.extent
	if l <= 0 || l > avail-small {
		l = avail * 0.6
	}
	m := max(avail-l, small)
	return []keyline{
		{-small - carouselGap, small},
		{0, l},
		{l + carouselGap, m},
		{l + m + 2*carouselGap, small},
		{l + m + small + 3*carouselGap, small},
	}, l
}

func (r *renderCarousel) maxPos() float32 {
	n := float32(r.count)
	switch r.layout {
	case CarouselUncontained:
		vw := r.Size().W
		if r.unit <= 0 || vw <= 0 {
			return max(n-1, 0)
		}
		content := n*r.unit - carouselGap
		return max((content-vw)/r.unit, 0)
	case CarouselMultiBrowse:
		return max(n-2, 0) // last item ends in the medium slot
	}
	return max(n-1, 0)
}

func (r *renderCarousel) PerformLayout(c geom.Constraints) geom.Size {
	width := c.MaxW
	if !c.HasBoundedWidth() {
		width = 600
	}
	h := r.height
	if h <= 0 {
		h = 200
		if r.layout == CarouselFullScreen && c.HasBoundedHeight() {
			h = c.MaxH
		}
	}
	size := c.Constrain(geom.Size{W: width, H: h})
	kids := r.Children()
	r.count = len(kids)
	kls, full := r.keylines(size.W)
	r.full = full
	r.unit = full + carouselGap
	if r.layout == CarouselFullScreen {
		r.unit = full
	}
	r.masks = r.masks[:0]
	for i, ch := range kids {
		slot := float32(i) - r.pos
		var m geom.Rect
		if kls == nil {
			m = geom.Rect{X: slot * r.unit, W: full, H: size.H}
		} else {
			m = interpKeylines(kls, slot, size.H)
		}
		r.masks = append(r.masks, m)
		render.Layout(ch, geom.Tight(geom.Size{W: full, H: size.H}))
		// Content is centered in its mask (parallax as the mask resizes).
		render.SetOffset(ch, geom.Offset{X: m.X + (m.W-full)/2})
	}
	return size
}

// interpKeylines linearly interpolates the keyline table at slot, which is
// offset so kls[1] is slot 0; outside the table it extrapolates at the
// small size.
func interpKeylines(kls []keyline, slot, h float32) geom.Rect {
	f := slot + 1 // index into kls
	last := float32(len(kls) - 1)
	switch {
	case f <= 0:
		k := kls[0]
		return geom.Rect{X: k.x + f*(k.w+carouselGap), W: k.w, H: h}
	case f >= last:
		k := kls[len(kls)-1]
		return geom.Rect{X: k.x + (f-last)*(k.w+carouselGap), W: k.w, H: h}
	}
	i := int(f)
	t := f - float32(i)
	a, b := kls[i], kls[i+1]
	return geom.Rect{X: a.x + (b.x-a.x)*t, W: a.w + (b.w-a.w)*t, H: h}
}

// itemAt returns the index of the item whose mask contains local x.
func (r *renderCarousel) itemAt(x float32) int {
	for i, m := range r.masks {
		if x >= m.X && x < m.X+m.W {
			return i
		}
	}
	return -1
}

func (r *renderCarousel) Paint(ctx *render.PaintContext, o geom.Offset) {
	c := ctx.Canvas
	view := geom.RectFrom(o, r.Size())
	c.PushClip(view)
	kids := r.Children()
	for i, ch := range kids {
		if i >= len(r.masks) {
			break
		}
		m := r.masks[i].Translate(o)
		if m.Intersect(view).Empty() {
			continue
		}
		col := geom.Color{}
		if len(r.colors) > 0 {
			col = r.colors[i%len(r.colors)]
		}
		c.FillRoundRect(m, r.radius, col)
		c.PushClip(m)
		ctx.PaintChild(ch, o)
		c.PopClip()
	}
	c.PopClip()
}

func (r *renderCarousel) HitTestSelf(geom.Offset) bool { return true }
