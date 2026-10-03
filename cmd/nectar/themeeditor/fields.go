package themeeditor

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// fieldEditor edits one field of the design: its label, an editor for
// its kind showing the effective value (the edit, or the default), and a
// reset button while it's edited. Its own fields never change, so it
// watches the view model itself (a parent rebuild skips it).
type fieldEditor struct {
	vm    *VM
	field Field
}

func (f fieldEditor) Key() any { return f.field.Path }

func (f fieldEditor) Build(ctx w.BuildContext) w.Widget {
	vm, fl := f.vm, f.field
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Dark)
	th := m.ThemeOf(ctx)
	edit, edited := vm.Edit(fl.Path)
	cur, hasCur := Value(vm.Theme(vm.Dark.Get()), fl.Path)
	if edited {
		cur, hasCur = edit, true
	}
	var editor w.Widget
	switch fl.Kind {
	case KindColor:
		c, _ := cur.(geom.Color)
		editor = colorEditor{vm: vm, path: fl.Path, color: c, set: hasCur}
	case KindFloat:
		v, _ := cur.(float32)
		editor = numberEditor(fl, v, hasCur, func(x float32) { vm.Set(fl.Path, x) })
	case KindInt:
		v, _ := cur.(int)
		editor = choice(th, fl.Path, []string{"0", "1", "2", "3", "4", "5"}, fmt.Sprint(v), hasCur, func(i int) { vm.Set(fl.Path, i) })
	case KindBool:
		v, _ := cur.(bool)
		sel := "Off"
		if v {
			sel = "On"
		}
		editor = choice(th, fl.Path, []string{"On", "Off"}, sel, hasCur, func(i int) { vm.Set(fl.Path, i == 0) })
	case KindFont:
		v, _ := cur.(string)
		editor = choice(th, fl.Path, []string{FontRegular, FontMedium, FontBold}, v, hasCur, func(i int) {
			vm.Set(fl.Path, []string{FontRegular, FontMedium, FontBold}[i])
		})
	case KindInsets:
		v, _ := cur.(geom.EdgeInsets)
		editor = insetsEditor(v, func(e geom.EdgeInsets) { vm.Set(fl.Path, e) })
	case KindDuration:
		v, _ := cur.(time.Duration)
		editor = sliderRow(float32(v.Milliseconds()), 0, 2000, 50, func(x float32) string { return fmt.Sprintf("%.0f ms", x) },
			func(x float32) { vm.Set(fl.Path, time.Duration(x)*time.Millisecond) })
	}

	labelColor := th.Scheme.OnSurfaceVariant
	if edited {
		labelColor = th.Scheme.Primary
	}
	head := []w.Widget{w.Expanded{Child: w.Text{Text: fl.Name, Style: m.Styled(th.Text.LabelLarge, labelColor), MaxLines: 1, Ellipsis: true}}}
	if !edited && !hasCur {
		head = append(head, w.Text{Text: "default", Style: m.Styled(th.Text.LabelSmall, th.Scheme.Outline)})
	}
	if edited {
		head = append(head, m.Tooltip{Message: "Reset to default", Child: m.IconButton{Icon: icons.Undo, Size: 18, OnPressed: func() { vm.Unset(fl.Path) }}})
	} else {
		head = append(head, w.SizedBox{Width: 40, Height: 40})
	}
	return w.Padding{Padding: geom.InsetsLTRB(16, 4, 8, 8), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 2, Children: []w.Widget{
		w.Row{Cross: w.CrossCenter, Children: head},
		editor,
	}}}
}

// choice is a segmented button over options; selected is shown only when
// the field has a value.
func choice(th m.Theme, key string, options []string, selected string, has bool, set func(i int)) w.Widget {
	segs := make([]m.Segment, len(options))
	for i, o := range options {
		segs[i] = m.Segment{Value: i, Label: o}
	}
	var sel []any
	for i, o := range options {
		if has && o == selected {
			sel = []any{i}
		}
	}
	return m.SegmentedButton{Segments: segs, Selected: sel, OnChanged: func(v []any) {
		if len(v) > 0 {
			set(v[0].(int))
		}
	}, Style: m.ButtonStyle{MinHeight: m.Dp(36), Padding: w.Ptr(geom.InsetsHV(4, 0))}}
}

