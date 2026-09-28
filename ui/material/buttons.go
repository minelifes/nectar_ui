package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// buttonSpec describes one common-button look.
type buttonSpec struct {
	bg, fg, border geom.Color
	borderW        float32
	elev           int
	raise          bool
}

func commonButton(ctx w.BuildContext, spec buttonSpec, onPressed func(), label string, icon *vector.Icon, child w.Widget) w.Widget {
	th := ThemeOf(ctx)
	if onPressed == nil {
		dc, dbg := disabledColors(th.Scheme)
		spec.fg = dc
		if spec.bg.A > 0 {
			spec.bg = dbg
		}
		if spec.borderW > 0 {
			spec.border = dbg
		}
		spec.elev, spec.raise = 0, false
	}
	padL, padR := float32(24), float32(24)
	if icon != nil {
		padL = 16
	}
	if spec.bg.A == 0 && spec.borderW == 0 { // text button
		padL, padR = 12, 12
		if icon != nil {
			padR = 16
		}
	}
	content := child
	if content == nil {
		content = w.Text{Text: label, Style: Styled(th.Text.LabelLarge, spec.fg), MaxLines: 1}
	}
	kids := []w.Widget{}
	if icon != nil {
		kids = append(kids, w.Icon{Icon: icon, Size: 18, Color: spec.fg})
	}
	kids = append(kids, content)
	return InkSurface{
		OnTap: onPressed, Color: spec.bg, ContentColor: spec.fg,
		BorderColor: spec.border, BorderWidth: spec.borderW,
		Radius: CornerFull, Elevation: spec.elev, RaiseOnHover: spec.raise,
		Child: w.ConstrainedBox{Constraints: geom.Constraints{MinW: 48, MaxW: geom.Inf, MinH: 40, MaxH: geom.Inf},
			Child: w.Padding{Padding: geom.InsetsLTRB(padL, 0, padR, 0), Child: w.IconTheme{Size: 18, Color: spec.fg,
				Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, ShrinkMain: true, Spacing: 8, Children: kids}}}},
	}
}

// ElevatedButton: tonal surface with a shadow, for emphasis on patterned
// backgrounds.
type ElevatedButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func() // nil = disabled
	Child     w.Widget
}

func (b ElevatedButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	return commonButton(ctx, buttonSpec{bg: s.SurfaceContainerLow, fg: s.Primary, elev: 1, raise: true}, b.OnPressed, b.Label, b.Icon, b.Child)
}

// FilledButton: the highest-emphasis button.
type FilledButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
}

func (b FilledButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	return commonButton(ctx, buttonSpec{bg: s.Primary, fg: s.OnPrimary}, b.OnPressed, b.Label, b.Icon, b.Child)
}

// FilledTonalButton: secondary-container fill, between filled and outlined.
type FilledTonalButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
}

func (b FilledTonalButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	return commonButton(ctx, buttonSpec{bg: s.SecondaryContainer, fg: s.OnSecondaryContainer}, b.OnPressed, b.Label, b.Icon, b.Child)
}

// OutlinedButton: medium emphasis, outline only.
type OutlinedButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
}

func (b OutlinedButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	return commonButton(ctx, buttonSpec{fg: s.Primary, border: s.Outline, borderW: 1}, b.OnPressed, b.Label, b.Icon, b.Child)
}

// TextButton: lowest emphasis.
type TextButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
}

func (b TextButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	return commonButton(ctx, buttonSpec{fg: s.Primary}, b.OnPressed, b.Label, b.Icon, b.Child)
}

// ---------------------------------------------------------------------------
// Icon buttons

// IconButtonVariant selects the icon button style.
type IconButtonVariant uint8

const (
	IconButtonStandard IconButtonVariant = iota
	IconButtonFilled
	IconButtonTonal
	IconButtonOutlined
)

// IconButton is a 40×40 round button with a 24px icon. Set Toggle to make
// it a toggle button (Selected picks the look, SelectedIcon the icon).
type IconButton struct {
	Icon         *vector.Icon
	SelectedIcon *vector.Icon
	Variant      IconButtonVariant
	Toggle       bool
	Selected     bool
	OnPressed    func()
	Color        geom.Color // override icon color (standard variant)
	Size         float32    // icon size; 0 = 24
}

func (b IconButton) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	var bg, fg, border geom.Color
	var bw float32
	sel := b.Toggle && b.Selected
	switch b.Variant {
	case IconButtonStandard:
		fg = s.OnSurfaceVariant
		if sel {
			fg = s.Primary
		}
	case IconButtonFilled:
		bg, fg = s.Primary, s.OnPrimary
		if b.Toggle && !b.Selected {
			bg, fg = s.SurfaceContainerHighest, s.Primary
		}
	case IconButtonTonal:
		bg, fg = s.SecondaryContainer, s.OnSecondaryContainer
		if b.Toggle && !b.Selected {
			bg, fg = s.SurfaceContainerHighest, s.OnSurfaceVariant
		}
	case IconButtonOutlined:
		fg, border, bw = s.OnSurfaceVariant, s.Outline, 1
		if sel {
			bg, fg, bw = s.InverseSurface, s.InverseOnSurface, 0
		}
	}
	if b.Color.A > 0 && b.Variant == IconButtonStandard && !sel {
		fg = b.Color
	}
	if b.OnPressed == nil {
		dc, dbg := disabledColors(s)
		fg = dc
		if bg.A > 0 {
			bg = dbg
		}
		if bw > 0 {
			border = dbg
		}
	}
	ic := b.Icon
	if sel && b.SelectedIcon != nil {
		ic = b.SelectedIcon
	}
	size := b.Size
	if size == 0 {
		size = 24
	}
	return InkSurface{OnTap: b.OnPressed, Color: bg, ContentColor: fg, BorderColor: border, BorderWidth: bw, Radius: CornerFull,
		Child: w.SizedBox{Width: 40, Height: 40, Child: w.Center{Child: w.Icon{Icon: ic, Size: size, Color: fg}}}}
}

