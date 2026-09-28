package material

import (
	"math"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// paintShadow draws the M3 two-part (key + ambient) shadow for an elevation
// level; level may be fractional while animating.
func paintShadow(c *render.Canvas, r geom.Rect, radius, level float32, shadow geom.Color) {
	if level <= 0 {
		return
	}
	i := int(level)
	var dp float32
	if i >= len(elevationDP)-1 {
		dp = elevationDP[len(elevationDP)-1]
	} else {
		f := level - float32(i)
		dp = elevationDP[i] + (elevationDP[i+1]-elevationDP[i])*f
	}
	c.DrawShadow(r.Translate(geom.Pt(0, dp*0.5)), radius, dp*1.1+1, shadow.WithAlpha(0.16))
	c.DrawShadow(r.Translate(geom.Pt(0, dp*0.15)), radius, dp*0.4+0.5, shadow.WithAlpha(0.14))
}

// Surface is a painted container: color, shape, optional outline and
// elevation shadow (Flutter's Material widget).
type Surface struct {
	Color       geom.Color
	Radius      float32
	BorderColor geom.Color
	BorderWidth float32
	Elevation   int
	Child       w.Widget
}

func (s Surface) Build(ctx w.BuildContext) w.Widget {
	shadow := ThemeOf(ctx).Scheme.Shadow
	return w.CustomPaint{Child: s.Child, Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
		r := geom.RectFrom(o, size)
		paintShadow(c, r, s.Radius, float32(s.Elevation), shadow)
		c.FillRoundRect(r, s.Radius, s.Color)
		if s.BorderWidth > 0 {
			c.StrokeRoundRect(r, s.Radius, s.BorderWidth, s.BorderColor)
		}
	}}
}

// InkSurface is an interactive Surface: hover/focus state layers, press
// ripples clipped to the shape, keyboard activation and a focus ring. All
// tappable Material components are built on it.
type InkSurface struct {
	OnTap   func()
	OnHover func(bool)

	Color        geom.Color // container fill (may be transparent)
	ContentColor geom.Color // state layer / ripple color
	BorderColor  geom.Color
	BorderWidth  float32
	Radius       float32
	Elevation    int
	RaiseOnHover bool // +1 elevation level while hovered (elevated buttons, FABs)
	Disabled     bool // no interaction (OnTap == nil also disables)
	NoFocus      bool // exclude from keyboard focus
	Child        w.Widget
}

func (InkSurface) CreateState() w.State { return &inkState{} }

type ripple struct {
	center geom.Offset // local
	grow   *w.AnimationController
	fade   *w.AnimationController
	up     bool
}

type inkState struct {
	w.StateBase
	hovered, pressed, focused bool
	hoverA, focusA, elev      *w.Animated
	ripples                   []*ripple
	node                      *w.FocusNode
	size                      geom.Size
}

func (s *inkState) widget() InkSurface { return w.WidgetOf[InkSurface](s) }

func (s *inkState) enabled() bool { ww := s.widget(); return ww.OnTap != nil && !ww.Disabled }

func (s *inkState) InitState() {
	ww := s.widget()
	s.hoverA = w.NewAnimated(s, 150*time.Millisecond, w.Standard, 0)
	s.focusA = w.NewAnimated(s, 150*time.Millisecond, w.Standard, 0)
	s.elev = w.NewAnimated(s, 150*time.Millisecond, w.Standard, float32(ww.Elevation))
	s.node = &w.FocusNode{
		OnFocusChange: func(f bool) { s.SetState(func() { s.focused = f }) },
		OnKey: func(e w.KeyEvent) bool {
			if (e.Key == w.KeyEnter || e.Key == w.KeySpace) && s.enabled() {
				s.startRipple(geom.Pt(s.size.W/2, s.size.H/2))
				s.release()
				s.widget().OnTap()
				return true
			}
			return false
		},
	}
}

func (s *inkState) startRipple(at geom.Offset) {
	rp := &ripple{center: at}
	rp.grow = w.NewAnimationController(s, 450*time.Millisecond)
	rp.grow.Curve = w.EaseOut
	rp.fade = w.NewAnimationController(s, 300*time.Millisecond)
	rp.fade.SetValue(1)
	rp.fade.OnStatus = func(st w.AnimationStatus) {
		if st == w.Dismissed {
			for i, x := range s.ripples {
				if x == rp {
					s.ripples = append(s.ripples[:i], s.ripples[i+1:]...)
					break
				}
			}
		}
	}
	s.ripples = append(s.ripples, rp)
	rp.grow.Forward()
}

