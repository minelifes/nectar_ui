package material

import (
	"fmt"
	"math"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// ---------------------------------------------------------------------------
// Checkbox

// Checkbox is an 18px box in a 40px touch target.
type Checkbox struct {
	Value         bool
	Indeterminate bool // shows a dash (tristate)
	Error         bool
	OnChanged     func(bool) // nil = disabled
	// Style overrides Theme.Checkbox.
	Style CheckboxTheme
}

func (Checkbox) CreateState() w.State { return &checkboxState{} }

type checkboxState struct {
	w.StateBase
	on *w.Animated
}

func (s *checkboxState) InitState() {
	cb := w.WidgetOf[Checkbox](s)
	v := float32(0)
	if cb.Value || cb.Indeterminate {
		v = 1
	}
	s.on = w.NewAnimated(s, 120*time.Millisecond, w.Standard, v)
}

func (s *checkboxState) Build(ctx w.BuildContext) w.Widget {
	cb := w.WidgetOf[Checkbox](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Checkbox, cb.Style)
	checked := cb.Value || cb.Indeterminate
	if checked {
		s.on.Set(1)
	} else {
		s.on.Set(0)
	}
	fill, mark, border := pick(st.FillColor, sc.Primary), pick(st.CheckColor, sc.OnPrimary), pick(st.BorderColor, sc.OnSurfaceVariant)
	if cb.Error {
		e := pick(st.ErrorColor, sc.Error)
		fill, mark, border = e, sc.OnError, e
	}
	side, radius, bw := pickF(st.Size, 18), pickF(st.Radius, 2), pickF(st.BorderWidth, 2)
	if cb.OnChanged == nil {
		dc, _ := disabledColors(sc)
		fill, border, mark = dc, dc, sc.Surface
	}
	ind := cb.Indeterminate
	var tap func()
	if cb.OnChanged != nil {
		tap = func() { cb.OnChanged(!cb.Value) }
	}
	state := sc.OnSurface
	if checked {
		state = fill
	}
	state = pick(st.OverlayColor, state)
	target := max(40, side+22)
	return InkSurface{OnTap: tap, ContentColor: state, Radius: CornerFull, Child: w.CustomPaint{Size: geom.Sz(target, target),
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			box := geom.Rect{X: o.X + (size.W-side)/2, Y: o.Y + (size.H-side)/2, W: side, H: side}
			t := s.on.Value()
			if t < 1 && bw > 0 {
				c.StrokeRoundRect(box, radius, bw, border.WithAlpha(border.A*(1-t)))
			}
			if t > 0 {
				c.FillRoundRect(box, radius, fill.WithAlpha(fill.A*t))
				k := side / 18
				if ind {
					c.FillRect(geom.Rect{X: box.X + 4*k, Y: box.Y + 8*k, W: 10 * k, H: 2 * k}, mark.WithAlpha(t))
				} else {
					c.DrawIcon(iconCheck, geom.Rect{X: box.X + k, Y: box.Y + k, W: 16 * k, H: 16 * k}, mark.WithAlpha(t))
				}
			}
		}}}
}

// ---------------------------------------------------------------------------
// Radio

// Radio is one option of a group: selected when Value == GroupValue.
type Radio[T comparable] struct {
	Value      T
	GroupValue T
	OnChanged  func(T) // nil = disabled
	// Style overrides Theme.Radio.
	Style RadioTheme
}

func (r Radio[T]) CreateState() w.State { return &radioState[T]{} }

type radioState[T comparable] struct {
	w.StateBase
	on *w.Animated
}

func (s *radioState[T]) InitState() {
	r := w.WidgetOf[Radio[T]](s)
	v := float32(0)
	if r.Value == r.GroupValue {
		v = 1
	}
	s.on = w.NewAnimated(s, 150*time.Millisecond, w.Standard, v)
}