// sliderRow is a slider with its value printed next to it.
func sliderRow(v, lo, hi, step float32, format func(float32) string, set func(float32)) w.Widget {
	div := 0 // ticks only for coarse sliders; values snap to step either way
	if step > 0 {
		if n := int(math.Round(float64((hi - lo) / step))); n <= 12 {
			div = n
		}
	}
	shown := min(max(v, lo), hi)
	return w.Row{Cross: w.CrossCenter, Children: []w.Widget{
		w.Expanded{Child: m.Slider{Value: shown, Min: lo, Max: hi, Divisions: div, Label: format, OnChanged: func(x float32) {
			if step > 0 {
				x = lo + float32(math.Round(float64((x-lo)/step)))*step
			}
			set(x)
		}}},
		w.SizedBox{Width: 64, Child: w.Text{Text: format(v), Align: text.AlignEnd, MaxLines: 1}},
	}}
}

// numberRange picks a slider range and step for a float field.
func numberRange(fl Field) (lo, hi, step float32) {
	leaf := fl.Path[strings.LastIndex(fl.Path, ".")+1:]
	if inTextStyle(fl.Path) {
		switch leaf {
		case "LetterSpacing":
			return -2, 4, 0.05
		case "LineHeight":
			return 0.8, 2.5, 0.05
		}
		return 6, 72, 1 // Size
	}
	switch {
	case strings.Contains(leaf, "Radius"):
		return 0, 48, 1
	case leaf == "SideWidth", leaf == "BorderWidth", leaf == "Thickness", leaf == "StrokeWidth",
		leaf == "TrackHeight", leaf == "IndicatorHeight", leaf == "LinearHeight":
		return 0, 12, 0.5
	case leaf == "MaxWidth":
		return 0, 1200, 10
	case strings.HasSuffix(leaf, "Width"):
		return 0, 600, 1
	case leaf == "CircularSize":
		return 0, 160, 1
	case strings.HasSuffix(leaf, "Size"):
		return 0, 64, 1
	case strings.HasSuffix(leaf, "Height"):
		return 0, 200, 1
	}
	return 0, 64, 1
}

// inTextStyle reports whether path is a field of a text.Style.
func inTextStyle(path string) bool {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return false
	}
	v, err := lookup(reflect.New(themeT).Elem(), path[:i])
	return err == nil && v.Type() == styleT
}

// numberEditor edits a float with a slider; radii past the slider (the
// "full" pill radius) read as "full".
func numberEditor(fl Field, v float32, has bool, set func(float32)) w.Widget {
	lo, hi, step := numberRange(fl)
	format := func(x float32) string {
		if !has && x == v {
			return "–"
		}
		if x >= m.CornerFull {
			return "full"
		}
		if step < 1 {
			return fmt.Sprintf("%.2g", x)
		}
		return fmt.Sprintf("%.0f", x)
	}
	row := sliderRow(v, lo, hi, step, format, set)
	if !strings.Contains(fl.Path[strings.LastIndex(fl.Path, ".")+1:], "Radius") {
		return row
	}
	return w.Row{Cross: w.CrossCenter, Children: []w.Widget{
		w.Expanded{Child: row},
		m.TextButton{Label: "Pill", OnPressed: func() { set(m.CornerFull) }},
	}}
}

// insetsEditor edits the four sides of an EdgeInsets.
func insetsEditor(e geom.EdgeInsets, set func(geom.EdgeInsets)) w.Widget {
	side := func(label string, v float32, with func(float32) geom.EdgeInsets) w.Widget {
		return w.Row{Cross: w.CrossCenter, Children: []w.Widget{
			w.SizedBox{Width: 52, Child: w.Text{Text: label}},
			w.Expanded{Child: sliderRow(v, 0, 64, 1, func(x float32) string { return fmt.Sprintf("%.0f", x) }, func(x float32) { set(with(x)) })},
		}}
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		side("Left", e.Left, func(x float32) geom.EdgeInsets { e.Left = x; return e }),
		side("Top", e.Top, func(x float32) geom.EdgeInsets { e.Top = x; return e }),
		side("Right", e.Right, func(x float32) geom.EdgeInsets { e.Right = x; return e }),
		side("Bottom", e.Bottom, func(x float32) geom.EdgeInsets { e.Bottom = x; return e }),
	}}
}

// ---------------------------------------------------------------------------
// Colors

// colorEditor shows a swatch (tap: color picker) and a hex field.
type colorEditor struct {
	vm    *VM
	path  string
	color geom.Color
	set   bool
}

func (c colorEditor) Build(ctx w.BuildContext) w.Widget {
	return w.Row{Cross: w.CrossCenter, Spacing: 12, Children: []w.Widget{
		swatch{color: c.color, empty: !c.set, onTap: func() {
			openColorPicker(ctx, c.vm, c.color, func(nc geom.Color) { c.vm.Set(c.path, nc) })
		}},
		w.Expanded{Child: hexField{color: c.color, set: c.set, onColor: func(nc geom.Color) { c.vm.Set(c.path, nc) }}},
	}}
}

