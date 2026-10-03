package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

func TestScrollView2D(t *testing.T) {
	h, v := w.NewScrollController(), w.NewScrollController()
	big := w.SizedBox{Width: 1000, Height: 800, Child: w.Stack{Children: []w.Widget{
		w.Positioned{Left: w.At(900), Top: w.At(700), Child: w.Text{Text: "corner"}},
	}}}
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 300, Height: 200,
		Child: w.ScrollView2D{Horizontal: h, Vertical: v, Child: big}}}, 400, 300)
	if h.MaxOffset() != 700 || v.MaxOffset() != 600 {
		t.Fatalf("max offsets %v %v", h.MaxOffset(), v.MaxOffset())
	}
	if _, ok := tt.Find("corner"); ok {
		t.Fatal("far corner visible before scrolling")
	}
	tt.ScrollXY(100, 100, 0, 650)               // wheel: vertical
	tt.ScrollXY(100, 100, 800, 0)               // sideways swipe: horizontal
	if h.Offset() != 700 || v.Offset() != 600 { // both clamped
		t.Fatalf("offsets %v %v", h.Offset(), v.Offset())
	}
	r, ok := tt.Find("corner")
	if !ok || r.X != 200 || r.Y != 100 {
		t.Fatalf("corner at %v %v", r, ok)
	}
	// Dragging pans both ways.
	tt.Drag(geom.Pt(150, 150), geom.Pt(250, 180))
	if h.Offset() >= 700 || v.Offset() >= 600 {
		t.Fatalf("drag didn't pan: %v %v", h.Offset(), v.Offset())
	}
	h.JumpTo(0)
	v.JumpTo(0)
	if _, ok := tt.Find("corner"); ok {
		t.Fatal("controllers don't drive the view")
	}
}

func TestVariableExtentListAndStickyHeaders(t *testing.T) {
	// 50 sections: a 40px header and 9 rows of 20px each.
	n := 500
	isHeader := func(i int) bool { return i%10 == 0 }
	extent := func(i int) float32 {
		if isHeader(i) {
			return 40
		}
		return 20
	}
	built := map[int]bool{}
	ctrl := w.NewScrollController()
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 200, Height: 300, Child: w.ListViewBuilder{
		Controller: ctrl, ItemCount: n, ItemExtentOf: extent, IsHeader: isHeader,
		Builder: func(_ w.BuildContext, i int) w.Widget {
			built[i] = true
			if isHeader(i) {
				return w.Text{Text: fmt.Sprintf("Section %d", i/10)}
			}
			return w.Text{Text: fmt.Sprintf("row %d", i)}
		}}}}, 300, 400)
	// Content height: 50 * (40 + 9*20) = 11000.
	if ctrl.MaxOffset() != 11000-300 {
		t.Fatalf("max offset %v", ctrl.MaxOffset())
	}
	built = map[int]bool{}
	ctrl.JumpTo(1) // rebuild now that the view's size is known
	tt.Pump()
	if len(built) > 40 {
		t.Fatalf("built %d rows for a 300px view", len(built))
	}
	// Scroll into section 3 (starts at 660): its header stays pinned on top.
	ctrl.JumpTo(700)
	tt.Pump()
	r, ok := tt.Find("Section 3")
	if !ok || r.Y != 0 {
		t.Fatalf("pinned header at %v %v", r, ok)
	}
	if r2, ok := tt.Find("row 33"); !ok || r2.Y != 660+40+2*20-700 {
		t.Fatalf("row 33 at %v %v", r2, ok)
	}
	// Near the end of section 3 the next header pushes it up.
	ctrl.JumpTo(880 - 20) // section 4 starts 20px below the top
	tt.Pump()
	if r, ok := tt.Find("Section 3"); !ok || r.Y != -20 {
		t.Fatalf("pushed header at %v %v", r, ok)
	}
	// Jumping far keeps building only what's visible.
	built = map[int]bool{}
	ctrl.JumpTo(10000)
	tt.Pump()
	for i := range built {
		if i < 400 && !isHeader(i) {
			t.Fatalf("built row %d far above the view", i)
		}
	}
}

