package widgets

import (
	"math"
	"time"

	"github.com/minelifes/nectar_ui/ui/render"
)

func resolvePhysics(ctx BuildContext, p ScrollPhysics) ScrollPhysics {
	return ResolveScrollPhysics(ctx, p)
}

// OverscrollFriction is the share of a drag past an edge that bouncing
// content follows, for an overscroll of over pixels in a viewport extent
// pixels long (it falls as the overscroll grows). For custom scrollables.
func OverscrollFriction(over, extent float32) float32 {
	if extent <= 0 {
		return 0.5
	}
	f := min(abs32(over)/extent, 1)
	return 0.52 * (1 - f) * (1 - f)
}

// SpringBack returns the position at time t (seconds) of an overscroll
// springing back to 0 from x0 with initial velocity v0 (px/s), and whether
// it has settled. For custom scrollables; scroll views use it after a drag.
func SpringBack(x0, v0 float32, t float64) (x float32, done bool) {
	w := springRate
	v0 = max(-bounceMaxSpeed, min(v0, bounceMaxSpeed))
	e := math.Exp(-w * t)
	xx := (float64(x0) + (float64(v0)+w*float64(x0))*t) * e
	vel := (float64(v0) - w*(float64(v0)+w*float64(x0))*t) * e
	if t > 0.05 && math.Abs(xx) < 0.5 && math.Abs(vel) < 20 {
		return 0, true
	}
	return float32(xx), false
}

// ScrollPhysics is how a scroll view behaves at its edges and after a
// drag. Every scrollable (ScrollView, ListView, ListViewBuilder,
// ScrollView2D, and the Material widgets built on them) uses the Physics
// it's given, else the nearest ScrollConfiguration's, else
// DefaultScrollPhysics.
type ScrollPhysics int

const (
	// InheritPhysics (the zero value) uses ScrollConfiguration /
	// DefaultScrollPhysics.
	InheritPhysics ScrollPhysics = iota
	// BouncingScrollPhysics (iOS / macOS): past an edge the content follows
	// the pointer with growing resistance and springs back on release; a
	// fling that hits an edge bounces. Lists too short to scroll don't
	// bounce.
	BouncingScrollPhysics
	// ClampingScrollPhysics stops hard at the edges.
	ClampingScrollPhysics
)

// DefaultScrollPhysics is what scroll views use when neither they nor a
// ScrollConfiguration above them set Physics. Set it at startup.
var DefaultScrollPhysics = BouncingScrollPhysics

// ScrollConfiguration sets the physics of every scroll view below it that
// doesn't set its own:
//
//	widgets.ScrollConfiguration{Physics: widgets.ClampingScrollPhysics, Child: page}
type ScrollConfiguration struct {
	Physics ScrollPhysics
	Child   Widget
}

func (w ScrollConfiguration) ChildWidget() Widget { return w.Child }
func (w ScrollConfiguration) UpdateShouldNotify(old Widget) bool {
	return old.(ScrollConfiguration).Physics != w.Physics
}

// ResolveScrollPhysics picks p, else the nearest ScrollConfiguration's,
// else DefaultScrollPhysics: for custom scrollables.
func ResolveScrollPhysics(ctx BuildContext, p ScrollPhysics) ScrollPhysics {
	if p != InheritPhysics {
		return p
	}
	if c, ok := DependOn[ScrollConfiguration](ctx); ok && c.Physics != InheritPhysics {
		return c.Physics
	}
	if DefaultScrollPhysics == InheritPhysics {
		return BouncingScrollPhysics
	}
	return DefaultScrollPhysics
}

// Tuning (pixels and seconds).
const (
	flingDecay     = 2.0  // velocity e-folding rate of a fling, 1/s (iOS "normal")
	flingMinSpeed  = 60   // slower releases don't fling, px/s
	flingStopSpeed = 15   // a fling ends below this, px/s
	flingMaxSpeed  = 8000 // cap, px/s
	springRate     = 14.0 // critically damped spring back, 1/s (~0.4 s)
	bounceMaxSpeed = 1600 // speed a fling carries past an edge, px/s (≈40 px overshoot)
	wheelIdle      = 120 * time.Millisecond
	velocityWindow = 80 * time.Millisecond
)

type scrollSample struct {
	t   time.Time
	pos float32
}

// Overscroll returns how far the content is pulled past an edge by
// bouncing physics: negative past the start, positive past the end, 0
// otherwise (e.g. for pull-to-refresh).
func (c *ScrollController) Overscroll() float32 { return c.over }

func (c *ScrollController) bounces() bool {
	return c.physics != ClampingScrollPhysics && c.vp != nil && c.MaxOffset() > 0.5
}

func (c *ScrollController) now() time.Time {
	if c.owner != nil {
		return c.owner.now()
	}
	return time.Now()
}

func (c *ScrollController) setOver(v float32) {
	if v == c.over {
		return
	}
	c.over = v
	if o, ok := c.vp.(render.Overscroller); ok {
		o.SetOverscroll(v)
	}
	c.notify()
}

// friction is the share of a drag past the edge that the content follows:
// it falls as the overscroll grows (Flutter's BouncingScrollPhysics).
func (c *ScrollController) friction() float32 {
	return OverscrollFriction(c.over, c.ViewportExtent())
}

