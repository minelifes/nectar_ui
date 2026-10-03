package material

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// buttonDefaults is the M3 look of each common-button variant.
func buttonDefaults(s ColorScheme, variant int, hasIcon bool) ButtonStyle {
	padL, padR := float32(24), float32(24)
	if hasIcon {
		padL = 16
	}
	st := ButtonStyle{Radius: Dp(CornerFull), Elevation: w.Ptr(0), MinWidth: Dp(48), MinHeight: Dp(40), IconSize: Dp(18)}
	switch variant {
	case btnElevated:
		st.BackgroundColor, st.ForegroundColor, st.Elevation = s.SurfaceContainerLow, s.Primary, w.Ptr(1)
	case btnFilled:
		st.BackgroundColor, st.ForegroundColor = s.Primary, s.OnPrimary
	case btnTonal:
		st.BackgroundColor, st.ForegroundColor = s.SecondaryContainer, s.OnSecondaryContainer
	case btnOutlined:
		st.ForegroundColor, st.SideColor, st.SideWidth = s.Primary, s.Outline, Dp(1)
	case btnText:
		st.ForegroundColor = s.Primary
		padL, padR = 12, 12
		if hasIcon {
			padR = 16
		}
	}
	st.Padding = w.Ptr(geom.InsetsLTRB(padL, 0, padR, 0))
	return st
}

const (
	btnElevated = iota
	btnFilled
	btnTonal
	btnOutlined
	btnText
)

// resolveButton merges the variant defaults, the theme's style and the
// widget's style, in that order.
func resolveButton(ctx w.BuildContext, variant int, themed func(Theme) ButtonStyle, own ButtonStyle, hasIcon bool) (Theme, ButtonStyle) {
	th := ThemeOf(ctx)
	return th, merge(merge(buttonDefaults(th.Scheme, variant, hasIcon), themed(th)), own)
}

func commonButton(th Theme, st ButtonStyle, onPressed func(), label string, icon *vector.Icon, child w.Widget) w.Widget {
	bg, fg := st.BackgroundColor, st.ForegroundColor
	sideC, sideW := side(st.SideColor, st.SideWidth, th.Scheme.Outline, 0)
	elev := pickI(st.Elevation, 0)
	raise := elev > 0
	if onPressed == nil {
		dc, dbg := disabledColors(th.Scheme)
		fg = pick(st.DisabledForegroundColor, dc)
		if bg.A > 0 {
			bg = pick(st.DisabledBackgroundColor, dbg)
		}
		if sideW > 0 {
			sideC = dbg
		}
		elev, raise = 0, false
	}
	iconC := pick(st.IconColor, fg)
	if onPressed == nil {
		iconC = fg
	}
	iconSize := pickF(st.IconSize, 18)
	content := child
	if content == nil {
		content = w.Text{Text: label, Style: pickTC(st.TextStyle, th.Text.LabelLarge, fg), MaxLines: 1}
	}
	kids := []w.Widget{}
	if icon != nil {
		kids = append(kids, w.Icon{Icon: icon, Size: iconSize, Color: iconC})
	}
	kids = append(kids, content)
	return w.Semantics{SemanticsData: w.SemanticsData{Role: "button", Label: label, Disabled: onPressed == nil, OnTap: onPressed}, Child: InkSurface{
		OnTap: onPressed, Color: bg, ContentColor: pick(st.OverlayColor, fg), ShadowColor: st.ShadowColor,
		BorderColor: sideC, BorderWidth: sideW,
		Radius: pickF(st.Radius, CornerFull), Elevation: elev, RaiseOnHover: raise,
		Child: w.ConstrainedBox{Constraints: geom.Constraints{MinW: pickF(st.MinWidth, 48), MaxW: geom.Inf, MinH: pickF(st.MinHeight, 40), MaxH: geom.Inf},
			Child: w.Padding{Padding: pickE(st.Padding, geom.InsetsHV(24, 0)), Child: w.IconTheme{Size: iconSize, Color: iconC,
				Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, ShrinkMain: true, Spacing: 8, Children: kids}}}},
	}}
}