func TestDataGrid(t *testing.T) {
	cols := []w.GridColumn{{Title: "Name", Width: 150, Resizable: true}, {Title: "Size", Width: 80, Align: text.AlignEnd}, {Title: "Kind", Width: 300}}
	var tapped = -1
	var sorted = -1
	var widths []float32
	v := w.NewScrollController()
	h := w.NewScrollController()
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 400, Height: 236, Child: w.DataGrid{
		Columns: cols, RowCount: 100000, Vertical: v, Horizontal: h,
		CellText:        func(r, c int) string { return fmt.Sprintf("r%dc%d", r, c) },
		OnRowTap:        func(r int) { tapped = r },
		OnHeaderTap:     func(c int) { sorted = c },
		OnColumnResized: func(c int, wd float32) { widths = append(widths, wd) },
		Style:           w.GridStyle{LineColor: geom.Hex(0xcccccc), ResizeHandle: geom.Hex(0x0000ff)},
	}}}, 500, 400)
	if _, ok := tt.Find("r0c0"); !ok {
		t.Fatal("first cell missing")
	}
	if texts := tt.Texts(); len(texts) > 3*20 {
		t.Fatalf("built %d cells for ~6 visible rows", len(texts))
	}
	// Header stays while rows scroll.
	v.JumpTo(32 * 5000)
	tt.Pump()
	if _, ok := tt.Find("r5000c1"); !ok {
		t.Fatal("row 5000 not shown")
	}
	if r, ok := tt.Find("Name"); !ok || r.Y > 20 {
		t.Fatalf("header moved: %v %v", r, ok)
	}
	tt.TapText("r5001c0")
	if tapped != 5001 {
		t.Fatalf("tapped row %d", tapped)
	}
	tt.TapText("Size")
	if sorted != 1 {
		t.Fatalf("header tap %d", sorted)
	}
	// Header and rows scroll sideways together; the wheel's vertical
	// delta doesn't scroll sideways.
	before := v.Offset()
	tt.ScrollXY(200, 150, 120, 0)
	nameH, _ := tt.Find("Name")
	cell, _ := tt.Find("r5001c0")
	if h.Offset() != 120 || nameH.X != cell.X || v.Offset() != before {
		t.Fatalf("sideways: h=%v header x=%v cell x=%v v=%v", h.Offset(), nameH.X, cell.X, v.Offset())
	}
	// Resize the first column by dragging its header edge.
	h.JumpTo(0)
	tt.Pump()
	tt.Drag(geom.Pt(147, 18), geom.Pt(197, 18))
	if len(widths) == 0 || widths[len(widths)-1] < 190 {
		t.Fatalf("resize: %v", widths)
	}
	if r, _ := tt.Find("r5001c1"); r.X < 200 {
		t.Fatalf("second column didn't move: %v", r)
	}
}

func TestDragAndDrop(t *testing.T) {
	var got []string
	var leaves int
	tt := tester.New(m.App{Home: w.Row{Cross: w.CrossStart, Children: []w.Widget{
		w.Draggable{Data: "apple", Child: w.SizedBox{Width: 80, Height: 40, Child: w.Text{Text: "drag me"}}},
		w.SizedBox{Width: 100},
		w.DragTarget{
			OnWillAccept: func(d any) bool { _, ok := d.(string); return ok },
			OnAccept:     func(d any, local geom.Offset) { got = append(got, fmt.Sprintf("%v@%.0f,%.0f", d, local.X, local.Y)) },
			OnLeave:      func(any) { leaves++ },
			Builder: func(_ w.BuildContext, cand any) w.Widget {
				label := "empty"
				if cand != nil {
					label = "over:" + cand.(string)
				}
				return w.SizedBox{Width: 120, Height: 100, Child: w.Text{Text: label}}
			},
		},
		w.DragTarget{OnWillAccept: func(any) bool { return false }, Builder: func(_ w.BuildContext, cand any) w.Widget {
			return w.SizedBox{Width: 100, Height: 100, Child: w.Text{Text: fmt.Sprintf("refuses %v", cand)}}
		}},
	}}}, 600, 300)
	// Move over the target: it shows the candidate; the feedback follows.
	tt.Pointer(render.PointerDown, 20, 20)
	tt.Pointer(render.PointerMove, 120, 30)
	tt.Pointer(render.PointerMove, 200, 30)
	if !contains(tt.Texts(), "over:apple") {
		t.Fatalf("hover: %v", tt.Texts())
	}
	n := 0
	for _, s := range tt.Texts() {
		if s == "drag me" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("feedback not shown (%d copies)", n)
	}
	// Leave and come back, then drop.
	tt.Pointer(render.PointerMove, 350, 30) // over the refusing target
	if leaves != 1 || contains(tt.Texts(), "refuses apple") {
		t.Fatalf("leave: %d %v", leaves, tt.Texts())
	}
	tt.Pointer(render.PointerMove, 200, 50)
	tt.Pointer(render.PointerUp, 200, 50)
	if len(got) != 1 || got[0] != "apple@20,50" {
		t.Fatalf("drop: %v", got)
	}
	if contains(tt.Texts(), "over:apple") {
		t.Fatal("target still highlighted after the drop")
	}
	// Dropping on nothing doesn't deliver.
	tt.Drag(geom.Pt(20, 20), geom.Pt(20, 250))
	if len(got) != 1 {
		t.Fatalf("dropped on nothing: %v", got)
	}
}

func TestFileDrop(t *testing.T) {
	var outer, inner []string
	tt := tester.New(w.FileDropTarget{OnDrop: func(p []string, _ geom.Offset) { outer = p }, Child: w.Align{Alignment: geom.TopLeft,
		Child: w.FileDropTarget{OnDrop: func(p []string, l geom.Offset) { inner = append(p, fmt.Sprint(l)) },
			Child: w.SizedBox{Width: 100, Height: 100}}}}, 300, 300)
	tt.DropFiles([]string{"/a.txt"}, 50, 60)
	tt.DropFiles([]string{"/b.txt"}, 250, 250)
	if strings.Join(inner, ",") != "/a.txt,{50 60}" || strings.Join(outer, ",") != "/b.txt" {
		t.Fatalf("inner %v outer %v", inner, outer)
	}
}