// pull applies d to the overscroll: moving back toward the content is
// followed 1:1 and returns what's left after reaching 0; moving further
// out is damped by friction.
func (c *ScrollController) pull(d float32) (rest float32) {
	if c.over == 0 || d == 0 {
		return d
	}
	if (d < 0) != (c.over < 0) { // toward the content
		nv := c.over + d
		if (nv < 0) != (c.over < 0) || nv == 0 {
			c.setOver(0)
			return nv
		}
		c.setOver(nv)
		return 0
	}
	c.setOver(c.over + d*c.friction())
	return 0
}

// --- drag -----------------------------------------------------------------

func (c *ScrollController) dragStart() {
	c.stopAnim()
	c.stopSim()
	c.samples = c.samples[:0]
	c.sample()
}

// dragBy moves by d (positive = toward the end) following the physics.
func (c *ScrollController) dragBy(d float32) {
	c.stopAnim()
	c.stopSim()
	d = c.pull(d)
	if d != 0 {
		target := c.Offset() + d
		c.set(target)
		if rest := target - c.Offset(); rest != 0 && c.bounces() {
			c.setOver(rest * c.friction())
		}
	}
	c.sample()
}

func (c *ScrollController) dragEnd() {
	c.sample()
	v := c.velocity()
	c.samples = c.samples[:0]
	switch {
	case c.over != 0:
		c.springBack(v)
	case abs32(v) >= flingMinSpeed:
		c.fling(v)
	}
}

func (c *ScrollController) sample() {
	now := c.now()
	c.samples = append(c.samples, scrollSample{now, c.Offset() + c.over})
	// Keep only what the velocity window needs.
	i := 0
	for i < len(c.samples)-2 && now.Sub(c.samples[i].t) > velocityWindow {
		i++
	}
	if i > 0 {
		c.samples = append(c.samples[:0], c.samples[i:]...)
	}
}

// velocity is the recent speed of the drag, px/s (0 if it paused).
func (c *ScrollController) velocity() float32 {
	n := len(c.samples)
	if n < 2 {
		return 0
	}
	last := c.samples[n-1]
	first := last
	for i := n - 2; i >= 0; i-- {
		if last.t.Sub(c.samples[i].t) > velocityWindow {
			break
		}
		first = c.samples[i]
	}
	dt := last.t.Sub(first.t).Seconds()
	if dt <= 0.004 {
		return 0
	}
	v := float32(float64(last.pos-first.pos) / dt)
	return max(-flingMaxSpeed, min(v, flingMaxSpeed))
}

// --- wheel / trackpad -------------------------------------------------------

// wheel scrolls by d for a scroll event and reports whether it moved (the
// event is then claimed). At an edge, with bouncing physics, the content is
// pulled past it only if no other scroll view takes the event (checked
// after the event went to all of them), and springs back once the wheel
// stops.
func (c *ScrollController) wheel(d float32, e PointerEvent) bool {
	c.stopAnim()
	if c.over != 0 {
		c.stopSim()
		if d = c.pull(d); d == 0 {
			c.wheelIdle()
			return true
		}
	}
	target := c.Offset() + d
	moved := c.set(target)
	rest := target - c.Offset()
	if rest != 0 && c.bounces() && c.owner != nil {
		vp := c.vp.Viewport()
		c.owner.Post(func() {
			if (e.Winner() == nil || e.Winner() == vp) && e.Claim(vp) {
				c.stopSim()
				c.setOver(c.over + rest*c.friction()*0.5)
				c.wheelIdle()
			}
		})
	}
	return moved
}

// wheelIdle springs back once no scroll event came for wheelIdle.
func (c *ScrollController) wheelIdle() {
	c.lastWheel = c.now()
	if c.owner == nil || (c.sim != nil && c.sim.Active() && c.simWait) {
		return
	}
	c.stopSim()
	c.simWait = true
	c.sim = c.owner.NewTicker(func(time.Duration) {
		if c.now().Sub(c.lastWheel) >= wheelIdle {
			c.springBack(0)
		}
	})
	c.sim.Start()
}

// --- simulations ------------------------------------------------------------

func (c *ScrollController) stopSim() {
	if c.sim != nil {
		c.sim.Stop()
		c.sim = nil
	}
	c.simWait = false
}

// fling coasts at velocity v (px/s), slowing down exponentially. Hitting
// an edge bounces (bouncing physics) or stops.
func (c *ScrollController) fling(v float32) {
	if c.owner == nil {
		return
	}
	c.stopSim()
	start := c.Offset()
	v0 := float64(v)
	c.sim = c.owner.NewTicker(func(el time.Duration) {
		t := el.Seconds()
		decay := math.Exp(-flingDecay * t)
		target := start + float32(v0/flingDecay*(1-decay))
		vel := float32(v0 * decay)
		c.set(target)
		if target != c.Offset() { // ran into an edge
			if c.bounces() {
				c.springBack(vel)
			} else {
				c.stopSim()
			}
			return
		}
		if abs32(vel) < flingStopSpeed {
			c.stopSim()
		}
	})
	c.sim.Start()
}

// springBack returns the overscroll to 0 with a critically damped spring,
// starting with velocity v (px/s; a fling arriving at an edge carries on
// past it for a moment).
func (c *ScrollController) springBack(v float32) {
	if c.owner == nil {
		c.setOver(0)
		return
	}
	c.stopSim()
	x0, v0 := c.over, v
	if x0 == 0 && v0 == 0 {
		return
	}
	c.sim = c.owner.NewTicker(func(el time.Duration) {
		x, done := SpringBack(x0, v0, el.Seconds())
		c.setOver(x)
		if done {
			c.stopSim()
		}
	})
	c.sim.Start()
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
