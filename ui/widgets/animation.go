package widgets

import (
	"math"
	"time"
)

// ---------------------------------------------------------------------------
// Tickers: per-frame callbacks driven by the BuildOwner.

// Ticker calls OnTick once per frame with the time since Start while active.
type Ticker struct {
	owner  *BuildOwner
	onTick func(elapsed time.Duration)
	start  time.Time
	active bool
}

// NewTicker creates a stopped ticker.
func (o *BuildOwner) NewTicker(onTick func(elapsed time.Duration)) *Ticker {
	return &Ticker{owner: o, onTick: onTick}
}

// Start begins ticking from the next frame.
func (t *Ticker) Start() {
	if t.active {
		return
	}
	t.active = true
	t.start = time.Time{} // set on the first tick
	o := t.owner
	if o.tickers == nil {
		o.tickers = map[*Ticker]struct{}{}
	}
	o.tickers[t] = struct{}{}
	o.requestFrame()
}

// Stop stops ticking.
func (t *Ticker) Stop() {
	if !t.active {
		return
	}
	t.active = false
	delete(t.owner.tickers, t)
}

// Active reports whether the ticker is running.
func (t *Ticker) Active() bool { return t.active }

// HasActiveTickers reports whether an animation needs more frames.
func (o *BuildOwner) HasActiveTickers() bool { return len(o.tickers) > 0 }

func (o *BuildOwner) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// tick advances every active ticker (called at the start of FlushBuild).
func (o *BuildOwner) tick() {
	if len(o.tickers) == 0 {
		return
	}
	now := o.now()
	ts := make([]*Ticker, 0, len(o.tickers))
	for t := range o.tickers {
		ts = append(ts, t)
	}
	for _, t := range ts {
		if !t.active {
			continue
		}
		if t.start.IsZero() {
			t.start = now
		}
		t.onTick(now.Sub(t.start))
	}
}

// ---------------------------------------------------------------------------
// Curves

// Curve maps linear progress t∈[0,1] to eased progress.
type Curve func(t float32) float32

// CubicBezier returns a CSS-style cubic-bezier(x1, y1, x2, y2) curve.
func CubicBezier(x1, y1, x2, y2 float32) Curve {
	return func(t float32) float32 {
		if t <= 0 {
			return 0
		}
		if t >= 1 {
			return 1
		}
		bez := func(a, b, s float32) float32 {
			u := 1 - s
			return 3*u*u*s*a + 3*u*s*s*b + s*s*s
		}
		// Solve x(s) = t by bisection (robust, fast enough for UI).
		lo, hi := float32(0), float32(1)
		s := t
		for i := 0; i < 24; i++ {
			x := bez(x1, x2, s)
			if float32(math.Abs(float64(x-t))) < 1e-4 {
				break
			}
			if x < t {
				lo = s
			} else {
				hi = s
			}
			s = (lo + hi) / 2
		}
		return bez(y1, y2, s)
	}
}

var (
	Linear    Curve = func(t float32) float32 { return t }
	EaseIn          = CubicBezier(0.42, 0, 1, 1)
	EaseOut         = CubicBezier(0, 0, 0.58, 1)
	EaseInOut       = CubicBezier(0.42, 0, 0.58, 1)

	// Material 3 motion easing.
	Emphasized           = CubicBezier(0.2, 0, 0, 1)
	EmphasizedDecelerate = CubicBezier(0.05, 0.7, 0.1, 1)
	EmphasizedAccelerate = CubicBezier(0.3, 0, 0.8, 0.15)
	Standard             = CubicBezier(0.2, 0, 0, 1)
	StandardDecelerate   = CubicBezier(0, 0, 0, 1)
	StandardAccelerate   = CubicBezier(0.3, 0, 1, 1)
)

// ---------------------------------------------------------------------------
// AnimationController

// AnimationStatus describes where an animation is.
type AnimationStatus uint8

const (
	Dismissed AnimationStatus = iota // at 0
	Forward                          // running towards 1
	Reverse                          // running towards 0
	Completed                        // at 1
)

// AnimationController drives a value from 0 to 1 over Duration. Create it in
// InitState with NewAnimationController(state, d): it rebuilds the state on
// every tick and stops by itself once the state is disposed.
type AnimationController struct {
	Duration time.Duration
	Curve    Curve // applied by Value(); nil = Linear

	// OnStatus is called when the status changes.
	OnStatus func(AnimationStatus)

	value    float32
	from, to float32
	status   AnimationStatus
	repeat   bool
	reverse  bool // for repeat: ping-pong
	ticker   *Ticker
	state    State
	listener func()
}

// NewAnimationController creates a controller bound to s (rebuilds s on tick).
func NewAnimationController(s State, d time.Duration) *AnimationController {
	c := &AnimationController{Duration: d, state: s}
	return c
}

// AddListener replaces the per-tick callback (default: rebuild the state).
func (c *AnimationController) AddListener(fn func()) { c.listener = fn }

func (c *AnimationController) ensureTicker() bool {
	if c.ticker != nil {
		return true
	}
	sb := c.state.stateBase()
	if sb.element == nil || sb.element.owner == nil {
		return false
	}
	c.ticker = sb.element.owner.NewTicker(c.onTick)
	return true
}