func TestDockController(t *testing.T) {
	c := w.NewDockController(nil)
	c.Open("a", "", w.DropCenter)
	c.Open("b", "", w.DropCenter)
	c.Open("c", "b", w.DropRight)  // split: [a b] | [c]
	c.Open("d", "c", w.DropBottom) // right side: [c] over [d]
	c.Open("e", "a", w.DropRight)  // same direction as root: a sibling
	l := c.Layout()
	if l.IsGroup() || len(l.Children) != 3 || l.Vertical || !l.Children[2].Vertical {
		t.Fatalf("layout: %s", js(l))
	}
	if got := strings.Join(c.Panels(), ","); got != "a,b,e,c,d" {
		t.Fatalf("panels %s in %s", got, js(l))
	}
	// Reorder inside a group, move across groups, close.
	c.Move("b", "a", w.DropCenter) // b before a
	if g := c.Layout().Children[0]; strings.Join(g.Tabs, ",") != "b,a" || g.Active != "b" {
		t.Fatalf("reorder: %s", js(c.Layout()))
	}
	c.MoveToGroup("e", "a") // e's group empties and disappears
	c.Close("d")            // the right split collapses to just c
	l = c.Layout()
	if len(l.Children) != 2 || strings.Join(l.Children[0].Tabs, ",") != "b,a,e" || strings.Join(l.Children[1].Tabs, ",") != "c" {
		t.Fatalf("after moves: %s", js(l))
	}
	// Save and restore.
	data, _ := json.Marshal(c)
	c2 := w.NewDockController(nil)
	if err := json.Unmarshal(data, c2); err != nil || js(c2.Layout()) != js(c.Layout()) {
		t.Fatalf("restore: %v\n%s\n%s", err, js(c2.Layout()), data)
	}
	// Closing everything leaves one empty group.
	for _, id := range c.Panels() {
		c.Close(id)
	}
	if l := c.Layout(); !l.IsGroup() || len(l.Tabs) != 0 {
		t.Fatalf("empty: %s", js(l))
	}
}

func js(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestDockUI(t *testing.T) {
	c := w.NewDockController(&w.DockNode{Tabs: []string{"one", "two"}, Active: "one"})
	counters := map[string]*int{}
	panel := func(id string) w.DockPanel {
		n := 0
		counters[id] = &n
		return w.DockPanel{ID: id, Title: strings.ToUpper(id), Closable: true, Build: func(ctx w.BuildContext) w.Widget {
			return counterPanel{id: id}
		}}
	}
	var closed []string
	tt := tester.New(m.App{Home: m.Dock{Controller: c, Panels: []w.DockPanel{panel("one"), panel("two"), panel("three")},
		OnClose: func(id string) { closed = append(closed, id) }}}, 800, 500)
	// Background tabs keep their state.
	tt.TapText("one: 0")
	tt.TapText("one: 1")
	tt.TapText("TWO")
	if _, ok := tt.Find("one: 2"); ok {
		t.Fatal("inactive tab is visible")
	}
	tt.TapText("ONE")
	if _, ok := tt.Find("one: 2"); !ok {
		t.Fatalf("tab state lost: %v", tt.Texts())
	}
	// Drag TWO onto the right edge of the content: a new group on the right.
	r, _ := tt.Find("TWO")
	tt.Pointer(render.PointerDown, r.X+5, r.Y+5)
	tt.Pointer(render.PointerMove, r.X+30, r.Y+5)
	tt.Pointer(render.PointerMove, 780, 250)
	tt.Pointer(render.PointerUp, 780, 250)
	l := c.Layout()
	if l.IsGroup() || len(l.Children) != 2 || strings.Join(l.Children[1].Tabs, ",") != "two" {
		t.Fatalf("drag to split: %s", js(l))
	}
	a, _ := tt.Find("ONE")
	b, _ := tt.Find("TWO")
	if !(b.X > a.X+200) {
		t.Fatalf("TWO not on the right: %v %v", a, b)
	}
	// Close a tab with its × button.
	c.Open("three", "two", w.DropCenter)
	tt.Pump()
	th, _ := tt.Find("THREE")
	tt.Tap(th.Right()+16, th.Y+th.H/2)
	if c.IsOpen("three") || strings.Join(closed, ",") != "three" {
		t.Fatalf("close: %v %s", closed, js(c.Layout()))
	}
}

// counterPanel is a panel with state: a button counting its taps.
type counterPanel struct{ id string }

func (counterPanel) CreateState() w.State { return &counterPanelState{} }

type counterPanelState struct {
	w.StateBase
	n int
}

func (s *counterPanelState) Build(w.BuildContext) w.Widget {
	id := w.WidgetOf[counterPanel](s).id
	return w.Align{Alignment: geom.TopLeft, Child: w.GestureDetector{OnTap: func() { s.SetState(func() { s.n++ }) },
		Child: w.Text{Text: fmt.Sprintf("%s: %d", id, s.n)}}}
}
