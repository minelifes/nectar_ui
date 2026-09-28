package tests

import (
	"github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// --- a stateful test widget ---

type counter struct{ Label string }

func (counter) CreateState() widgets.State { return &counterState{} }

type counterState struct {
	widgets.StateBase
	n        int
	inits    int
	disposed bool
}

func (s *counterState) InitState() { s.inits++ }
func (s *counterState) Dispose()   { s.disposed = true }
func (s *counterState) Build(widgets.BuildContext) widgets.Widget {
	return widgets.Text{Text: widgets.WidgetOf[counter](s).Label + string(rune('0'+s.n))}
}

func setup(app widgets.Widget) (*widgets.Root, *widgets.BuildOwner, *render.PipelineOwner) {
	bo := widgets.NewBuildOwner()
	po := render.NewPipelineOwner()
	root := widgets.Mount(app, geom.White, bo, po)
	po.FlushLayout(geom.Size{W: 400, H: 300})
	return root, bo, po
}

func findParagraphs(ro render.RenderObject, out *[]*render.RenderParagraph) {
	if p, ok := ro.(*render.RenderParagraph); ok {
		*out = append(*out, p)
	}
	ro.VisitChildren(func(c render.RenderObject) { findParagraphs(c, out) })
}

func paragraphs(po *render.PipelineOwner) []string {
	var ps []*render.RenderParagraph
	findParagraphs(po.Root(), &ps)
	var s []string
	for _, p := range ps {
		s = append(s, p.Text())
	}
	return s
}

func TestSetStateRebuildsAndUpdatesRenderTree(t *testing.T) {
	var st *counterState
	root, bo, po := setup(widgets.Center{Child: counter{Label: "n="}})
	root.element.visitChildren(func(widgets.Element) {})
	// grab the state
	var walk func(e widgets.Element)
	walk = func(e widgets.Element) {
		if se, ok := e.(*widgets.statefulElement); ok {
			st = se.state.(*counterState)
		}
		e.visitChildren(walk)
	}
	walk(root.element)
	if st == nil || st.inits != 1 {
		t.Fatal("state not created / InitState not called once")
	}
	before := paragraphs(po)
	st.SetState(func() { st.n = 5 })
	bo.FlushBuild()
	after := paragraphs(po)
	if before[0] != "n=0" || after[0] != "n=5" {
		t.Fatalf("before=%v after=%v", before, after)
	}
}

func TestKeyedChildrenKeepState(t *testing.T) {
	mk := func(order ...string) widgets.Widget {
		var kids []widgets.Widget
		for _, k := range order {
			kids = append(kids, widgets.KeyedSubtree{ID: k, Child: counter{Label: k}})
		}
		return widgets.Column{Children: kids}
	}
	root, bo, po := setup(mk("a", "b"))
	states := map[string]*counterState{}
	var walk func(e widgets.Element)
	walk = func(e widgets.Element) {
		if se, ok := e.(*widgets.statefulElement); ok {
			s := se.state.(*counterState)
			states[widgets.WidgetOf[counter](s).Label] = s
		}
		e.visitChildren(walk)
	}
	walk(root.element)
	states["a"].SetState(func() { states["a"].n = 1 })
	states["b"].SetState(func() { states["b"].n = 2 })
	bo.FlushBuild()

	root.SetApp(mk("b", "a"), geom.White)
	bo.FlushBuild()
	got := paragraphs(po)
	if len(got) != 2 || got[0] != "b2" || got[1] != "a1" {
		t.Fatalf("state not preserved across reorder: %v", got)
	}

	root.SetApp(mk("b"), geom.White)
	if !states["a"].disposed {
		t.Fatal("removed child should be disposed")
	}
	if states["b"].inits != 1 {
		t.Fatal("kept child should not be re-initialized")
	}
}

func TestSwapWidgetTypeReattachesRenderObject(t *testing.T) {
	root, _, po := setup(widgets.Text{Text: "one"})
	root.SetApp(widgets.Padding{Padding: geom.Insets(4), Child: widgets.Text{Text: "two"}}, geom.White)
	view := po.Root().(*render.RenderView)
	if _, ok := view.Child().(*render.RenderPadding); !ok {
		t.Fatalf("root child = %T, want *RenderPadding", view.Child())
	}
	if got := paragraphs(po); len(got) != 1 || got[0] != "two" {
		t.Fatalf("got %v", got)
	}
}

func TestDefaultTextStyleInheritance(t *testing.T) {
	root, _, po := setup(widgets.DefaultTextStyle{Style: text.Style{Size: 30}, Child: widgets.Text{Text: "x"}})
	var ps []*render.RenderParagraph
	findParagraphs(po.Root(), &ps)
	po.FlushLayout(geom.Size{W: 400, H: 300})
	if ps[0].Paragraph().Style.Size != 30 {
		t.Fatalf("size %v", ps[0].Paragraph().Style.Size)
	}
	root.SetApp(widgets.DefaultTextStyle{Style: text.Style{Size: 10}, Child: widgets.Text{Text: "x"}}, geom.White)
	root.owner.FlushBuild() // Text is skipped by SetApp and rebuilt as a dependent
	po.FlushLayout(geom.Size{W: 400, H: 300})
	if ps[0].Paragraph().Style.Size != 10 {
		t.Fatalf("size after update %v", ps[0].Paragraph().Style.Size)
	}
}

func TestLayoutRowWithExpanded(t *testing.T) {
	_, _, po := setup(widgets.Row{Children: []widgets.Widget{
		widgets.SizedBox{Width: 100, Height: 20},
		widgets.Expanded{Child: widgets.Container{Color: geom.Black}},
		widgets.SizedBox{Width: 50, Height: 20},
	}})
	flex := po.Root().(*render.RenderView).Child().(*render.RenderFlex)
	kids := flex.Children()
	mid := kids[1].Base()
	if mid.Size().W != 250 || mid.Offset().X != 100 {
		t.Fatalf("expanded child size=%v offset=%v", mid.Size(), mid.Offset())
	}
	if kids[2].Base().Offset().X != 350 {
		t.Fatalf("last child at %v", kids[2].Base().Offset())
	}
}

func TestPaintProducesCommands(t *testing.T) {
	_, _, po := setup(widgets.Container{
		Color: geom.Hex(0x336699), Radius: 8, Padding: geom.Insets(10),
		Child: widgets.Text{Text: "Hello"},
	})
	c := po.FlushPaint(geom.Size{W: 400, H: 300})
	var rects, texts int
	for _, cmd := range c.Commands {
		switch cmd.Kind {
		case render.CmdRect:
			rects++
		case render.CmdText:
			texts++
			if cmd.Rect.X != 10 || cmd.Rect.Y != 10 {
				t.Fatalf("text at %v,%v", cmd.Rect.X, cmd.Rect.Y)
			}
		}
	}
	if rects != 2 || texts != 1 { // background + container, one paragraph
		t.Fatalf("rects=%d texts=%d", rects, texts)
	}
}

func TestPostFromGoroutine(t *testing.T) {
	scheduled := 0
	bo := widgets.NewBuildOwner()
	bo.OnScheduleFrame = func() { scheduled++ }
	po := render.NewPipelineOwner()
	root := widgets.Mount(counter{Label: "p"}, geom.White, bo, po)
	st := root.element.child.(*widgets.statefulElement).state.(*counterState)
	done := make(chan struct{})
	go func() { st.Post(func() { st.n = 7 }); close(done) }()
	<-done
	bo.FlushBuild()
	if got := paragraphs(po); got[0] != "p7" || scheduled == 0 {
		t.Fatalf("got %v scheduled=%d", got, scheduled)
	}
}
