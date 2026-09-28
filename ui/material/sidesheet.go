package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// SideSheet shows secondary content along the right edge of a screen
// (M3 side sheet): a headline with close (and optional back) buttons, a
// scrollable body and optional actions at the bottom.
//
// Standard sheets sit next to the main content and push it aside; put one
// in a Row after an Expanded body and toggle Open. Modal sheets float over
// a scrim: open them with ShowModalSideSheet.
type SideSheet struct {
	Title   string
	Child   w.Widget
	Actions []w.Widget // e.g. FilledButton + OutlinedButton
	OnBack  func()     // shows a back arrow before the title
	OnClose func()     // shows the close button (standard sheets)
	Width   float32    // default 360 (M3 allows 256–400)
	// Open animates a standard sheet in and out (ignored for modal sheets).
	Open bool

	modal bool
}

func (SideSheet) CreateState() w.State { return &sideSheetState{} }

type sideSheetState struct {
	w.StateBase
	open *w.Animated
}

func (s *sideSheetState) InitState() {
	v := float32(0)
	if sh := w.WidgetOf[SideSheet](s); sh.Open || sh.modal {
		v = 1
	}
	s.open = w.NewAnimated(s, 250*time.Millisecond, w.Emphasized, v)
}

func (s *sideSheetState) Build(ctx w.BuildContext) w.Widget {
	sh := w.WidgetOf[SideSheet](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	width := sh.Width
	if width == 0 {
		width = 360
	}

	header := []w.Widget{}
	if sh.OnBack != nil {
		header = append(header, IconButton{Icon: iconArrowBack, Color: sc.OnSurfaceVariant, OnPressed: sh.OnBack})
	} else {
		header = append(header, w.SizedBox{Width: 12})
	}
	header = append(header, w.Expanded{Child: w.Padding{Padding: geom.InsetsHV(4, 0),
		Child: w.Text{Text: sh.Title, Style: Styled(th.Text.TitleLarge, sc.OnSurfaceVariant), MaxLines: 1, Ellipsis: true}}})
	if sh.OnClose != nil {
		header = append(header, IconButton{Icon: iconClose, Color: sc.OnSurfaceVariant, OnPressed: sh.OnClose})
	}
	col := []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(4, 12, 12, 12), Child: w.SizedBox{Height: 48, Child: w.Row{Cross: w.CrossCenter, Children: header}}},
	}
	body := sh.Child
	if body == nil {
		body = w.SizedBox{}
	}
	col = append(col, w.Expanded{Child: w.ScrollView{Padding: geom.InsetsLTRB(24, 0, 24, 16), Child: body}})
	if len(sh.Actions) > 0 {
		col = append(col, Divider{}, w.Padding{Padding: geom.InsetsLTRB(24, 16, 24, 24),
			Child: w.Row{ShrinkMain: true, Spacing: 8, Children: sh.Actions}})
	}
	content := w.SizedBox{Width: width, Child: w.Column{Cross: w.CrossStretch, Children: col}}

	if sh.modal {
		// Modal: rounded leading corners, elevation 1, container-low tone.
		bg, shadow := sc.SurfaceContainerLow, sc.Shadow
		return w.AbsorbPointer{Child: w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				// Only the leading (left) corners are rounded: extend right.
				r := geom.Rect{X: o.X, Y: o.Y, W: size.W + CornerLarge, H: size.H}
				paintShadow(c, r, CornerLarge, 1, shadow)
				c.FillRoundRect(r, CornerLarge, bg)
			},
			Child: content,
		}}
	}

	// Standard: surface color with a divider on the leading edge; animates
	// its width so the main content slides over.
	if sh.Open {
		s.open.Set(1)
	} else {
		s.open.Set(0)
	}
	t := s.open.Value()
	if t <= 0 {
		return w.SizedBox{}
	}
	line := sc.OutlineVariant
	return w.SizeTransition{Horizontal: true, Factor: t, Child: w.CustomPaint{
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			c.FillRect(geom.RectFrom(o, size), sc.Surface)
			c.FillRect(geom.Rect{X: o.X, Y: o.Y, W: 1, H: size.H}, line)
		},
		Child: content,
	}}
}

// ShowModalSideSheet slides sheet in from the right over a scrim. It
// closes on the close button, a scrim tap, Escape, or material.Pop(ctx,
// result); onClose (optional) receives the result.
func ShowModalSideSheet(ctx w.BuildContext, sheet SideSheet, onClose func(result any)) {
	nav := w.NavigatorOf(ctx)
	if nav == nil {
		return
	}
	scrim := ThemeOf(ctx).Scheme.Scrim.WithAlpha(0.32)
	nav.Push(&w.Route{
		Barrier: scrim, BarrierDismissible: true, Duration: 300 * time.Millisecond, OnPop: onClose,
		Builder: func(ctx w.BuildContext) w.Widget {
			sh := sheet
			sh.modal = true
			userClose := sheet.OnClose
			sh.OnClose = func() {
				Pop(ctx, nil)
				if userClose != nil {
					userClose()
				}
			}
			return sh
		},
		// Position the sheet here (not in Builder) so only the sheet
		// slides, by its own width.
		Transition: func(child w.Widget, t float32) w.Widget {
			return w.Stack{Expand: true, Children: []w.Widget{
				w.Positioned{Top: w.At(0), Bottom: w.At(0), Right: w.At(0),
					Child: w.Translate{Fraction: geom.Pt(1-t, 0), Child: child}},
			}}
		},
	})
}
