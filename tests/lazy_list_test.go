package tests

import (
	"fmt"
	"testing"

	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// resizableList is a lazy list whose length the test changes.
type resizableList struct{ set *func(int) }

func (resizableList) CreateState() w.State { return &resizableListState{n: 1000} }

type resizableListState struct {
	w.StateBase
	n int
}

func (s *resizableListState) Build(w.BuildContext) w.Widget {
	*w.WidgetOf[resizableList](s).set = func(n int) { s.SetState(func() { s.n = n }) }
	return w.ListViewBuilder{ItemCount: s.n, ItemExtent: 20, Builder: func(_ w.BuildContext, i int) w.Widget {
		return w.Text{Text: fmt.Sprint("row ", i)}
	}}
}

// A list that shrinks while scrolled far down (a big folder collapsing in
// a TreeView) used to panic with "makeslice: cap out of range", and must
// show its remaining rows.
func TestLazyListShrinksWhileScrolled(t *testing.T) {
	var set func(int)
	tt := tester.New(resizableList{set: &set}, 300, 200)
	tt.Scroll(100, 100, 15000)
	tt.Pump()
	if has(tt, "row 0") {
		t.Fatalf("not scrolled: %v", tt.Texts())
	}
	set(5)
	tt.Pump()
	for i := range 5 {
		if r, ok := tt.Find(fmt.Sprint("row ", i)); !ok || r.Y < 0 || r.Y > 200 {
			t.Fatalf("row %d after shrinking: %v %v (%v)", i, r, ok, tt.Texts())
		}
	}
	set(0)
	tt.Pump()
	if len(tt.Texts()) != 0 {
		t.Fatalf("empty list shows %v", tt.Texts())
	}
	set(1000)
	tt.Pump()
	if !has(tt, "row 0") {
		t.Fatalf("after growing back: %v", tt.Texts())
	}
}

// The same through a TreeView: collapse a big expanded folder while
// scrolled to its bottom.
func TestTreeCollapseWhileScrolled(t *testing.T) {
	var kids []w.TreeNode
	for i := range 500 {
		kids = append(kids, w.TreeNode{Key: fmt.Sprint("f", i), Label: fmt.Sprint("file ", i)})
	}
	ctrl := w.NewTreeController()
	tt := tester.New(w.TreeView{Controller: ctrl, Roots: []w.TreeNode{
		{Key: "src", Label: "src", Children: kids, Expanded: true},
		{Key: "docs", Label: "docs"},
	}}, 300, 200)
	tt.Scroll(100, 100, 20000)
	tt.Pump()
	if has(tt, "src") {
		t.Fatalf("not scrolled: %v", tt.Texts())
	}
	ctrl.CollapseAll()
	tt.Pump()
	if !has(tt, "src") || !has(tt, "docs") {
		t.Fatalf("after collapsing: %v", tt.Texts())
	}
}