// swatch is a tappable color sample over a checkerboard (so transparency
// shows).
type swatch struct {
	color geom.Color
	empty bool
	size  float32
	onTap func()
}

func (s swatch) Build(ctx w.BuildContext) w.Widget {
	sc := m.ThemeOf(ctx).Scheme
	size := s.size
	if size == 0 {
		size = 40
	}
	var inner w.Widget = w.Container{Width: size, Height: size, Color: s.color, Border: &geom.Border{Radius: 8, Color: sc.OutlineVariant, Width: 1}}
	if s.empty {
		inner = w.Container{Width: size, Height: size, Border: &geom.Border{Radius: 8, Color: sc.OutlineVariant, Width: 1},
			Alignment: w.Ptr(geom.Center), Child: w.Icon{Icon: icons.FormatColorFill, Size: 18, Color: sc.Outline}}
	}
	return m.InkSurface{OnTap: s.onTap, Radius: 8, ContentColor: sc.OnSurface, Child: w.Stack{Children: []w.Widget{checker{size: size}, inner}}}
}

// checker paints a small checkerboard behind translucent swatches.
type checker struct{ size float32 }

func (c checker) Build(w.BuildContext) w.Widget {
	const cell = 8
	var kids []w.Widget
	for y := float32(0); y < c.size; y += cell {
		for x := float32(0); x < c.size; x += cell {
			if int(x/cell+y/cell)%2 == 0 {
				kids = append(kids, w.Positioned{Left: w.At(x), Top: w.At(y), Child: w.Container{Width: cell, Height: cell, Color: geom.Hex(0xDDDDDD)}})
			}
		}
	}
	return w.ClipRect{Child: w.SizedBox{Width: c.size, Height: c.size, Child: w.Stack{Children: kids}}}
}

// hexField is a text field for "#RRGGBB[AA]" that applies valid input as
// it's typed and follows outside changes while not focused.
type hexField struct {
	color   geom.Color
	set     bool
	onColor func(geom.Color)
}

func (hexField) CreateState() w.State { return &hexFieldState{} }

type hexFieldState struct {
	w.StateBase
	ctrl *w.TextEditingController
	err  string
	last [2]any // the (color, set) the text was last synced to
}

func hexText(c geom.Color, set bool) string {
	if !set {
		return ""
	}
	return strings.TrimSuffix(ColorHex(c), "FF")
}

func (s *hexFieldState) InitState() {
	f := w.WidgetOf[hexField](s)
	s.ctrl = w.NewTextController(hexText(f.color, f.set))
	s.last = [2]any{f.color, f.set}
}

// DidUpdateWidget shows a value changed from outside (picker, reset);
// the field's own edits come back equal and leave the text alone.
func (s *hexFieldState) DidUpdateWidget(w.Widget) {
	f := w.WidgetOf[hexField](s)
	if now := [2]any{f.color, f.set}; now != s.last {
		s.last = now
		if c, err := ParseColor(s.ctrl.Text()); err != nil || Quantize(c) != Quantize(f.color) || !f.set {
			s.ctrl.SetText(hexText(f.color, f.set))
			s.err = ""
		}
	}
}

func (s *hexFieldState) Build(w.BuildContext) w.Widget {
	f := w.WidgetOf[hexField](s)
	hint := ""
	if !f.set {
		hint = "#RRGGBB or #RRGGBBAA"
	}
	return m.TextField{Controller: s.ctrl, Hint: hint, Error: s.err, Outlined: true,
		OnChanged: func(v string) {
			if strings.TrimSpace(v) == "" {
				s.SetState(func() { s.err = "" })
				return
			}
			c, err := ParseColor(v)
			if err != nil {
				s.SetState(func() { s.err = "Use #RRGGBB or #RRGGBBAA" })
				return
			}
			s.SetState(func() { s.err = "" })
			f.onColor(c)
		}}
}

// ---------------------------------------------------------------------------
// Color picker

// openColorPicker shows a dialog with hue / saturation / value / alpha
// sliders, a hex field and the scheme's colors to pick from.
func openColorPicker(ctx w.BuildContext, vm *VM, initial geom.Color, apply func(geom.Color)) {
	m.ShowDialog(ctx, func(ctx w.BuildContext) w.Widget {
		return colorPicker{vm: vm, initial: initial, apply: apply}
	}, nil)
}

type colorPicker struct {
	vm      *VM
	initial geom.Color
	apply   func(geom.Color)
}

func (colorPicker) CreateState() w.State { return &colorPickerState{} }

