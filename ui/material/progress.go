package material

import (
	"math"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// LinearProgressIndicator shows progress along a line. Indeterminate
// animates continuously.
type LinearProgressIndicator struct {
	Value         float32 // 0..1
	Indeterminate bool
	// Style overrides Theme.Progress.
	Style ProgressIndicatorTheme
}

func (LinearProgressIndicator) CreateState() w.State { return &progressState{} }

type progressState struct {
	w.StateBase
	loop *w.AnimationController
}

func (s *progressState) ensureLoop(on bool, d time.Duration) {
	if on && s.loop == nil {
		s.loop = w.NewAnimationController(s, d)
		s.loop.Repeat(false)
	}
	if !on && s.loop != nil {
		s.loop.Dispose()
		s.loop = nil
	}
}

func (s *progressState) Dispose() {
	if s.loop != nil {
		s.loop.Dispose()
	}
}

func (s *progressState) Build(ctx w.BuildContext) w.Widget {
	p := w.WidgetOf[LinearProgressIndicator](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Progress, p.Style)
	s.ensureLoop(p.Indeterminate, 1800*time.Millisecond)
	active, track := pick(st.Color, sc.Primary), pick(st.TrackColor, sc.SecondaryContainer)
	h := pickF(st.LinearHeight, 4)
	return w.CustomPaint{Size: geom.Sz(geom.Inf, h), Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
		r := geom.Rect{X: o.X, Y: o.Y, W: size.W, H: h}
		c.PushClip(r)
		c.FillRoundRect(r, h/2, track)
		if s.loop != nil {
			t := s.loop.Value()
			seg := func(a, b float32) {
				a, b = min(max(a, 0), 1), min(max(b, 0), 1)
				if b > a {
					c.FillRoundRect(geom.Rect{X: o.X + a*size.W, Y: o.Y, W: (b - a) * size.W, H: h}, h/2, active)
				}
			}
			// Two bars chasing each other (M3 indeterminate linear).
			e := w.EaseInOut
			seg(e(t)*1.4-0.4, e(min(t*1.2, 1))*1.4-0.1)
			t2 := float32(math.Mod(float64(t)+0.5, 1))
			seg(e(t2)*1.6-0.8, e(t2)*1.6-0.4)
		} else {
			v := min(max(p.Value, 0), 1)
			c.FillRoundRect(geom.Rect{X: o.X, Y: o.Y, W: v * size.W, H: h}, h/2, active)
		}
		c.PopClip()
	}}
}

// CircularProgressIndicator shows progress as an arc.
type CircularProgressIndicator struct {
	Value         float32
	Indeterminate bool
	Size          float32 // default Style / 48
	StrokeWidth   float32 // default Style / 4
	// Style overrides Theme.Progress.
	Style ProgressIndicatorTheme
}

func (CircularProgressIndicator) CreateState() w.State { return &circularState{} }

type circularState struct{ progressState }

func (s *circularState) Build(ctx w.BuildContext) w.Widget {
	p := w.WidgetOf[CircularProgressIndicator](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Progress, p.Style)
	s.ensureLoop(p.Indeterminate, 1333*time.Millisecond)
	size := p.Size
	if size == 0 {
		size = pickF(st.CircularSize, 48)
	}
	sw := p.StrokeWidth
	if sw == 0 {
		sw = pickF(st.StrokeWidth, 4)
	}
	active, track := pick(st.Color, sc.Primary), pick(st.TrackColor, sc.SecondaryContainer)
	return w.CustomPaint{Size: geom.Sz(size, size), Painter: func(c *render.Canvas, o geom.Offset, sz geom.Size) {
		d := min(sz.W, sz.H) - 4
		r := geom.Rect{X: o.X + (sz.W-d)/2, Y: o.Y + (sz.H-d)/2, W: d, H: d}
		const tau = 2 * math.Pi
		if s.loop != nil {
			t := s.loop.Value()
			// Sweep grows then shrinks while the whole arc rotates.
			grow := w.EaseInOut(float32(math.Min(float64(t*2), 1)))
			shrink := w.EaseInOut(float32(math.Max(float64(t*2-1), 0)))
			sweep := 0.1 + 0.75*tau*(grow-shrink)
			start := -math.Pi/2 + float64(t)*tau*1.5 + float64(shrink)*0.75*tau
			c.StrokeArc(r, float32(start), float32(sweep), sw, active)
			return
		}
		v := min(max(p.Value, 0), 1)
		c.StrokeCircle(geom.Pt(r.X+d/2, r.Y+d/2), d/2, sw, track)
		if v > 0 {
			c.StrokeArc(r, -math.Pi/2, v*tau, sw, active)
		}
	}}
}
