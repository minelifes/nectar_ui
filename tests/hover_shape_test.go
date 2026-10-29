package tests

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// hoverLayers returns the fills drawn with the state-layer color c at 8%.
func hoverLayers(tt *tester.Tester, c geom.Color) []render.Command {
	want := c.WithAlpha(c.A * m.HoverOpacity)
	var out []render.Command
	for _, cmd := range tt.Pump().Commands {
		if cmd.Kind == render.CmdRect && cmd.Color == want {
			out = append(out, cmd)
		}
	}
	return out
}

func TestSegmentedButtonHoverFollowsThePill(t *testing.T) {
	th := m.NewTheme(geom.Hex(0x6750A4), false)
	tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: w.Padding{Padding: geom.Insets(20), Child: w.SizedBox{Width: 300,
		Child: m.SegmentedButton{Segments: []m.Segment{{Value: 1, Label: "Day"}, {Value: 2, Label: "Week"}, {Value: 3, Label: "Month"}},
			Selected: []any{2}, OnChanged: func([]any) {}}}}}}, 400, 120)
	tt.Settle()
	r, _ := tt.Find("Day")
	tt.Hover(r.X, r.Y+r.H/2)
	tt.Settle()
	layers := hoverLayers(tt, th.Scheme.OnSurface)
	if len(layers) == 0 {
		t.Fatal("no hover layer")
	}
	for _, l := range layers {
		if l.Round.Radius < 19 || l.Round.Rect.X != 20 || l.Round.Rect.W != 300 {
			t.Fatalf("hover layer not clipped to the pill: %+v", l.Round)
		}
	}
	// And in pixels: the corner outside the pill stays the page color.
	img, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	page := img.RGBAAt(5, 5)
	if c := img.RGBAAt(21, 21); c != page {
		t.Errorf("hover paints outside the rounded end: %v vs page %v", c, page)
	}
	if c := img.RGBAAt(40, 40); c == page {
		t.Errorf("no hover inside the first segment")
	}
}

func TestNavigationHoverOnIndicator(t *testing.T) {
	th := m.NewTheme(geom.Hex(0x6750A4), false)
	dests := []m.NavigationDestination{{Icon: icons.Home, Label: "Home"}, {Icon: icons.Settings, Label: "Settings"}}
	cases := map[string]struct {
		widget w.Widget
		w, h   float32
	}{
		"bar":  {w.Align{Alignment: geom.BottomCenter, Child: m.NavigationBar{Destinations: dests, OnSelected: func(int) {}}}, 64, 32},
		"rail": {w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Height: 400, Child: m.NavigationRail{Destinations: dests, OnSelected: func(int) {}}}}, 56, 32},
	}
	for name, c := range cases {
		tt := tester.New(m.App{Home: c.widget}, 400, 400)
		tt.Settle()
		lr, _ := tt.Find("Settings")
		tt.Hover(lr.X+lr.W/2, lr.Y+lr.H/2) // hovering the label highlights the pill
		tt.Settle()
		layers := hoverLayers(tt, th.Scheme.OnSurface)
		if len(layers) != 1 {
			t.Fatalf("%s: %d hover layers", name, len(layers))
		}
		l := layers[0]
		if l.Rect.W != c.w || l.Rect.H != c.h || l.Radius != c.h/2 {
			t.Errorf("%s: hover layer %v radius %v, want the %vx%v pill", name, l.Rect, l.Radius, c.w, c.h)
		}
		cx := l.Rect.X + l.Rect.W/2
		if d := cx - (lr.X + lr.W/2); d > 1 || d < -1 {
			t.Errorf("%s: pill centered at %v, label at %v", name, cx, lr.X+lr.W/2)
		}
		if l.Rect.Bottom() > lr.Y {
			t.Errorf("%s: pill %v overlaps the label %v", name, l.Rect, lr)
		}
	}
}

func TestFilledTextFieldHoverHasSquareBottom(t *testing.T) {
	th := m.NewTheme(geom.Hex(0x6750A4), false)
	tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: w.Padding{Padding: geom.Insets(20),
		Child: w.SizedBox{Width: 240, Child: m.TextField{Label: "Name"}}}}}, 400, 200)
	tt.Settle()
	tt.Hover(100, 40)
	tt.Settle()
	layers := hoverLayers(tt, th.Scheme.OnSurface)
	if len(layers) != 1 {
		t.Fatalf("%d hover layers", len(layers))
	}
	l := layers[0]
	// Drawn taller than the field and clipped at its bottom edge: the
	// bottom corners are square like the fill.
	if l.Rect.Bottom() <= l.Clip.Bottom() || l.Clip.Bottom() > 20+56+0.5 {
		t.Errorf("hover %v clip %v: bottom corners would be rounded", l.Rect, l.Clip)
	}
}