type colorPickerState struct {
	w.StateBase
	h, s, v, a float32
}

func (st *colorPickerState) InitState() {
	c := w.WidgetOf[colorPicker](st).initial
	if c == (geom.Color{}) {
		c = geom.Hex(0x6750A4)
	}
	st.h, st.s, st.v = toHSV(c)
	st.a = c.A
}

func (st *colorPickerState) color() geom.Color { return Quantize(fromHSV(st.h, st.s, st.v, st.a)) }

func (st *colorPickerState) setColor(c geom.Color) {
	st.SetState(func() {
		st.h, st.s, st.v = toHSV(c)
		st.a = c.A
	})
}

func (st *colorPickerState) Build(ctx w.BuildContext) w.Widget {
	p := w.WidgetOf[colorPicker](st)
	th := m.ThemeOf(ctx)
	cur := st.color()
	slider := func(label string, v, hi float32, format func(float32) string, set func(float32)) w.Widget {
		return w.Row{Cross: w.CrossCenter, Children: []w.Widget{
			w.SizedBox{Width: 88, Child: w.Text{Text: label}},
			w.Expanded{Child: sliderRow(v, 0, hi, 0, format, func(x float32) { st.SetState(func() { set(x) }) })},
		}}
	}
	pct := func(x float32) string { return fmt.Sprintf("%.0f%%", x*100) }
	// Scheme colors of the designed theme, to pick from.
	sc := p.vm.Theme(p.vm.Dark.Get()).Scheme
	roles := []geom.Color{sc.Primary, sc.OnPrimary, sc.PrimaryContainer, sc.OnPrimaryContainer, sc.Secondary, sc.SecondaryContainer,
		sc.Tertiary, sc.TertiaryContainer, sc.Error, sc.ErrorContainer, sc.Surface, sc.SurfaceContainer, sc.SurfaceContainerHighest,
		sc.OnSurface, sc.OnSurfaceVariant, sc.Outline, sc.OutlineVariant, sc.InverseSurface, geom.White, geom.Black}
	var chips []w.Widget
	for _, c := range roles {
		c := c
		chips = append(chips, swatch{color: c, size: 28, onTap: func() { st.setColor(c) }})
	}
	return m.Dialog{Child: w.SizedBox{Width: 420, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 8, Children: []w.Widget{
		w.Text{Text: "Pick a color", Style: m.Styled(th.Text.HeadlineSmall, th.Scheme.OnSurface)},
		w.Row{Spacing: 16, Cross: w.CrossCenter, Children: []w.Widget{
			swatch{color: cur, size: 56},
			w.Expanded{Child: hexField{color: cur, set: true, onColor: st.setColor}},
		}},
		slider("Hue", st.h, 360, func(x float32) string { return fmt.Sprintf("%.0f°", x) }, func(x float32) { st.h = x }),
		slider("Saturation", st.s, 1, pct, func(x float32) { st.s = x }),
		slider("Brightness", st.v, 1, pct, func(x float32) { st.v = x }),
		slider("Opacity", st.a, 1, pct, func(x float32) { st.a = x }),
		w.Text{Text: "From the scheme", Style: m.Styled(th.Text.LabelLarge, th.Scheme.OnSurfaceVariant)},
		w.Wrap{Spacing: 6, RunSpacing: 6, Children: chips},
		w.SizedBox{Height: 8},
		w.Row{Main: w.MainEnd, Spacing: 8, Children: []w.Widget{
			m.TextButton{Label: "Transparent", OnPressed: func() { p.apply(m.Transparent); m.Pop(ctx, nil) }},
			w.Spacer{},
			m.TextButton{Label: "Cancel", OnPressed: func() { m.Pop(ctx, nil) }},
			m.FilledButton{Label: "Apply", OnPressed: func() { p.apply(cur); m.Pop(ctx, nil) }},
		}},
	}}}}
}

// toHSV converts to hue (0–360), saturation and value (0–1).
func toHSV(c geom.Color) (h, s, v float32) {
	mx := max(c.R, c.G, c.B)
	mn := min(c.R, c.G, c.B)
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case c.R:
		h = (c.G - c.B) / d
		if h < 0 {
			h += 6
		}
	case c.G:
		h = (c.B-c.R)/d + 2
	default:
		h = (c.R-c.G)/d + 4
	}
	return h * 60, s, v
}

// fromHSV is the inverse of toHSV.
func fromHSV(h, s, v, a float32) geom.Color {
	h = float32(math.Mod(float64(h), 360)) / 60
	i := int(h)
	f := h - float32(i)
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float32
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return geom.Color{R: r, G: g, B: b, A: a}
}