// ---------------------------------------------------------------------------
// Floating action buttons

// FABSize selects the FAB size.
type FABSize uint8

const (
	FABRegular FABSize = iota // 56px
	FABSmall                  // 40px
	FABLarge                  // 96px
)

// FloatingActionButton is the primary action of a screen. With a Label it
// becomes an extended FAB.
type FloatingActionButton struct {
	Icon      *vector.Icon
	Label     string
	Size      FABSize
	OnPressed func()
	// Lowered removes the resting shadow (e.g. inside a BottomAppBar).
	Lowered bool
	// Colors: default primary container; set Surface for a surface FAB.
	Surface bool
}

func (f FloatingActionButton) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	bg, fg := s.PrimaryContainer, s.OnPrimaryContainer
	if f.Surface {
		bg, fg = s.SurfaceContainerHigh, s.Primary
	}
	elev := 3
	if f.Lowered {
		elev = 1
	}
	side, radius, icon := float32(56), float32(16), float32(24)
	switch f.Size {
	case FABSmall:
		side, radius = 40, 12
	case FABLarge:
		side, radius, icon = 96, 28, 36
	}
	var child w.Widget
	if f.Label != "" {
		kids := []w.Widget{}
		if f.Icon != nil {
			kids = append(kids, w.Icon{Icon: f.Icon, Size: 24, Color: fg})
		}
		kids = append(kids, w.Text{Text: f.Label, Style: Styled(th.Text.LabelLarge, fg)})
		child = w.ConstrainedBox{Constraints: geom.Constraints{MinW: 80, MaxW: geom.Inf, MinH: 56, MaxH: 56},
			Child: w.Padding{Padding: geom.InsetsHV(16, 0), Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, ShrinkMain: true, Spacing: 12, Children: kids}}}
	} else {
		child = w.SizedBox{Width: side, Height: side, Child: w.Center{Child: w.Icon{Icon: f.Icon, Size: icon, Color: fg}}}
	}
	return InkSurface{OnTap: f.OnPressed, Color: bg, ContentColor: fg, Radius: radius, Elevation: elev, RaiseOnHover: true, Child: child}
}

// ---------------------------------------------------------------------------
// Segmented button

// Segment is one option of a SegmentedButton.
type Segment struct {
	Value any // comparable
	Label string
	Icon  *vector.Icon
}

// SegmentedButton selects one (or, with Multi, several) options.
type SegmentedButton struct {
	Segments  []Segment
	Selected  []any // selected values
	Multi     bool
	OnChanged func(selected []any) // nil = disabled
}

func (b SegmentedButton) isSel(v any) bool {
	for _, x := range b.Selected {
		if x == v {
			return true
		}
	}
	return false
}

func (b SegmentedButton) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	n := len(b.Segments)
	border := s.Outline
	if b.OnChanged == nil {
		_, border = disabledColors(s)
	}
	kids := make([]w.Widget, 0, n*2)
	for i, seg := range b.Segments {
		sel := b.isSel(seg.Value)
		fg := s.OnSurface
		if sel {
			fg = s.OnSecondaryContainer
		}
		if b.OnChanged == nil {
			fg, _ = disabledColors(s)
		}
		inner := []w.Widget{}
		if sel {
			inner = append(inner, w.Icon{Icon: iconCheck, Size: 18, Color: fg})
		} else if seg.Icon != nil {
			inner = append(inner, w.Icon{Icon: seg.Icon, Size: 18, Color: fg})
		}
		if seg.Label != "" {
			inner = append(inner, w.Text{Text: seg.Label, Style: Styled(th.Text.LabelLarge, fg), MaxLines: 1, Ellipsis: true})
		}
		var onTap func()
		if b.OnChanged != nil {
			seg := seg
			onTap = func() {
				if b.Multi {
					var out []any
					if b.isSel(seg.Value) {
						for _, x := range b.Selected {
							if x != seg.Value {
								out = append(out, x)
							}
						}
					} else {
						out = append(append(out, b.Selected...), seg.Value)
					}
					b.OnChanged(out)
				} else {
					b.OnChanged([]any{seg.Value})
				}
			}
		}
		first, last := i == 0, i == n-1
		selBG := s.SecondaryContainer
		cell := w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				if !sel {
					return
				}
				r := geom.RectFrom(o, size)
				// Round only the outer corners of the first/last segment.
				ext := r
				if !first {
					ext.X -= size.H
					ext.W += size.H
				}
				if !last {
					ext.W += size.H
				}
				c.PushClip(r)
				c.FillRoundRect(ext, size.H/2, selBG)
				c.PopClip()
			},
			Child: InkSurface{OnTap: onTap, ContentColor: fg, NoFocus: false,
				Child: w.SizedBox{Height: 40, Child: w.Padding{Padding: geom.InsetsHV(12, 0),
					Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, Spacing: 8, Children: inner}}}},
		}
		kids = append(kids, w.Expanded{Child: cell})
		if !last {
			kids = append(kids, w.SizedBox{Width: 1, Height: 40, Child: w.DecoratedBox{Color: border}})
		}
	}
	return w.CustomPaint{
		ForegroundPainter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			c.StrokeRoundRect(geom.RectFrom(o, size), size.H/2, 1, border)
		},
		Child: w.Row{ShrinkMain: false, Children: kids},
	}
}

// helpers shared by components
func textOf(ctx w.BuildContext, st text.Style, c geom.Color, s string) w.Widget {
	return w.Text{Text: s, Style: Styled(st, c)}
}
