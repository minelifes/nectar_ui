package tests

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// iconCount counts drawn icons: in these bars only the window buttons are
// icons.
func iconCount(tt *tester.Tester) int {
	n := 0
	for _, c := range tt.Pump().Commands {
		if c.Kind == render.CmdIcon {
			n++
		}
	}
	return n
}

func TestWindowControlsSet(t *testing.T) {
	cases := []struct {
		c                     w.WindowControls
		close, minim, maxim   bool
		str                   string
		effectiveIsAll, empty bool
	}{
		{0, true, true, true, "close+minimize+maximize", true, false},
		{w.AllControls, true, true, true, "close+minimize+maximize", true, false},
		{w.CloseControl, true, false, false, "close", false, false},
		{w.MinimizeControl | w.MaximizeControl, false, true, true, "minimize+maximize", false, false},
		{w.NoControls, false, false, false, "none", false, true},
	}
	for _, c := range cases {
		if c.c.Has(w.CloseControl) != c.close || c.c.Has(w.MinimizeControl) != c.minim || c.c.Has(w.MaximizeControl) != c.maxim {
			t.Errorf("%v: Has wrong", c.c)
		}
		if c.c.String() != c.str {
			t.Errorf("String() = %q, want %q", c.c.String(), c.str)
		}
		if (c.c.Effective() == w.AllControls) != c.effectiveIsAll || (c.c.Effective() == 0) != c.empty {
			t.Errorf("%v: Effective %v", c.c, c.c.Effective())
		}
	}
	if !w.NoControls.Has(0) {
		t.Error("an empty query should always hold")
	}
}

func TestFramelessOnlyClose(t *testing.T) {
	var pressed int
	tt := tester.New(titleBarApp(&pressed), 800, 500,
		tester.WithTitleBar(tester.FramelessTitleBar), tester.WithWindowControls(w.CloseControl))
	if n := iconCount(tt); n != 1 {
		t.Fatalf("%d window buttons drawn, want only close", n)
	}
	// The close button is the last 46px; content now reaches up to it.
	if got := tt.Window.HitTest(800-23, 20); got != render.WindowHitClient {
		t.Fatalf("close button: %v", got)
	}
	if got := tt.Window.HitTest(500, 20); got != render.WindowHitCaption {
		t.Fatalf("empty bar: %v, want caption", got)
	}
	if r, _ := tt.Find("Settings"); r.X+r.W > 800-46 || r.X+r.W < 800-46-20 {
		t.Fatalf("action at %v: should end right before the close button", r)
	}
	tt.Tap(800-23, 20)
	if !tt.Window.Closed {
		t.Fatal("close didn't close")
	}
}

// customChrome draws its own window buttons with NoControls.
type customChrome struct{}

func (customChrome) Build(ctx w.BuildContext) w.Widget {
	win := w.WindowOf(ctx)
	dot := func(label string, c uint32, f func()) w.Widget {
		return w.GestureDetector{OnTap: f, Child: w.SizedBox{Width: 24, Height: 24,
			Child: w.DecoratedBox{Color: geom.Hex(c), Child: w.Center{Child: w.Text{Text: label}}}}}
	}
	return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		w.TitleBar{Height: 36, Child: w.Row{Cross: w.CrossCenter, Spacing: 6, Children: []w.Widget{
			dot("x", 0xFF5F57, win.Close), dot("-", 0xFEBC2E, win.Minimize), w.Spacer{}, w.Text{Text: "Custom"},
		}}},
		w.Expanded{Child: w.SizedBox{}},
	}}
}

func TestNoControlsCompletelyCustom(t *testing.T) {
	for name, info := range map[string]w.TitleBarInfo{"mac": tester.MacTitleBar, "frameless": tester.FramelessTitleBar} {
		tt := tester.New(customChrome{}, 600, 400, tester.WithTitleBar(info), tester.WithWindowControls(w.NoControls))
		if n := iconCount(tt); n != 0 {
			t.Fatalf("%s: %d system-style buttons drawn", name, n)
		}
		// Nothing reserved for system buttons: the app's own start at the
		// padding.
		if r, _ := tt.Find("x"); r.X > 8+24 {
			t.Fatalf("%s: custom close at %v, want at the left edge", name, r)
		}
		if got := tt.Window.HitTest(300, 18); got != render.WindowHitCaption {
			t.Fatalf("%s: empty bar %v, want caption", name, got)
		}
		tt.TapText("-")
		tt.TapText("x")
		if !tt.Window.Minimized || !tt.Window.Closed {
			t.Fatalf("%s: custom buttons: minimized %v closed %v", name, tt.Window.Minimized, tt.Window.Closed)
		}
	}
}

func TestMacInsetShrinksWithHiddenButtons(t *testing.T) {
	var pressed int
	for _, c := range []struct {
		controls w.WindowControls
		minX     float32
		maxX     float32
	}{
		{0, 76 + 8, 76 + 8},
		{w.CloseControl, 36 + 8, 36 + 8},
		{w.CloseControl | w.MinimizeControl, 56 + 8, 56 + 8},
		{w.NoControls, 8, 8},
	} {
		tt := tester.New(titleBarApp(&pressed), 800, 500,
			tester.WithTitleBar(tester.MacTitleBar), tester.WithWindowControls(c.controls))
		if r, _ := tt.Find("Aether"); r.X < c.minX || r.X > c.maxX {
			t.Errorf("%v: title at x %v, want %v", c.controls, r.X, c.minX)
		}
		if n := iconCount(tt); n != 0 {
			t.Errorf("%v: drew %d buttons; macOS draws its own", c.controls, n)
		}
	}
}

// controlsToggle changes the window's buttons from a widget.
type controlsToggle struct{}

func (controlsToggle) Build(ctx w.BuildContext) w.Widget {
	win := w.WindowOf(ctx)
	return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		w.TitleBar{Child: w.Text{Text: "App"}},
		w.GestureDetector{OnTap: func() { win.SetControls(w.CloseControl) }, Child: w.Text{Text: "only close"}},
		w.GestureDetector{OnTap: func() { win.SetControls(0) }, Child: w.Text{Text: "all"}},
	}}
}

func TestSetControlsAtRuntime(t *testing.T) {
	tt := tester.New(controlsToggle{}, 600, 400, tester.WithTitleBar(tester.FramelessTitleBar))
	if n := iconCount(tt); n != 3 {
		t.Fatalf("%d buttons at start", n)
	}
	tt.TapText("only close")
	tt.Pump()
	if n := iconCount(tt); n != 1 || tt.Window.Controls() != w.CloseControl {
		t.Fatalf("after SetControls(close): %d buttons, controls %v", n, tt.Window.Controls())
	}
	tt.TapText("all")
	tt.Pump()
	if n := iconCount(tt); n != 3 {
		t.Fatalf("after SetControls(all): %d buttons", n)
	}
}

func TestWindowButtonsShowOverride(t *testing.T) {
	tt := tester.New(w.Align{Alignment: geom.TopRight, Child: w.WindowButtons{Show: w.MinimizeControl | w.CloseControl}},
		400, 100, tester.WithTitleBar(tester.FramelessTitleBar))
	if n := iconCount(tt); n != 2 {
		t.Fatalf("%d buttons, want minimize + close", n)
	}
	tt.Tap(400-46-23, 16) // minimize sits left of close
	if !tt.Window.Minimized {
		t.Fatal("minimize isn't where expected")
	}
}