// Value returns the curved value in [0,1].
func (c *AnimationController) Value() float32 {
	if c.Curve != nil {
		return c.Curve(c.value)
	}
	return c.value
}

// RawValue returns the linear value.
func (c *AnimationController) RawValue() float32 { return c.value }

// Status returns the current status.
func (c *AnimationController) Status() AnimationStatus { return c.status }

// IsAnimating reports whether a tween is running.
func (c *AnimationController) IsAnimating() bool { return c.ticker != nil && c.ticker.Active() }

// SetValue jumps to v and stops.
func (c *AnimationController) SetValue(v float32) {
	c.Stop()
	c.value = clamp01(v)
	switch c.value {
	case 0:
		c.setStatus(Dismissed)
	case 1:
		c.setStatus(Completed)
	}
}

// Forward animates to 1.
func (c *AnimationController) Forward() { c.AnimateTo(1) }

// Reverse animates to 0.
func (c *AnimationController) Reverse() { c.AnimateTo(0) }

// Toggle animates towards whichever end it's not heading to.
func (c *AnimationController) Toggle() {
	if c.status == Forward || c.status == Completed {
		c.Reverse()
	} else {
		c.Forward()
	}
}

// AnimateTo animates from the current value to target. The time taken is
// proportional to the distance, so reversing mid-way looks natural.
func (c *AnimationController) AnimateTo(target float32) {
	target = clamp01(target)
	c.repeat = false
	c.start(target)
}

// Repeat loops 0→1 forever (ping-pong if reverse is set).
func (c *AnimationController) Repeat(reverse bool) {
	c.repeat, c.reverse = true, reverse
	c.value = 0
	c.start(1)
}

func (c *AnimationController) start(target float32) {
	if target == c.value && !c.repeat {
		c.Stop()
		if target == 1 {
			c.setStatus(Completed)
		} else if target == 0 {
			c.setStatus(Dismissed)
		}
		return
	}
	if !c.ensureTicker() {
		c.value = target
		return
	}
	c.from, c.to = c.value, target
	if target > c.value {
		c.setStatus(Forward)
	} else {
		c.setStatus(Reverse)
	}
	c.ticker.Stop()
	c.ticker.Start()
}

// Stop halts the animation where it is.
func (c *AnimationController) Stop() {
	if c.ticker != nil {
		c.ticker.Stop()
	}
}

// Dispose stops the controller for good.
func (c *AnimationController) Dispose() { c.Stop() }

func (c *AnimationController) onTick(elapsed time.Duration) {
	if !c.state.stateBase().Mounted() {
		c.Stop()
		return
	}
	span := float32(math.Abs(float64(c.to - c.from)))
	d := time.Duration(float32(c.Duration) * span)
	var t float32 = 1
	if d > 0 {
		t = min(float32(elapsed)/float32(d), 1)
	}
	c.value = c.from + (c.to-c.from)*t
	if t >= 1 {
		c.value = c.to
		if c.repeat {
			if c.reverse {
				c.from, c.to = c.to, c.from
			} else {
				c.value = 0
			}
			c.ticker.Stop()
			c.ticker.Start()
		} else {
			c.ticker.Stop()
			if c.to >= 1 {
				c.setStatus(Completed)
			} else if c.to <= 0 {
				c.setStatus(Dismissed)
			}
		}
	}
	if c.listener != nil {
		c.listener()
	} else {
		c.state.stateBase().SetState(nil)
	}
}

func (c *AnimationController) setStatus(s AnimationStatus) {
	if c.status == s {
		return
	}
	c.status = s
	if c.OnStatus != nil {
		c.OnStatus(s)
	}
}

func clamp01(v float32) float32 { return min(max(v, 0), 1) }

// ---------------------------------------------------------------------------
// Animated: implicit animation of a single float.

// Animated smoothly follows a target value: call Set(target) (in Build or
// an event handler) and read Value(). Create it in InitState.
type Animated struct {
	c        *AnimationController
	from, to float32
}

// NewAnimated creates an implicit animation bound to s, starting at initial.
func NewAnimated(s State, d time.Duration, curve Curve, initial float32) *Animated {
	c := NewAnimationController(s, d)
	c.Curve = curve
	c.SetValue(1)
	return &Animated{c: c, from: initial, to: initial}
}

// Set animates towards target (no-op if already heading there).
func (a *Animated) Set(target float32) {
	if target == a.to {
		return
	}
	a.from = a.Value()
	a.to = target
	a.c.SetValue(0)
	a.c.Forward()
}

// Jump sets the value immediately.
func (a *Animated) Jump(v float32) {
	a.from, a.to = v, v
	a.c.SetValue(1)
}

// Value returns the current (eased) value.
func (a *Animated) Value() float32 {
	t := a.c.Value()
	return a.from + (a.to-a.from)*t
}

// Target returns the value being animated to.
func (a *Animated) Target() float32 { return a.to }

// IsAnimating reports whether the value is still moving.
func (a *Animated) IsAnimating() bool { return a.c.IsAnimating() }

// Lerp interpolates between a and b.
func Lerp(a, b, t float32) float32 { return a + (b-a)*t }