// ElevatedButton: tonal surface with a shadow, for emphasis on patterned
// backgrounds. Styled by Theme.ElevatedButton and Style.
type ElevatedButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func() // nil = disabled
	Child     w.Widget
	Style     ButtonStyle
}

func (b ElevatedButton) Build(ctx w.BuildContext) w.Widget {
	th, st := resolveButton(ctx, btnElevated, func(t Theme) ButtonStyle { return t.ElevatedButton }, b.Style, b.Icon != nil)
	return commonButton(th, st, b.OnPressed, b.Label, b.Icon, b.Child)
}

// FilledButton: the highest-emphasis button. Styled by Theme.FilledButton
// and Style.
type FilledButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
	Style     ButtonStyle
}

func (b FilledButton) Build(ctx w.BuildContext) w.Widget {
	th, st := resolveButton(ctx, btnFilled, func(t Theme) ButtonStyle { return t.FilledButton }, b.Style, b.Icon != nil)
	return commonButton(th, st, b.OnPressed, b.Label, b.Icon, b.Child)
}

// FilledTonalButton: secondary-container fill, between filled and outlined.
// Styled by Theme.FilledTonalButton and Style.
type FilledTonalButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
	Style     ButtonStyle
}

func (b FilledTonalButton) Build(ctx w.BuildContext) w.Widget {
	th, st := resolveButton(ctx, btnTonal, func(t Theme) ButtonStyle { return t.FilledTonalButton }, b.Style, b.Icon != nil)
	return commonButton(th, st, b.OnPressed, b.Label, b.Icon, b.Child)
}

// OutlinedButton: medium emphasis, outline only. Styled by
// Theme.OutlinedButton and Style.
type OutlinedButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
	Style     ButtonStyle
}

func (b OutlinedButton) Build(ctx w.BuildContext) w.Widget {
	th, st := resolveButton(ctx, btnOutlined, func(t Theme) ButtonStyle { return t.OutlinedButton }, b.Style, b.Icon != nil)
	return commonButton(th, st, b.OnPressed, b.Label, b.Icon, b.Child)
}

// TextButton: lowest emphasis. Styled by Theme.TextButton and Style.
type TextButton struct {
	Label     string
	Icon      *vector.Icon
	OnPressed func()
	Child     w.Widget
	Style     ButtonStyle
}

