package tests

import (
	"image/color"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// paints counts how often staticPanel's painter runs.
var paints int

// staticPanel is an expensive-looking subtree that never changes.
type staticPanel struct{ color uint32 }

func (p staticPanel) Build(w.BuildContext) w.Widget {
	c := geom.Hex(p.color)
	return w.CustomPaint{Size: geom.Sz(100, 40), Painter: func(cv *render.Canvas, o geom.Offset, s geom.Size) {
		paints++
		cv.FillRect(geom.RectFrom(o, s), c)
	}}
}

// ticker rebuilds itself on demand (like an animation next to the panel).
type ticker struct{ bump *func() }

func (ticker) CreateState() w.State { return &tickerState{} }

type tickerState struct {
	w.StateBase
	n int
}

func (s *tickerState) Build(w.BuildContext) w.Widget {
	*w.WidgetOf[ticker](s).bump = func() { s.SetState(func() { s.n++ }) }
	return w.SizedBox{Width: float32(10 + s.n%2), Height: 10}
}

func boundaryOf(tt *tester.Tester) *render.RenderRepaintBoundary {
	var found *render.RenderRepaintBoundary
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if b, ok := ro.(*render.RenderRepaintBoundary); ok && found == nil {
			found = b
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	return found
}

func rectsOf(cv *render.Canvas, c geom.Color) []render.Command {
	var out []render.Command
	for _, cmd := range cv.Commands {
		if cmd.Kind == render.CmdRect && cmd.Color.R == c.R && cmd.Color.G == c.G && cmd.Color.B == c.B {
			out = append(out, cmd)
		}
	}
	return out
}

func TestRepaintBoundaryReplaysWhileSiblingAnimates(t *testing.T) {
	paints = 0
	var bump func()
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.Column{Cross: w.CrossStart, Children: []w.Widget{
		ticker{bump: &bump},
		w.RepaintBoundary{Child: staticPanel{0x00AA00}},
	}}}, 300, 200)
	first := paints
	for i := 0; i < 5; i++ {
		bump()
		cv := tt.Pump()
		if n := len(rectsOf(cv, geom.Hex(0x00AA00))); n != 1 {
			t.Fatalf("frame %d: panel drawn %d times", i, n)
		}
	}
	if paints != first {
		t.Fatalf("panel repainted %d times while only its sibling changed", paints-first)
	}
	if hits, misses := boundaryOf(tt).Stats(); hits < 5 || misses != 1 {
		t.Fatalf("stats: %d hits, %d misses", hits, misses)
	}
}

// panelHost puts a staticPanel (whose color can change) in a boundary,
// optionally shifted and faded.
type panelHost struct {
	set *func(func(*panelConf))
}

type panelConf struct {
	color   uint32
	shift   float32
	opacity float32
}

func (panelHost) CreateState() w.State { return &panelHostState{conf: panelConf{0x00AA00, 0, 1}} }

type panelHostState struct {
	w.StateBase
	conf panelConf
}

func (s *panelHostState) Build(w.BuildContext) w.Widget {
	*w.WidgetOf[panelHost](s).set = func(f func(*panelConf)) { s.SetState(func() { f(&s.conf) }) }
	return w.Align{Alignment: geom.TopLeft, Child: w.Padding{Padding: geom.InsetsLTRB(s.conf.shift, 0, 0, 0),
		Child: w.Opacity{Opacity: s.conf.opacity, Child: w.RepaintBoundary{Child: staticPanel{s.conf.color}}}}}
}

func TestRepaintBoundaryInvalidation(t *testing.T) {
	paints = 0
	var set func(func(*panelConf))
	tt := tester.New(panelHost{set: &set}, 300, 200)
	green := geom.Hex(0x00AA00)
	base := rectsOf(tt.Pump(), green)[0].Rect

	// Moving the boundary replays the cache, shifted.
	set(func(c *panelConf) { c.shift = 50 })
	cv := tt.Pump()
	if r := rectsOf(cv, green); len(r) != 1 || r[0].Rect.X != base.X+50 {
		t.Fatalf("moved boundary drawn at %v, want x %v", r, base.X+50)
	}
	// Fading it replays with the opacity applied.
	set(func(c *panelConf) { c.opacity = 0.5 })
	cv = tt.Pump()
	if r := rectsOf(cv, green); len(r) != 1 || r[0].Color.A < 0.49 || r[0].Color.A > 0.51 {
		t.Fatalf("faded boundary alpha: %v", r)
	}
	if _, misses := boundaryOf(tt).Stats(); misses != 1 || paints != 1 {
		t.Fatalf("moving/fading repainted the subtree: %d misses, %d paints", misses, paints)
	}
	// Changing what's inside re-records it.
	set(func(c *panelConf) { c.color = 0xAA0000 })
	cv = tt.Pump()
	if len(rectsOf(cv, geom.Hex(0xAA0000))) != 1 || len(rectsOf(cv, green)) != 0 {
		t.Fatal("changed content not repainted (stale cache)")
	}
	if _, misses := boundaryOf(tt).Stats(); misses != 2 {
		t.Fatalf("misses %d after a content change", misses)
	}
}

func TestScrollViewReplaysWhileScrolling(t *testing.T) {
	paints = 0
	ctrl := w.NewScrollController()
	var rows []w.Widget
	for i := 0; i < 20; i++ {
		rows = append(rows, staticPanel{0x0000AA + uint32(i)<<16})
	}
	tt := tester.New(w.SizedBox{Width: 100, Height: 200, Child: w.ScrollView{Controller: ctrl, Child: w.Column{Children: rows}}}, 100, 200)
	defer tt.Close()
	first := paints
	row3 := geom.Hex(0x0000AA + 3<<16)
	y0 := rectsOf(tt.Pump(), row3)[0].Rect.Y
	ctrl.JumpTo(55)
	cv := tt.Pump()
	if paints != first {
		t.Fatalf("scrolling repainted %d rows instead of replaying", paints-first)
	}
	r := rectsOf(cv, row3)
	if len(r) != 1 || r[0].Rect.Y != y0-55 {
		t.Fatalf("row 3 after scrolling: %v, want y %v", r, y0-55)
	}
	// Rows scrolled out of view are skipped when replaying.
	if len(cv.Commands) > 8 {
		t.Fatalf("%d commands for a 200px view of 40px rows", len(cv.Commands))
	}
	// And the pixels agree.
	snap, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	want := color.RGBA{0x03, 0x00, 0xAA, 0xFF} // row 3 is at y 120-55 = 65..105
	if got := snap.RGBAAt(50, 80); got != want {
		t.Fatalf("pixel at row 3: %v, want %v", got, want)
	}
}
