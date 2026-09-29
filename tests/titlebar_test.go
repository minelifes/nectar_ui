package tests

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// titleBarApp is a window with a custom title bar: a title, a hover-only
// label, an action button and a body below.
func titleBarApp(pressed *int) w.Widget {
	return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		w.TitleBar{Height: 40, Color: geom.Hex(0x101418), Child: w.Row{Cross: w.CrossCenter, Children: []w.Widget{
			w.Text{Text: "Aether"},
			w.SizedBox{Width: 20},
			w.MouseRegion{Child: w.Text{Text: "hover only"}},
			w.Spacer{},
			w.GestureDetector{OnTap: func() { *pressed++ }, Child: w.Text{Text: "Settings"}},
		}}},
		w.Expanded{Child: w.Center{Child: w.GestureDetector{OnTap: func() {}, Child: w.Text{Text: "body"}}}},
	}}
}

func center(r geom.Rect) (float32, float32) { return r.X + r.W/2, r.Y + r.H/2 }

func TestTitleBarMacInsets(t *testing.T) {
	var pressed int
	tt := tester.New(titleBarApp(&pressed), 800, 500, tester.WithTitleBar(tester.MacTitleBar))
	r, ok := tt.Find("Aether")
	if !ok || r.X < 76+8 {
		t.Fatalf("title %v overlaps the traffic lights (want x >= 84)", r)
	}
	// The OS draws the window buttons: none of ours.
	if n := len(rectsOf(tt.Pump(), geom.Hex(0xC42B1C))); n != 0 {
		t.Fatalf("window buttons drawn on macOS")
	}
	if err := tt.TapText("Settings"); err != nil || pressed != 1 {
		t.Fatalf("action in the title bar: %v, pressed %d", err, pressed)
	}
	// Fullscreen hides the traffic lights: the inset goes away.
	tt.Window.TitleBarInfo = w.TitleBarInfo{Custom: true, SystemButtons: true}
	tt.Window.SetFullscreen(true)
	tt.Pump()
	if r, _ := tt.Find("Aether"); r.X >= 76 {
		t.Fatalf("fullscreen: title still inset at %v", r.X)
	}
}

func TestTitleBarFramelessButtons(t *testing.T) {
	var pressed int
	tt := tester.New(titleBarApp(&pressed), 800, 500, tester.WithTitleBar(tester.FramelessTitleBar))
	// Three 46px buttons at the right edge, full bar height.
	minX, maxX, closeX := float32(800-46*3+23), float32(800-46*2+23), float32(800-23)
	tt.Tap(minX, 20)
	tt.Tap(maxX, 20)
	if !tt.Window.Minimized || !tt.Window.Maximized {
		t.Fatalf("minimize %v maximize %v", tt.Window.Minimized, tt.Window.Maximized)
	}
	tt.Tap(maxX, 20) // restore
	if tt.Window.Maximized {
		t.Fatal("second tap didn't restore")
	}
	// Hovering close paints it red.
	tt.Hover(closeX, 20)
	if r := rectsOf(tt.Pump(), geom.Hex(0xC42B1C)); len(r) != 1 || r[0].Rect.X != 800-46 || r[0].Rect.H != 40 {
		t.Fatalf("close hover fill: %v", r)
	}
	tt.Hover(400, 300)
	if len(rectsOf(tt.Pump(), geom.Hex(0xC42B1C))) != 0 {
		t.Fatal("close stays red after the pointer left")
	}
	tt.Tap(closeX, 20)
	if !tt.Window.Closed {
		t.Fatal("close button didn't close")
	}
	// Content keeps out of the buttons.
	if r, _ := tt.Find("Settings"); r.X+r.W > 800-46*3 {
		t.Fatalf("action %v under the window buttons", r)
	}
}