func (b TextButton) Build(ctx w.BuildContext) w.Widget {
	th, st := resolveButton(ctx, btnText, func(t Theme) ButtonStyle { return t.TextButton }, b.Style, b.Icon != nil)
	return commonButton(th, st, b.OnPressed, b.Label, b.Icon, b.Child)
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
// Styled by Theme.IconButton and Style: BackgroundColor/ForegroundColor
// apply to the unselected look, Selected* to the selected one.
type IconButton struct {
	Icon         *vector.Icon
	SelectedIcon *vector.Icon
	Variant      IconButtonVariant
	Toggle       bool
	Selected     bool
	OnPressed    func()
	Color        geom.Color // icon color (unselected); wins over Style
	Size         float32    // icon size; 0 = Style / 24
	Style        ButtonStyle
	// SemanticLabel names the button for assistive technology and tests
	// (it shows only an icon).
	SemanticLabel string
}

func iconButtonDefaults(s ColorScheme, v IconButtonVariant, toggle bool) ButtonStyle {
	st := ButtonStyle{Radius: Dp(CornerFull), IconSize: Dp(24), MinWidth: Dp(40), MinHeight: Dp(40)}
	switch v {
	case IconButtonStandard:
		st.ForegroundColor, st.SelectedForegroundColor = s.OnSurfaceVariant, s.Primary
		st.SelectedBackgroundColor = Transparent
	case IconButtonFilled:
		st.BackgroundColor, st.ForegroundColor = s.Primary, s.OnPrimary
		st.SelectedBackgroundColor, st.SelectedForegroundColor = s.Primary, s.OnPrimary
		if toggle {
			st.BackgroundColor, st.ForegroundColor = s.SurfaceContainerHighest, s.Primary
		}
	case IconButtonTonal:
		st.BackgroundColor, st.ForegroundColor = s.SecondaryContainer, s.OnSecondaryContainer
		st.SelectedBackgroundColor, st.SelectedForegroundColor = s.SecondaryContainer, s.OnSecondaryContainer
		if toggle {
			st.BackgroundColor, st.ForegroundColor = s.SurfaceContainerHighest, s.OnSurfaceVariant
		}
	case IconButtonOutlined:
		st.ForegroundColor, st.SideColor, st.SideWidth = s.OnSurfaceVariant, s.Outline, Dp(1)
		st.SelectedBackgroundColor, st.SelectedForegroundColor = s.InverseSurface, s.InverseOnSurface
	}
	return st
}

func (b IconButton) Build(ctx w.BuildContext) w.Widget {
	return w.Semantics{SemanticsData: w.SemanticsData{Role: "button", Label: b.SemanticLabel, Checkable: b.Toggle, Checked: b.Toggle && b.Selected,
		Disabled: b.OnPressed == nil, OnTap: b.OnPressed}, Child: b.build(ctx)}
}

func (b IconButton) build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	st := merge(merge(iconButtonDefaults(s, b.Variant, b.Toggle), th.IconButton), b.Style)
	sel := b.Toggle && b.Selected
	bg, fg := st.BackgroundColor, st.ForegroundColor
	border, bw := side(st.SideColor, st.SideWidth, s.Outline, 0)
	if sel {
		bg, fg = st.SelectedBackgroundColor, st.SelectedForegroundColor
		if b.Variant == IconButtonOutlined {
			bw = 0
		}
	} else if b.Color.A > 0 {
		fg = b.Color
	}
	if b.OnPressed == nil {
		dc, dbg := disabledColors(s)
		fg = pick(st.DisabledForegroundColor, dc)
		if bg.A > 0 {
			bg = pick(st.DisabledBackgroundColor, dbg)
		}
		if bw > 0 {
			border = dbg
		}
	}
	iconC := fg
	if st.IconColor != (geom.Color{}) && !sel && b.OnPressed != nil && b.Color.A == 0 {
		iconC = st.IconColor
	}
	ic := b.Icon
	if sel && b.SelectedIcon != nil {
		ic = b.SelectedIcon
	}
	size := b.Size
	if size == 0 {
		size = pickF(st.IconSize, 24)
	}
	return InkSurface{OnTap: b.OnPressed, Color: bg, ContentColor: pick(st.OverlayColor, fg), BorderColor: border, BorderWidth: bw,
		Radius: pickF(st.Radius, CornerFull), Elevation: pickI(st.Elevation, 0), ShadowColor: st.ShadowColor,
		Child: w.SizedBox{Width: pickF(st.MinWidth, 40), Height: pickF(st.MinHeight, 40), Child: w.Center{Child: w.Icon{Icon: ic, Size: size, Color: iconC}}}}
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
	// Style overrides Theme.FAB.
	Style FloatingActionButtonTheme
}