// release starts fading the ripples once the pointer is up.
func (s *inkState) release() {
	for _, rp := range s.ripples {
		if !rp.up {
			rp.up = true
			rp.fade.Reverse()
		}
	}
}

func (s *inkState) Dispose() {
	for _, rp := range s.ripples {
		rp.grow.Dispose()
		rp.fade.Dispose()
	}
}

func (s *inkState) Build(ctx w.BuildContext) w.Widget {
	ww := s.widget()
	th := ThemeOf(ctx)
	enabled := s.enabled()

	hover := float32(0)
	if s.hovered && enabled {
		hover = 1
	}
	s.hoverA.Set(hover)
	focus := float32(0)
	if s.focused && enabled {
		focus = 1
	}
	s.focusA.Set(focus)
	el := float32(ww.Elevation)
	if ww.RaiseOnHover && s.hovered && enabled && !s.pressed {
		el++
	}
	s.elev.Set(el)

	shadow, ring := th.Scheme.Shadow, th.Scheme.Secondary
	painter := func(c *render.Canvas, o geom.Offset, size geom.Size) {
		s.size = size
		ww := s.widget()
		r := geom.RectFrom(o, size)
		rad := min(ww.Radius, size.W/2, size.H/2)
		paintShadow(c, r, rad, s.elev.Value(), shadow)
		c.FillRoundRect(r, rad, ww.Color)
		if ww.BorderWidth > 0 && ww.BorderColor.A > 0 {
			c.StrokeRoundRect(r, rad, ww.BorderWidth, ww.BorderColor)
		}
		layer := max(s.hoverA.Value()*HoverOpacity, s.focusA.Value()*FocusOpacity)
		if layer > 0 {
			c.FillRoundRect(r, rad, ww.ContentColor.WithAlpha(ww.ContentColor.A*layer))
		}
		maxR := float32(math.Hypot(float64(size.W), float64(size.H)))
		for _, rp := range s.ripples {
			rr := maxR * (0.3 + 0.7*rp.grow.Value())
			a := PressedOpacity * rp.fade.Value()
			ctr := o.Add(rp.center)
			c.FillRipple(r, rad, ctr, rr, ww.ContentColor.WithAlpha(ww.ContentColor.A*a))
		}
		if f := s.focusA.Value(); f > 0 {
			ringR := geom.Rect{X: r.X - 3, Y: r.Y - 3, W: r.W + 6, H: r.H + 6}
			c.StrokeRoundRect(ringR, rad+3, 3, ring.WithAlpha(f))
		}
	}

	var child w.Widget = w.CustomPaint{Painter: painter, Child: ww.Child}
	if !enabled {
		return child
	}
	child = w.GestureDetector{
		OnTapDown: func(d w.TapDetails) {
			s.SetState(func() { s.pressed = true })
			s.startRipple(d.Local)
		},
		OnTapCancel: func() {
			s.SetState(func() { s.pressed = false })
			s.release()
		},
		OnTap: func() {
			s.SetState(func() { s.pressed = false })
			s.release()
			if cb := s.widget().OnTap; cb != nil {
				cb()
			}
		},
		Child: child,
	}
	child = w.MouseRegion{
		Cursor: w.CursorPointer,
		OnEnter: func(w.PointerEvent) {
			s.SetState(func() { s.hovered = true })
			if cb := s.widget().OnHover; cb != nil {
				cb(true)
			}
		},
		OnExit: func(w.PointerEvent) {
			s.SetState(func() { s.hovered = false })
			if cb := s.widget().OnHover; cb != nil {
				cb(false)
			}
		},
		Child: child,
	}
	if ww.NoFocus {
		return child
	}
	return w.Focus{Node: s.node, Child: child}
}

// disabledColors returns the M3 disabled content / container colors.
func disabledColors(s ColorScheme) (content, container geom.Color) {
	return s.OnSurface.WithAlpha(DisabledContentOpacity), s.OnSurface.WithAlpha(DisabledContainerOpacity)
}