func TestTitleBarHitTest(t *testing.T) {
	var pressed int
	tt := tester.New(titleBarApp(&pressed), 800, 500, tester.WithTitleBar(tester.FramelessTitleBar))
	hover, _ := tt.Find("hover only")
	settings, _ := tt.Find("Settings")
	title, _ := tt.Find("Aether")
	cases := []struct {
		name string
		x, y float32
		want render.WindowHit
	}{
		{"empty bar", 400, 20, render.WindowHitCaption},
		{"title text", title.X + 2, title.Y + 2, render.WindowHitCaption},
		{"hover-only label", hover.X + 2, hover.Y + 2, render.WindowHitCaption},
		{"action button", settings.X + 2, settings.Y + 2, render.WindowHitClient},
		{"minimize button", 800 - 46*3 + 10, 20, render.WindowHitClient},
		{"close button", 790 - 10, 20, render.WindowHitClient},
		{"body", 400, 300, render.WindowHitClient},
		{"top edge", 400, 2, render.WindowHitResizeN},
		{"left edge", 2, 300, render.WindowHitResizeW},
		{"bottom edge", 400, 498, render.WindowHitResizeS},
		{"right edge", 798, 300, render.WindowHitResizeE},
		{"top-left corner", 3, 3, render.WindowHitResizeNW},
		{"top-right corner", 797, 8, render.WindowHitResizeNE},
		{"bottom-left corner", 8, 497, render.WindowHitResizeSW},
		{"bottom-right corner", 797, 497, render.WindowHitResizeSE},
	}
	for _, c := range cases {
		if got := tt.Window.HitTest(c.x, c.y); got != c.want {
			t.Errorf("%s at (%v,%v): %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
	// Maximized windows have no resize border.
	tt.Window.Maximize()
	tt.Pump()
	if got := tt.Window.HitTest(400, 2); got != render.WindowHitCaption {
		t.Errorf("maximized top edge: %v, want caption", got)
	}
}

func TestTitleBarMaximizeGlyphFollowsWindow(t *testing.T) {
	tt := tester.New(w.TitleBar{}, 600, 400, tester.WithTitleBar(tester.FramelessTitleBar))
	icons := func() (ids []uint32) {
		for _, c := range tt.Pump().Commands {
			if c.Kind == render.CmdIcon {
				ids = append(ids, c.Icon.ID())
			}
		}
		return ids
	}
	before := icons()
	tt.Window.Maximize() // e.g. double-click on the caption: not via our button
	tt.Pump()
	after := icons()
	if len(before) != 3 || len(after) != 3 || before[1] == after[1] || before[0] != after[0] {
		t.Fatalf("glyphs before %v after %v: maximize should turn into restore", before, after)
	}
}

func TestTitleBarNativeIsPlainBar(t *testing.T) {
	var pressed int
	tt := tester.New(titleBarApp(&pressed), 800, 500)
	if r, _ := tt.Find("Aether"); r.X != 8 {
		t.Fatalf("title at %v, want the plain 8px padding", r.X)
	}
	if got := tt.Window.HitTest(400, 20); got != render.WindowHitClient {
		t.Fatalf("native title bar: %v, want client (nothing custom)", got)
	}
	if tt.Window.HitTest(400, 2) != render.WindowHitClient {
		t.Fatal("native window reports resize borders")
	}
}

func TestMaterialTitleBar(t *testing.T) {
	var opened bool
	app := m.App{Home: w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		m.TitleBar{Title: "Aether", Center: w.Text{Text: "Welcome"}, Actions: []w.Widget{
			m.TextButton{Label: "Tune", OnPressed: func() { opened = true }},
		}},
		w.Expanded{Child: w.SizedBox{}},
	}}}
	tt := tester.New(app, 900, 500, tester.WithTitleBar(tester.MacTitleBar))
	title, _ := tt.Find("Aether")
	mid, _ := tt.Find("Welcome")
	if title.X < 84 {
		t.Fatalf("title %v under the traffic lights", title)
	}
	if cx, _ := center(mid); cx < 440 || cx > 460 || mid.W > 200 {
		t.Fatalf("center widget at x %v, want the middle", cx)
	}
	if got := tt.Window.HitTest(center(mid)); got != render.WindowHitCaption {
		t.Fatalf("centered text blocks dragging: %v", got)
	}
	if err := tt.TapText("Tune"); err != nil || !opened {
		t.Fatalf("action: %v %v", err, opened)
	}
	if bg := m.NewTheme(m.BaselineSeed, false).Scheme.SurfaceContainer; len(rectsOf(tt.Pump(), bg)) == 0 {
		t.Fatal("no SurfaceContainer background")
	}
}
