package tests

import (
	"fmt"
	"slices"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// responsive switches layout at a 600px breakpoint and counts its builds.
type responsive struct{ builds *int }

func (r responsive) Build(w.BuildContext) w.Widget {
	return w.LayoutBuilder{Builder: func(ctx w.BuildContext, c geom.Constraints) w.Widget {
		*r.builds++
		if c.MaxW >= 600 {
			return w.Row{Children: []w.Widget{w.Text{Text: "sidebar"}, w.Text{Text: fmt.Sprintf("wide %.0f", c.MaxW)}}}
		}
		return w.Text{Text: fmt.Sprintf("narrow %.0f", c.MaxW)}
	}}
}

func has(tt *tester.Tester, s string) bool { return slices.Contains(tt.Texts(), s) }

func TestLayoutBuilderBreakpoint(t *testing.T) {
	builds := 0
	tt := tester.New(responsive{builds: &builds}, 800, 400)
	if !has(tt, "sidebar") || !has(tt, "wide 800") {
		t.Fatalf("wide layout: %v", tt.Texts())
	}
	tt.Window.SetSize(420, 400)
	tt.Pump()
	if has(tt, "sidebar") || !has(tt, "narrow 420") {
		t.Fatalf("after shrinking: %v", tt.Texts())
	}
	tt.Window.SetSize(900, 400)
	tt.Pump()
	if !has(tt, "wide 900") {
		t.Fatalf("after growing: %v", tt.Texts())
	}
	// Same constraints: no rebuild, however many frames run.
	n := builds
	for range 5 {
		tt.Pump()
	}
	if builds != n {
		t.Fatalf("builder ran %d extra times with unchanged constraints", builds-n)
	}
}

// sideWidth puts a LayoutBuilder in a pane whose width the test controls,
// next to a sibling that rebuilds on its own.
type sideWidth struct {
	set    *func(float32)
	bump   *func()
	builds *int
}

func (sideWidth) CreateState() w.State { return &sideWidthState{width: 300} }

type sideWidthState struct {
	w.StateBase
	width float32
}

func (s *sideWidthState) Build(w.BuildContext) w.Widget {
	cfg := w.WidgetOf[sideWidth](s)
	*cfg.set = func(v float32) { s.SetState(func() { s.width = v }) }
	return w.Align{Alignment: geom.TopLeft, Child: w.Column{Cross: w.CrossStart, Children: []w.Widget{
		ticker{bump: cfg.bump},
		w.SizedBox{Width: s.width, Height: 100, Child: paneLabel{builds: cfg.builds}},
	}}}
}

// paneLabel is comparable, so the LayoutBuilder inside is only rebuilt by
// its own triggers (constraints, dependencies), not by the parent.
type paneLabel struct{ builds *int }

func (p paneLabel) Build(w.BuildContext) w.Widget {
	return w.LayoutBuilder{Builder: func(ctx w.BuildContext, c geom.Constraints) w.Widget {
		*p.builds++
		return w.Text{Text: fmt.Sprintf("pane %.0f", c.MaxW)}
	}}
}

func TestLayoutBuilderFollowsLocalConstraints(t *testing.T) {
	var set func(float32)
	var bump func()
	builds := 0
	tt := tester.New(sideWidth{set: &set, bump: &bump, builds: &builds}, 800, 400)
	if !has(tt, "pane 300") {
		t.Fatalf("initial: %v", tt.Texts())
	}
	// A sibling rebuilding (and relaying out the column) doesn't re-run it.
	n := builds
	for range 3 {
		bump()
		tt.Pump()
	}
	if builds != n {
		t.Fatalf("sibling rebuilds re-ran the builder %d times", builds-n)
	}
	set(250)
	tt.Pump()
	if !has(tt, "pane 250") || builds != n+1 {
		t.Fatalf("after resizing the pane: %v, %d builds", tt.Texts(), builds-n)
	}
	// Its size is the child's, laid out under the pane's constraints.
	r, ok := tt.Find("pane 250")
	if !ok || r.X != 0 || r.W > 250 {
		t.Fatalf("label rect %v", r)
	}
}

// counterBox keeps state; the test checks it survives builder re-runs.
type counterBox struct{}

func (counterBox) CreateState() w.State { return &counterBoxState{} }

type counterBoxState struct {
	w.StateBase
	n int
}

func (s *counterBoxState) Build(w.BuildContext) w.Widget {
	return w.GestureDetector{OnTap: func() { s.SetState(func() { s.n++ }) },
		Child: w.Text{Text: fmt.Sprintf("taps %d", s.n)}}
}

func TestLayoutBuilderKeepsStateAndHitTests(t *testing.T) {
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.LayoutBuilder{Builder: func(_ w.BuildContext, c geom.Constraints) w.Widget {
		return w.Column{Cross: w.CrossStart, Children: []w.Widget{
			w.Text{Text: fmt.Sprintf("max %.0f", c.MaxW)},
			counterBox{},
		}}
	}}}, 500, 300)
	if err := tt.TapText("taps 0"); err != nil {
		t.Fatal(err)
	}
	tt.Pump()
	if !has(tt, "taps 1") {
		t.Fatalf("tap inside the builder: %v", tt.Texts())
	}
	tt.Window.SetSize(320, 300)
	tt.Pump()
	if !has(tt, "max 320") || !has(tt, "taps 1") {
		t.Fatalf("state lost across a re-run: %v", tt.Texts())
	}
	tt.TapText("taps 1")
	tt.Pump()
	if !has(tt, "taps 2") {
		t.Fatalf("second tap: %v", tt.Texts())
	}
}