func (s *radioState[T]) Build(ctx w.BuildContext) w.Widget {
	r := w.WidgetOf[Radio[T]](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Radio, r.Style)
	sel := r.Value == r.GroupValue
	if sel {
		s.on.Set(1)
	} else {
		s.on.Set(0)
	}
	on, off := pick(st.FillColor, sc.Primary), pick(st.UnselectedColor, sc.OnSurfaceVariant)
	ring := pickF(st.Radius, 10)
	if r.OnChanged == nil {
		on, _ = disabledColors(sc)
		off = on
	}
	var tap func()
	if r.OnChanged != nil {
		tap = func() { r.OnChanged(r.Value) }
	}
	state := sc.OnSurface
	if sel {
		state = on
	}
	target := max(40, 2*ring+20)
	return InkSurface{OnTap: tap, ContentColor: pick(st.OverlayColor, state), Radius: CornerFull, Child: w.CustomPaint{Size: geom.Sz(target, target),
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			ctr := geom.Pt(o.X+size.W/2, o.Y+size.H/2)
			t := s.on.Value()
			col := LerpColor(off, on, t)
			c.StrokeCircle(ctr, ring, 2, col)
			if t > 0 {
				c.FillCircle(ctr, ring/2*t, on)
			}
		}}}
}

// ---------------------------------------------------------------------------
// Switch

// Switch toggles a setting on or off.
type Switch struct {
	Value     bool
	OnChanged func(bool) // nil = disabled
	// ThumbIcon shows an icon (e.g. a check) in the thumb when on.
	ThumbIcon bool
	// Style overrides Theme.Switch.
	Style SwitchTheme
}

func (Switch) CreateState() w.State { return &switchState{} }

type switchState struct {
	w.StateBase
	pos              *w.Animated
	grow             *w.Animated
	hovered, pressed bool
	dragging         bool
	dragPos          float32
}

func (s *switchState) InitState() {
	v := float32(0)
	if w.WidgetOf[Switch](s).Value {
		v = 1
	}
	s.pos = w.NewAnimated(s, 200*time.Millisecond, w.Standard, v)
	s.grow = w.NewAnimated(s, 100*time.Millisecond, w.Standard, 0)
}

