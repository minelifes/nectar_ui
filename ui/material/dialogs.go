package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// ShowDialog opens builder's widget as a modal dialog over a scrim. The
// dialog closes on Escape, on a scrim tap, or with material.Pop(ctx, result);
// onClose (optional) receives the result.
func ShowDialog(ctx w.BuildContext, builder func(ctx w.BuildContext) w.Widget, onClose func(result any)) {
	nav := w.NavigatorOf(ctx)
	if nav == nil {
		return
	}
	scrim := ThemeOf(ctx).Scheme.Scrim.WithAlpha(0.32)
	nav.Push(&w.Route{
		Barrier: scrim, BarrierDismissible: true, Duration: 200 * time.Millisecond, OnPop: onClose,
		Builder: func(ctx w.BuildContext) w.Widget {
			return w.Center{Child: w.Padding{Padding: geom.Insets(24), Child: builder(ctx)}}
		},
		Transition: func(child w.Widget, t float32) w.Widget {
			if t >= 1 {
				return child
			}
			return w.Opacity{Opacity: t, Child: w.Translate{Offset: geom.Pt(0, (1-t)*24), Child: child}}
		},
	})
}

// Dialog is the bare M3 dialog container (for custom content).
type Dialog struct {
	Child   w.Widget
	Padding *geom.EdgeInsets // default 24
}

func (d Dialog) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	pad := geom.Insets(24)
	if d.Padding != nil {
		pad = *d.Padding
	}
	return w.ConstrainedBox{Constraints: geom.Constraints{MinW: 280, MaxW: 560, MaxH: geom.Inf},
		Child: w.AbsorbPointer{Child: Surface{Color: s.SurfaceContainerHigh, Radius: CornerExtraLarge, Elevation: 3,
			Child: w.Padding{Padding: pad, Child: d.Child}}}}
}

// AlertDialog shows a title, supporting text and actions.
type AlertDialog struct {
	Icon    *vector.Icon
	Title   string
	Content string
	// ContentWidget replaces Content.
	ContentWidget w.Widget
	Actions       []w.Widget // usually TextButtons, right-aligned
}

func (d AlertDialog) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	s := th.Scheme
	centered := d.Icon != nil
	cross := w.CrossStart
	align := 0
	if centered {
		cross = w.CrossCenter
		align = 1
	}
	kids := []w.Widget{}
	if d.Icon != nil {
		kids = append(kids, w.Icon{Icon: d.Icon, Size: 24, Color: s.Secondary}, w.SizedBox{Height: 16})
	}
	if d.Title != "" {
		ta := textAlign(align)
		kids = append(kids, w.Text{Text: d.Title, Style: Styled(th.Text.HeadlineSmall, s.OnSurface), Align: ta}, w.SizedBox{Height: 16})
	}
	if d.ContentWidget != nil {
		kids = append(kids, d.ContentWidget)
	} else if d.Content != "" {
		kids = append(kids, w.Text{Text: d.Content, Style: Styled(th.Text.BodyMedium, s.OnSurfaceVariant)})
	}
	if len(d.Actions) > 0 {
		kids = append(kids, w.SizedBox{Height: 24}, w.Row{Main: w.MainEnd, Spacing: 8, Children: d.Actions})
	}
	return Dialog{Child: w.Column{Cross: cross, ShrinkMain: true, Children: kids}}
}

// SimpleDialog offers a list of choices.
type SimpleDialog struct {
	Title   string
	Options []w.Widget // e.g. ListTiles; tap them to Pop
}

func (d SimpleDialog) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	pad := geom.InsetsLTRB(0, 24, 0, 12)
	kids := []w.Widget{}
	if d.Title != "" {
		kids = append(kids, w.Padding{Padding: geom.InsetsLTRB(24, 0, 24, 16),
			Child: w.Text{Text: d.Title, Style: Styled(th.Text.HeadlineSmall, th.Scheme.OnSurface)}})
	}
	kids = append(kids, d.Options...)
	return Dialog{Padding: &pad, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: kids}}
}

// ShowFullScreenDialog opens a full-screen dialog page (with a close button
// app bar supplied by you).
func ShowFullScreenDialog(ctx w.BuildContext, builder func(ctx w.BuildContext) w.Widget) {
	nav := w.NavigatorOf(ctx)
	if nav == nil {
		return
	}
	nav.Push(&w.Route{Opaque: true, Duration: 300 * time.Millisecond,
		Builder: func(ctx w.BuildContext) w.Widget {
			return w.DecoratedBox{Color: ThemeOf(ctx).Scheme.Surface, Child: builder(ctx)}
		},
		Transition: func(child w.Widget, t float32) w.Widget {
			if t >= 1 {
				return child
			}
			return w.Translate{Fraction: geom.Pt(0, 1-t), Child: child}
		}})
}

// ---------------------------------------------------------------------------
// Bottom sheets

// ShowModalBottomSheet slides builder's content up from the bottom over a
// scrim. Close it with material.Pop(ctx, result).
func ShowModalBottomSheet(ctx w.BuildContext, builder func(ctx w.BuildContext) w.Widget, onClose func(result any)) {
	nav := w.NavigatorOf(ctx)
	if nav == nil {
		return
	}
	scrim := ThemeOf(ctx).Scheme.Scrim.WithAlpha(0.32)
	nav.Push(&w.Route{
		Barrier: scrim, BarrierDismissible: true, Duration: 300 * time.Millisecond, OnPop: onClose,
		Builder: func(ctx w.BuildContext) w.Widget {
			return w.Align{Alignment: geom.BottomCenter, Child: BottomSheet{Child: builder(ctx)}}
		},
		Transition: func(child w.Widget, t float32) w.Widget {
			return w.Translate{Fraction: geom.Pt(0, 1-t), Child: child}
		},
	})
}

// BottomSheet is the sheet container: rounded top, drag handle.
type BottomSheet struct {
	Child      w.Widget
	HideHandle bool
}

func (b BottomSheet) Build(ctx w.BuildContext) w.Widget {
	s := ThemeOf(ctx).Scheme
	bg := s.SurfaceContainerLow
	shadow := s.Shadow
	kids := []w.Widget{}
	if !b.HideHandle {
		kids = append(kids, w.Padding{Padding: geom.InsetsHV(0, 22), Child: w.Center{
			Child: w.Container{Width: 32, Height: 4, Color: s.OnSurfaceVariant.WithAlpha(0.4), Border: &geom.Border{Radius: 2}}}})
	}
	kids = append(kids, b.Child)
	return w.ConstrainedBox{Constraints: geom.Constraints{MaxW: 640, MaxH: geom.Inf},
		Child: w.AbsorbPointer{Child: w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				// Only the top corners are rounded: extend the shape below.
				r := geom.Rect{X: o.X, Y: o.Y, W: size.W, H: size.H + CornerExtraLarge}
				paintShadow(c, r, CornerExtraLarge, 1, shadow)
				c.FillRoundRect(r, CornerExtraLarge, bg)
			},
			Child: w.Padding{Padding: geom.InsetsLTRB(0, 0, 0, 24), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: kids}},
		}}}
}

func textAlign(center int) textAlignT {
	if center == 1 {
		return alignCenter
	}
	return alignStart
}