// themeName is read from a Provider inside the builder.
type themeName string

type providerHost struct{ set *func(themeName) }

func (providerHost) CreateState() w.State { return &providerHostState{v: "light"} }

type providerHostState struct {
	w.StateBase
	v themeName
}

var providerBuilds int

// providerChild is hoisted (comparable), so only the Provider can re-run it.
type providerChild struct{}

func (providerChild) Build(w.BuildContext) w.Widget {
	return w.LayoutBuilder{Builder: func(ctx w.BuildContext, c geom.Constraints) w.Widget {
		providerBuilds++
		return w.Text{Text: fmt.Sprintf("%s %.0f", w.MustOf[themeName](ctx), c.MaxW)}
	}}
}

func (s *providerHostState) Build(w.BuildContext) w.Widget {
	*w.WidgetOf[providerHost](s).set = func(v themeName) { s.SetState(func() { s.v = v }) }
	return w.Provider[themeName]{Value: s.v, Child: providerChild{}}
}

func TestLayoutBuilderRebuildsOnDependency(t *testing.T) {
	providerBuilds = 0
	var set func(themeName)
	tt := tester.New(providerHost{set: &set}, 400, 200)
	if !has(tt, "light 400") {
		t.Fatalf("initial: %v", tt.Texts())
	}
	set("dark")
	tt.Pump()
	if !has(tt, "dark 400") || providerBuilds != 2 {
		t.Fatalf("after provider change: %v (%d builds)", tt.Texts(), providerBuilds)
	}
	set("dark") // same value: nothing to do
	tt.Pump()
	if providerBuilds != 2 {
		t.Fatalf("unchanged provider re-ran the builder (%d builds)", providerBuilds)
	}
}

func TestLayoutBuilderUnboundedInScroll(t *testing.T) {
	var got geom.Constraints
	tt := tester.New(w.ScrollView{Child: w.LayoutBuilder{Builder: func(_ w.BuildContext, c geom.Constraints) w.Widget {
		got = c
		if !c.HasBoundedHeight() {
			return w.Text{Text: "unbounded"}
		}
		return w.Text{Text: "bounded"}
	}}}, 300, 200)
	if !has(tt, "unbounded") || got.MaxW != 300 {
		t.Fatalf("in a vertical scroll view: %v, constraints %+v", tt.Texts(), got)
	}
}

func TestLayoutBuilderSwapsAndRemovesChild(t *testing.T) {
	tt := tester.New(w.LayoutBuilder{Builder: func(_ w.BuildContext, c geom.Constraints) w.Widget {
		switch {
		case c.MaxW < 200:
			return nil // nothing at all
		case c.MaxW < 400:
			return w.Text{Text: "small"}
		}
		return w.Padding{Padding: geom.Insets(8), Child: w.Text{Text: "big"}}
	}}, 500, 200)
	steps := []struct {
		width float32
		want  []string
	}{{500, []string{"big"}}, {300, []string{"small"}}, {100, nil}, {450, []string{"big"}}}
	for _, st := range steps {
		tt.Window.SetSize(int(st.width), 200)
		tt.Pump()
		if got := tt.Texts(); !slices.Equal(got, st.want) {
			t.Fatalf("width %v: %v, want %v", st.width, got, st.want)
		}
	}
}