func (s *switchState) Build(ctx w.BuildContext) w.Widget {
	sw := w.WidgetOf[Switch](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Switch, sw.Style)
	if !s.dragging {
		if sw.Value {
			s.pos.Set(1)
		} else {
			s.pos.Set(0)
		}
	}
	if s.pressed {
		s.grow.Set(1)
	} else {
		s.grow.Set(0)
	}
	enabled := sw.OnChanged != nil
	thumbIcon := sw.ThumbIcon
	paint := func(c *render.Canvas, o geom.Offset, size geom.Size) {
		t := s.pos.Value()
		if s.dragging {
			t = s.dragPos
		}
		track := geom.Rect{X: o.X + (size.W-52)/2, Y: o.Y + (size.H-32)/2, W: 52, H: 32}
		offTrack, onTrack := pick(st.InactiveTrackColor, sc.SurfaceContainerHighest), pick(st.TrackColor, sc.Primary)
		offThumb, onThumb := pick(st.InactiveThumbColor, sc.Outline), pick(st.ThumbColor, sc.OnPrimary)
		border := pick(st.TrackOutlineColor, sc.Outline)
		if !enabled {
			_, dbg := disabledColors(sc)
			offTrack, onTrack = dbg, sc.OnSurface.WithAlpha(0.12)
			offThumb, onThumb = sc.OnSurface.WithAlpha(0.38), sc.Surface
			border = sc.OnSurface.WithAlpha(0.12)
		}
		c.FillRoundRect(track, 16, LerpColor(offTrack, onTrack, t))
		if t < 1 {
			c.StrokeRoundRect(track, 16, 2, border.WithAlpha(border.A*(1-t)))
		}
		// Thumb: 16px off, 24px on, 28px pressed.
		d := widgetsLerp(16, 24, t)
		if thumbIcon {
			d = 24
		}
		d = widgetsLerp(d, 28, s.grow.Value())
		cx := track.X + widgetsLerp(16, 36, t)
		cy := track.Y + 16
		if s.hovered && enabled {
			ov := pick(st.OverlayColor, LerpColor(sc.OnSurface, onTrack, t))
			c.FillCircle(geom.Pt(cx, cy), 20, ov.WithAlpha(ov.A*HoverOpacity))
		}
		thumb := LerpColor(offThumb, onThumb, t)
		if s.pressed && enabled && t > 0.5 {
			thumb = pick(st.PressedThumbColor, sc.PrimaryContainer)
		}
		c.FillCircle(geom.Pt(cx, cy), d/2, thumb)
		if thumbIcon && t > 0.5 {
			c.DrawIcon(iconCheck, geom.Rect{X: cx - 8, Y: cy - 8, W: 16, H: 16}, pick(st.ThumbIconColor, sc.OnPrimaryContainer).WithAlpha((t-0.5)*2))
		}
	}
	var child w.Widget = w.CustomPaint{Size: geom.Sz(52, 40), Painter: paint}
	if !enabled {
		return child
	}
	return w.MouseRegion{Cursor: w.CursorPointer,
		OnEnter: func(w.PointerEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(w.PointerEvent) { s.SetState(func() { s.hovered = false }) },
		Child: w.GestureDetector{
			OnTapDown:   func(w.TapDetails) { s.SetState(func() { s.pressed = true }) },
			OnTapCancel: func() { s.SetState(func() { s.pressed = false }) },
			OnTap: func() {
				s.SetState(func() { s.pressed = false })
				sw.OnChanged(!sw.Value)
			},
			OnPanStart: func(w.DragDetails) {
				s.SetState(func() { s.dragging, s.pressed, s.dragPos = true, true, s.pos.Value() })
			},
			OnPanUpdate: func(d w.DragDetails) {
				s.SetState(func() { s.dragPos = min(max(s.dragPos+d.Delta.X/20, 0), 1) })
			},
			OnPanEnd: func(w.DragDetails) {
				on := s.dragPos > 0.5
				s.SetState(func() { s.dragging, s.pressed = false, false; s.pos.Jump(s.dragPos) })
				if on != sw.Value {
					sw.OnChanged(on)
				}
			},
			Child: child,
		}}
}

func widgetsLerp(a, b, t float32) float32 { return a + (b-a)*t }

// ---------------------------------------------------------------------------
// Slider

// Slider picks a value from a range by dragging a handle.
type Slider struct {
	Value, Min, Max float32                // Max 0 means 1
	Divisions       int                    // >0 snaps to steps and shows ticks
	Label           func(v float32) string // value bubble while dragging (nil = "%.0f" with divisions)
	OnChanged       func(float32)          // nil = disabled
	OnChangeEnd     func(float32)
	// Style overrides Theme.Slider.
	Style SliderTheme
}

func (Slider) CreateState() w.State { return &sliderState{} }

type sliderState struct {
	w.StateBase
	hovered, dragging bool
	size              geom.Size
	active            int // which thumb (range slider)
	node              *w.FocusNode
}

const sliderPad = 20

func (sl Slider) rng() (float32, float32) {
	mx := sl.Max
	if mx == sl.Min {
		mx = sl.Min + 1
	}
	return sl.Min, mx
}

func valueAt(x, width, lo, hi float32, div int) float32 {
	t := (x - sliderPad) / max(width-2*sliderPad, 1)
	t = min(max(t, 0), 1)
	if div > 0 {
		t = float32(math.Round(float64(t*float32(div)))) / float32(div)
	}
	return lo + t*(hi-lo)
}

func (s *sliderState) Build(ctx w.BuildContext) w.Widget {
	sl := w.WidgetOf[Slider](s)
	th := ThemeOf(ctx)
	st := merge(th.Slider, sl.Style)
	lo, hi := sl.rng()
	t := (min(max(sl.Value, lo), hi) - lo) / (hi - lo)
	label := sl.Label
	if label == nil && sl.Divisions > 0 {
		label = func(v float32) string { return fmt.Sprintf("%.0f", v) }
	}
	paint := func(c *render.Canvas, o geom.Offset, size geom.Size) {
		s.size = size
		paintSliderTrack(c, o, size, th, st, []float32{t}, sl.Divisions, sl.OnChanged != nil, s.hovered, s.dragging)
		if s.dragging && label != nil {
			paintValueBubble(c, o, size, th, st, t, label(sl.Value))
		}
	}
	return sliderInteraction(s, sl.OnChanged != nil, paint, func(x float32, start bool) {
		v := valueAt(x, s.size.W, lo, hi, sl.Divisions)
		if v != sl.Value {
			sl.OnChanged(v)
		}
	}, func() {
		if sl.OnChangeEnd != nil {
			sl.OnChangeEnd(sl.Value)
		}
	}, func(dir float32) {
		step := (hi - lo) / 100
		if sl.Divisions > 0 {
			step = (hi - lo) / float32(sl.Divisions)
		}
		sl.OnChanged(min(max(sl.Value+dir*step, lo), hi))
	})
}

// sliderInteraction wires hover, tap, drag and arrow keys for sliders.
func sliderInteraction(s *sliderState, enabled bool, paint render.Painter, set func(x float32, start bool), end func(), key func(dir float32)) w.Widget {
	var child w.Widget = w.CustomPaint{Size: geom.Sz(geom.Inf, 44), Painter: paint}
	child = w.ConstrainedBox{Constraints: geom.Constraints{MinW: 0, MaxW: geom.Inf, MinH: 44, MaxH: 44}, Child: child}
	if !enabled {
		return child
	}
	if s.node == nil {
		s.node = &w.FocusNode{}
	}
	node := s.node
	node.OnKey = func(e w.KeyEvent) bool {
		switch e.Key {
		case w.KeyLeft, w.KeyDown:
			key(-1)
		case w.KeyRight, w.KeyUp:
			key(1)
		default:
			return false
		}
		return true
	}
	return w.Focus{Node: node, Child: w.MouseRegion{Cursor: w.CursorPointer,
		OnEnter: func(w.PointerEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(w.PointerEvent) { s.SetState(func() { s.hovered = false }) },
		Child: w.GestureDetector{
			OnTapDown: func(d w.TapDetails) { set(d.Local.X, true) },
			OnTapUp:   func(w.TapDetails) { end() },
			OnPanStart: func(d w.DragDetails) {
				s.SetState(func() { s.dragging = true })
			},
			OnPanUpdate: func(d w.DragDetails) { set(d.Local.X, false) },
			OnPanEnd: func(w.DragDetails) {
				s.SetState(func() { s.dragging = false })
				end()
			},
			Child: child,
		}}}
}

// paintSliderTrack draws track, ticks and handles at positions ts (0..1).
// With two positions the active range lies between them.
func paintSliderTrack(c *render.Canvas, o geom.Offset, size geom.Size, th Theme, st SliderTheme, ts []float32, div int, enabled, hovered, dragging bool) {
	sc := th.Scheme
	active, inactive := pick(st.ActiveTrackColor, sc.Primary), pick(st.InactiveTrackColor, sc.SecondaryContainer)
	handle, overlay := pick(st.ThumbColor, sc.Primary), pick(st.OverlayColor, sc.Primary)
	tickOn, tickOff := pick(st.ActiveTickMarkColor, sc.OnPrimary), pick(st.InactiveTickMarkColor, sc.OnSecondaryContainer)
	th4, thumbR := pickF(st.TrackHeight, 4), pickF(st.ThumbRadius, 10)
	if !enabled {
		dc, dbg := disabledColors(sc)
		active, inactive, handle = dc, dbg, dc
		tickOn, tickOff = sc.OnSurface.WithAlpha(0.38), sc.OnSurface.WithAlpha(0.38)
	}
	cy := o.Y + size.H/2
	x0, x1 := o.X+sliderPad, o.X+size.W-sliderPad
	xAt := func(t float32) float32 { return x0 + (x1-x0)*t }
	a, b := float32(0), ts[0]
	if len(ts) == 2 {
		a, b = ts[0], ts[1]
	}
	hh := th4 / 2
	c.FillRoundRect(geom.Rect{X: x0 - hh, Y: cy - hh, W: x1 - x0 + th4, H: th4}, hh, inactive)
	c.FillRoundRect(geom.Rect{X: xAt(a) - hh, Y: cy - hh, W: xAt(b) - xAt(a) + th4, H: th4}, hh, active)
	if div > 0 && div <= 100 {
		for i := 0; i <= div; i++ {
			t := float32(i) / float32(div)
			col := tickOff
			if t >= a && t <= b {
				col = tickOn
			}
			c.FillCircle(geom.Pt(xAt(t), cy), 1, col)
		}
	}
	for _, t := range ts {
		p := geom.Pt(xAt(t), cy)
		if (hovered || dragging) && enabled {
			op := HoverOpacity
			if dragging {
				op = DraggedOpacity
			}
			c.FillCircle(p, 2*thumbR, overlay.WithAlpha(overlay.A*op))
		}
		paintShadow(c, geom.Rect{X: p.X - thumbR, Y: p.Y - thumbR, W: 2 * thumbR, H: 2 * thumbR}, thumbR, 1, sc.Shadow)
		c.FillCircle(p, thumbR, handle)
	}
}

// paintValueBubble draws the value label above the handle at t.
func paintValueBubble(c *render.Canvas, o geom.Offset, size geom.Size, th Theme, st SliderTheme, t float32, s string) {
	sc := th.Scheme
	p := text.Layout(s, pickTC(st.ValueIndicatorTextStyle, th.Text.LabelMedium, sc.OnPrimary), text.Options{})
	x := o.X + sliderPad + (size.W-2*sliderPad)*t
	wd := max(p.Width+16, 28)
	r := geom.Rect{X: x - wd/2, Y: o.Y + size.H/2 - 48, W: wd, H: 28}
	c.FillRoundRect(r, 14, pick(st.ValueIndicatorColor, sc.Primary))
	c.DrawParagraph(p, geom.Pt(r.X+(r.W-p.Width)/2, r.Y+(r.H-p.Height)/2))
}

// RangeSlider selects a sub-range with two handles.
type RangeSlider struct {
	Start, End float32
	Min, Max   float32
	Divisions  int
	Label      func(v float32) string
	OnChanged  func(start, end float32)
	// Style overrides Theme.Slider.
	Style SliderTheme
}

func (RangeSlider) CreateState() w.State { return &rangeSliderState{} }

type rangeSliderState struct{ sliderState }

func (s *rangeSliderState) Build(ctx w.BuildContext) w.Widget {
	rs := w.WidgetOf[RangeSlider](s)
	th := ThemeOf(ctx)
	st := merge(th.Slider, rs.Style)
	sl := Slider{Min: rs.Min, Max: rs.Max}
	lo, hi := sl.rng()
	norm := func(v float32) float32 { return (min(max(v, lo), hi) - lo) / (hi - lo) }
	ta, tb := norm(rs.Start), norm(rs.End)
	label := rs.Label
	if label == nil && rs.Divisions > 0 {
		label = func(v float32) string { return fmt.Sprintf("%.0f", v) }
	}
	paint := func(c *render.Canvas, o geom.Offset, size geom.Size) {
		s.size = size
		paintSliderTrack(c, o, size, th, st, []float32{ta, tb}, rs.Divisions, rs.OnChanged != nil, s.hovered, s.dragging)
		if s.dragging && label != nil {
			if s.active == 0 {
				paintValueBubble(c, o, size, th, st, ta, label(rs.Start))
			} else {
				paintValueBubble(c, o, size, th, st, tb, label(rs.End))
			}
		}
	}
	return sliderInteraction(&s.sliderState, rs.OnChanged != nil, paint, func(x float32, start bool) {
		v := valueAt(x, s.size.W, lo, hi, rs.Divisions)
		if start {
			// Pick the nearest handle on press.
			if abs32(v-rs.Start) <= abs32(v-rs.End) {
				s.active = 0
			} else {
				s.active = 1
			}
		}
		a, b := rs.Start, rs.End
		if s.active == 0 {
			a = min(v, b)
		} else {
			b = max(v, a)
		}
		if a != rs.Start || b != rs.End {
			rs.OnChanged(a, b)
		}
	}, func() {}, func(dir float32) {
		step := (hi - lo) / 100
		if rs.Divisions > 0 {
			step = (hi - lo) / float32(rs.Divisions)
		}
		if s.active == 0 {
			rs.OnChanged(min(max(rs.Start+dir*step, lo), rs.End), rs.End)
		} else {
			rs.OnChanged(rs.Start, min(max(rs.End+dir*step, rs.Start), hi))
		}
	})
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
