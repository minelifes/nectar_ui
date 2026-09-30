package tests

import (
	"reflect"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

var (
	tRed   = geom.Hex(0xE53935)
	tGreen = geom.Hex(0x43A047)
	tBlue  = geom.Hex(0x1E88E5)
)

// sameRGB compares colors ignoring alpha (state layers fade their color).
func sameRGB(a, b geom.Color) bool {
	d := func(x, y float32) bool { return x-y < 0.002 && y-x < 0.002 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

// rectsIn returns the filled rects drawn in color c.
func rectsIn(cv *render.Canvas, c geom.Color) []render.Command {
	var out []render.Command
	for _, cmd := range cv.Commands {
		if cmd.Kind == render.CmdRect && cmd.Color.A > 0 && sameRGB(cmd.Color, c) {
			out = append(out, cmd)
		}
	}
	return out
}

func themed(th m.Theme, child w.Widget) w.Widget {
	return m.App{Theme: th, Home: w.Align{Alignment: geom.TopLeft, Child: child}}
}

func TestStylePrecedence(t *testing.T) {
	btn := func(style m.ButtonStyle) w.Widget {
		return m.FilledButton{Label: "Go", OnPressed: func() {}, Style: style}
	}
	def := m.NewTheme(m.BaselineSeed, false).Scheme.Primary

	// Default: the scheme's primary.
	if len(rectsIn(tester.New(themed(m.Theme{}, btn(m.ButtonStyle{})), 300, 100).Pump(), def)) == 0 {
		t.Fatal("default filled button isn't primary")
	}
	// Theme wins over the default.
	th := m.NewTheme(m.BaselineSeed, false)
	th.FilledButton = m.ButtonStyle{BackgroundColor: tRed, Radius: m.Dp(4)}
	cv := tester.New(themed(th, btn(m.ButtonStyle{})), 300, 100).Pump()
	rs := rectsIn(cv, tRed)
	if len(rs) == 0 || rs[0].Radius != 4 {
		t.Fatalf("theme style not applied: %v", rs)
	}
	// The widget's Style wins over the theme, field by field.
	cv = tester.New(themed(th, btn(m.ButtonStyle{BackgroundColor: tGreen})), 300, 100).Pump()
	if len(rectsIn(cv, tRed)) != 0 {
		t.Fatal("theme color used although Style set one")
	}
	rs = rectsIn(cv, tGreen)
	if len(rs) == 0 || rs[0].Radius != 4 {
		t.Fatalf("Style should override color only, keeping the theme's radius: %v", rs)
	}
}

func TestZeroValuesAndSentinels(t *testing.T) {
	card := func(st m.CardTheme) *render.Canvas {
		return tester.New(themed(m.Theme{}, m.Card{Style: st, Child: w.SizedBox{Width: 80, Height: 40}}), 200, 100).Pump()
	}
	sc := m.NewTheme(m.BaselineSeed, false).Scheme
	shadows := func(cv *render.Canvas) int {
		n := 0
		for _, c := range cv.Commands {
			if c.Kind == render.CmdShadow {
				n++
			}
		}
		return n
	}
	cv := card(m.CardTheme{})
	if rs := rectsIn(cv, sc.SurfaceContainerLow); len(rs) == 0 || rs[0].Radius != m.CornerMedium || shadows(cv) == 0 {
		t.Fatal("default elevated card: surface-container-low, 12px corners, shadow")
	}
	// Dp(0) and Ptr(0) are real values, not "unset".
	cv = card(m.CardTheme{Radius: m.Dp(0), Elevation: w.Ptr(0)})
	if rs := rectsIn(cv, sc.SurfaceContainerLow); len(rs) == 0 || rs[0].Radius != 0 || shadows(cv) != 0 {
		t.Fatal("Radius Dp(0) / Elevation 0 should give square corners and no shadow")
	}
	// Transparent removes the fill; the zero color means "default".
	cv = card(m.CardTheme{Color: m.Transparent})
	if len(rectsIn(cv, sc.SurfaceContainerLow)) != 0 {
		t.Fatal("Transparent should replace the card color")
	}
	// A border color alone draws a 1px border.
	cv = card(m.CardTheme{BorderColor: tRed})
	found := false
	for _, c := range cv.Commands {
		found = found || c.Kind == render.CmdStroke && sameRGB(c.Color, tRed) && c.Width == 1
	}
	if !found {
		t.Fatal("BorderColor alone should draw a 1px border")
	}
}

func TestThemeScopeRestylesSubtree(t *testing.T) {
	th := m.NewTheme(m.BaselineSeed, false)
	tt := tester.New(m.App{Theme: th, Home: w.Column{ShrinkMain: true, Children: []w.Widget{
		m.Divider{},
		w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
			local := m.ThemeOf(ctx)
			local.Divider = m.DividerTheme{Color: tRed, Thickness: m.Dp(3)}
			return m.ThemeScope{Theme: local, Child: m.Divider{}}
		}},
	}}}, 200, 100)
	cv := tt.Pump()
	rs := rectsIn(cv, tRed)
	if len(rs) != 1 || rs[0].Rect.H != 3 {
		t.Fatalf("scoped divider: %v", rs)
	}
	if len(rectsIn(cv, th.Scheme.OutlineVariant)) != 1 {
		t.Fatal("divider outside the scope should keep the default look")
	}
}

func TestDarkModeKeepsComponentThemes(t *testing.T) {
	var got m.Theme
	grab := w.Builder{Builder: func(ctx w.BuildContext) w.Widget { got = m.ThemeOf(ctx); return w.SizedBox{} }}
	th := m.NewTheme(m.BaselineSeed, false)
	th.Card.Radius = m.Dp(3)
	tester.New(m.App{Theme: th, Dark: true, Home: grab}, 100, 100)
	if !got.Scheme.Dark || got.Card.Radius == nil || *got.Card.Radius != 3 {
		t.Fatalf("dark theme lost component themes: dark=%v card=%v", got.Scheme.Dark, got.Card.Radius)
	}
	if got.Text.BodyMedium.Color != got.Scheme.OnSurface {
		t.Fatal("text styles should follow the dark scheme")
	}
	// Without customizations the dark theme is exactly NewTheme(seed, true).
	tester.New(m.App{Dark: true, Home: grab}, 100, 100)
	if !reflect.DeepEqual(got, m.NewTheme(m.BaselineSeed, true)) {
		t.Fatal("default dark theme differs from NewTheme(baseline, true)")
	}
}

// themeReader counts builds of a widget that depends on the theme.
type themeReader struct{ n *int }

func (r themeReader) Build(ctx w.BuildContext) w.Widget {
	*r.n++
	_ = m.ThemeOf(ctx)
	return w.SizedBox{}
}

func TestEqualThemeWithPointersDoesNotRebuild(t *testing.T) {
	builds := 0
	mk := func() w.Widget {
		th := m.NewTheme(m.BaselineSeed, false)
		th.Card = m.CardTheme{Radius: m.Dp(4)} // a fresh pointer every time
		return m.App{Theme: th, Home: themeReader{n: &builds}}
	}
	tt := tester.New(mk(), 100, 100)
	before := builds
	tt.Root.SetApp(mk(), geom.Transparent)
	tt.Pump()
	if builds != before {
		t.Fatalf("an equal theme rebuilt its dependents (%d → %d)", before, builds)
	}
}

// ctxGrab exposes a BuildContext below the App for Show* calls.
type ctxGrab struct{ ctx *w.BuildContext }

func (g ctxGrab) Build(ctx w.BuildContext) w.Widget {
	*g.ctx = ctx
	return w.SizedBox{Width: 10, Height: 10}
}

func TestThemeInteractionFields(t *testing.T) {
	th := m.NewTheme(m.BaselineSeed, false)
	th.TextButton = m.ButtonStyle{OverlayColor: tRed}
	th.SnackBar = m.SnackBarTheme{BackgroundColor: tGreen}
	th.Menu = m.MenuTheme{BackgroundColor: tBlue, ItemHeight: m.Dp(30)}
	th.Dialog = m.DialogTheme{BarrierColor: tRed.WithAlpha(0.5), BackgroundColor: tBlue}
	th.Tooltip = m.TooltipTheme{Color: tGreen, WaitDuration: 100 * time.Millisecond}
	var ctx w.BuildContext
	tt := tester.New(m.App{Theme: th, Home: w.Column{ShrinkMain: true, Cross: w.CrossStart, Children: []w.Widget{
		ctxGrab{ctx: &ctx},
		m.TextButton{Label: "hover me", OnPressed: func() {}},
		m.Tooltip{Message: "tip", Child: w.Text{Text: "tooltip"}},
	}}}, 400, 300)

	r, _ := tt.Find("hover me")
	tt.Hover(r.X+2, r.Y+2)
	tt.Advance(300 * time.Millisecond)
	if len(rectsIn(tt.Pump(), tRed)) == 0 {
		t.Error("OverlayColor not used for the hover state layer")
	}

	r, _ = tt.Find("tooltip")
	tt.Hover(r.X+2, r.Y+2)
	tt.Advance(150 * time.Millisecond)
	if len(rectsIn(tt.Pump(), tGreen)) == 0 {
		t.Error("tooltip theme (color, wait duration) not used")
	}
	tt.Hover(390, 290)

	m.ShowSnackBar(ctx, m.SnackBar{Message: "saved"})
	tt.Advance(300 * time.Millisecond)
	if len(rectsIn(tt.Pump(), tGreen)) == 0 {
		t.Error("SnackBarTheme.BackgroundColor not used")
	}

	m.ShowMenu(ctx, geom.Rect{X: 10, Y: 10, W: 10, H: 10}, []m.MenuItem{{Label: "one"}, {Label: "two"}}, m.MenuOptions{})
	tt.Advance(300 * time.Millisecond)
	rs := rectsIn(tt.Pump(), tBlue)
	if len(rs) == 0 || rs[0].Rect.H != 2*30+16 {
		t.Errorf("MenuTheme not used: %v", rs)
	}
	tt.Key(w.KeyEscape)
	tt.Pump()

	m.ShowDialog(ctx, func(w.BuildContext) w.Widget { return m.AlertDialog{Title: "t"} }, nil)
	tt.Advance(300 * time.Millisecond)
	cv := tt.Pump()
	if len(rectsIn(cv, tBlue)) == 0 {
		t.Error("DialogTheme.BackgroundColor not used")
	}
	barrier := false
	for _, c := range cv.Commands {
		barrier = barrier || c.Kind == render.CmdRect && sameRGB(c.Color, tRed) && c.Color.A > 0.4 && c.Rect.W >= 400
	}
	if !barrier {
		t.Error("DialogTheme.BarrierColor not used")
	}
}

// Explicit widget settings win over the theme, even for bool-like fields
// whose zero value can't be told apart from "unset".
func TestExplicitWidgetSettingsBeatTheme(t *testing.T) {
	th := m.NewTheme(m.BaselineSeed, false)
	th.Input = m.InputDecorationTheme{Outlined: w.Ptr(false)}
	th.AppBar = m.AppBarTheme{CenterTitle: w.Ptr(false)}
	strokes := func(cv *render.Canvas) int {
		n := 0
		for _, c := range cv.Commands {
			if c.Kind == render.CmdStroke {
				n++
			}
		}
		return n
	}
	cv := tester.New(themed(th, w.SizedBox{Width: 300, Child: m.TextField{Label: "x", Outlined: true}}), 400, 200).Pump()
	if strokes(cv) == 0 {
		t.Error("TextField{Outlined: true} drawn filled because the theme said Outlined=false")
	}

	// title returns where the title's text starts (its alignment offset).
	title := func(th m.Theme, v m.AppBarVariant) float32 {
		cv := tester.New(themed(th, w.SizedBox{Width: 400, Child: m.AppBar{Title: "Title", Variant: v}}), 400, 200).Pump()
		for _, c := range cv.Commands {
			if c.Kind == render.CmdText && c.Text != nil && len(c.Text.Lines) > 0 {
				return c.Rect.X + c.Text.Lines[0].X
			}
		}
		t.Fatal("no title drawn")
		return 0
	}
	if title(th, m.AppBarCenterAligned) == title(th, m.AppBarSmall) {
		t.Error("AppBar{Variant: AppBarCenterAligned} lost its centered title to Theme.AppBar.CenterTitle=false")
	}
	th.AppBar.CenterTitle = w.Ptr(true)
	if title(th, m.AppBarSmall) != title(th, m.AppBarCenterAligned) {
		t.Error("Theme.AppBar.CenterTitle=true should center small app bars")
	}
}

// A plain widgets.Provider[Theme] (how themes were provided before
// ThemeScope compared them by value) still works.
func TestProviderThemeStillFound(t *testing.T) {
	th := m.NewTheme(m.BaselineSeed, false)
	th.Divider.Color = tRed
	cv := tester.New(w.Provider[m.Theme]{Value: th, Child: w.Column{ShrinkMain: true, Children: []w.Widget{m.Divider{}}}}, 200, 50).Pump()
	if len(rectsIn(cv, tRed)) == 0 {
		t.Error("ThemeOf ignores a widgets.Provider[Theme]")
	}
}