func (f FloatingActionButton) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	bg, fg := s.PrimaryContainer, s.OnPrimaryContainer
	if f.Surface {
		bg, fg = s.SurfaceContainerHigh, s.Primary
	}
	st := merge(th.FAB, f.Style)
	bg, fg = pick(st.BackgroundColor, bg), pick(st.ForegroundColor, fg)
	elev := 3
	if f.Lowered {
		elev = 1
	}
	elev = pickI(st.Elevation, elev)
	side, radius, icon := float32(56), float32(16), float32(24)
	switch f.Size {
	case FABSmall:
		side, radius = 40, 12
	case FABLarge:
		side, radius, icon = 96, 28, 36
	}
	radius, icon = pickF(st.Radius, radius), pickF(st.IconSize, icon)
	var child w.Widget
	if f.Label != "" {
		kids := []w.Widget{}
		if f.Icon != nil {
			kids = append(kids, w.Icon{Icon: f.Icon, Size: pickF(st.IconSize, 24), Color: fg})
		}
		kids = append(kids, w.Text{Text: f.Label, Style: pickTC(st.ExtendedTextStyle, th.Text.LabelLarge, fg)})
		child = w.ConstrainedBox{Constraints: geom.Constraints{MinW: 80, MaxW: geom.Inf, MinH: 56, MaxH: 56},
			Child: w.Padding{Padding: pickE(st.ExtendedPadding, geom.InsetsHV(16, 0)), Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, ShrinkMain: true, Spacing: 12, Children: kids}}}
	} else {
		child = w.SizedBox{Width: side, Height: side, Child: w.Center{Child: w.Icon{Icon: f.Icon, Size: icon, Color: fg}}}
	}
	return InkSurface{OnTap: f.OnPressed, Color: bg, ContentColor: pick(st.OverlayColor, fg), Radius: radius, Elevation: elev, RaiseOnHover: true, Child: child}
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
	// Style overrides Theme.SegmentedButton: ForegroundColor (unselected
	// segments), Selected* (selected ones), SideColor/SideWidth (outline),
	// MinHeight, IconSize, Padding, TextStyle, OverlayColor.
	Style ButtonStyle
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
	st := merge(merge(ButtonStyle{
		ForegroundColor: s.OnSurface, SelectedBackgroundColor: s.SecondaryContainer, SelectedForegroundColor: s.OnSecondaryContainer,
		SideColor: s.Outline, SideWidth: Dp(1), MinHeight: Dp(40), IconSize: Dp(18), Padding: w.Ptr(geom.InsetsHV(12, 0)),
	}, th.SegmentedButton), b.Style)
	n := len(b.Segments)
	border, bw := side(st.SideColor, st.SideWidth, s.Outline, 1)
	height, iconSize := pickF(st.MinHeight, 40), pickF(st.IconSize, 18)
	unselBG := st.BackgroundColor
	radius := pickF(st.Radius, CornerFull)
	if b.OnChanged == nil {
		_, border = disabledColors(s)
	}
	kids := make([]w.Widget, 0, n*2)
	for i, seg := range b.Segments {
		sel := b.isSel(seg.Value)
		fg := st.ForegroundColor
		if sel {
			fg = st.SelectedForegroundColor
		}
		if b.OnChanged == nil {
			dc, _ := disabledColors(s)
			fg = pick(st.DisabledForegroundColor, dc)
		}
		iconC := fg
		if !sel && b.OnChanged != nil {
			iconC = pick(st.IconColor, fg)
		}
		inner := []w.Widget{}
		if sel {
			inner = append(inner, w.Icon{Icon: iconCheck, Size: iconSize, Color: fg})
		} else if seg.Icon != nil {
			inner = append(inner, w.Icon{Icon: seg.Icon, Size: iconSize, Color: iconC})
		}
		if seg.Label != "" {
			inner = append(inner, w.Text{Text: seg.Label, Style: pickTC(st.TextStyle, th.Text.LabelLarge, fg), MaxLines: 1, Ellipsis: true})
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
		fillBG := unselBG
		if sel {
			fillBG = st.SelectedBackgroundColor
		}
		cell := w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				if fillBG.A == 0 {
					return
				}
				r := geom.RectFrom(o, size)
				rad := min(radius, size.H/2)
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
				c.FillRoundRect(ext, rad, fillBG)
				c.PopClip()
			},
			Child: InkSurface{OnTap: onTap, ContentColor: pick(st.OverlayColor, fg), NoFocus: false,
				Child: w.SizedBox{Height: height, Child: w.Padding{Padding: pickE(st.Padding, geom.InsetsHV(12, 0)),
					Child: w.Row{Main: w.MainCenter, Cross: w.CrossCenter, Spacing: 8, Children: inner}}}},
		}
		kids = append(kids, w.Expanded{Child: cell})
		if !last {
			kids = append(kids, w.SizedBox{Width: max(bw, 1), Height: height, Child: w.DecoratedBox{Color: border}})
		}
	}
	return w.CustomPaint{
		ForegroundPainter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			if bw > 0 {
				c.StrokeRoundRect(geom.RectFrom(o, size), min(radius, size.H/2), bw, border)
			}
		},
		Child: w.Row{ShrinkMain: false, Children: kids},
	}
}

// helpers shared by components
func textOf(ctx w.BuildContext, st text.Style, c geom.Color, s string) w.Widget {
	return w.Text{Text: s, Style: Styled(st, c)}
}
