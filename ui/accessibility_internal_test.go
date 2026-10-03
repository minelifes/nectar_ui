package ui

import (
	"testing"

	"github.com/minelifes/nectar_ui/internal/gogpu"

	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func TestAccessibilityPublishing(t *testing.T) {
	wrap := false
	ctl := w.NewTextController("Ada")
	page := func() w.Widget {
		return m.App{Home: m.Scaffold{Body: w.Column{Cross: w.CrossStart, Children: []w.Widget{
			m.Checkbox{Value: wrap, SemanticLabel: "Wrap", OnChanged: func(v bool) { wrap = v }},
			w.SizedBox{Width: 300, Child: m.TextField{Label: "Name", Controller: ctl}},
		}}}}
	}
	tt := tester.New(page(), 400, 300)
	defer tt.Close()

	var trees []*gogpu.AccessibilityTree
	var action func(uint64)
	x := &accessibility{owner: tt.Build, publish: func(tree *gogpu.AccessibilityTree, act func(uint64)) {
		trees, action = append(trees, tree), act
	}}
	frame := func() {
		tt.Settle() // finish animations (updates are throttled while they run)
		x.update(tt.Pipeline.Root(), "Demo", tt.Size)
	}
	frame()
	frame()
	if len(trees) != 1 {
		t.Fatalf("%d publishes for one unchanged screen", len(trees))
	}
	tree := trees[0]
	if tree.Title != "Demo" || tree.Width != 400 || len(tree.Roots) == 0 {
		t.Fatalf("tree %+v", tree)
	}
	find := func(tr *gogpu.AccessibilityTree, role string) *gogpu.AccessibilityNode {
		for _, n := range tr.Nodes {
			if n.Role == role {
				return n
			}
		}
		t.Fatalf("no %s in %+v", role, tr.Nodes)
		return nil
	}
	box := find(tree, "checkbox")
	if box.Name != "Wrap" || box.Checked || !box.Checkable || !box.Actionable {
		t.Fatalf("checkbox %+v", box)
	}
	for _, id := range tree.Roots {
		if tree.Nodes[id] == nil {
			t.Fatalf("root %d missing", id)
		}
	}

	// The screen reader presses the box: the action runs on the UI
	// goroutine and the new state is published with the same node ID.
	action(box.ID)
	frame()
	if !wrap {
		t.Fatal("action not run")
	}
	tt.Root.SetApp(page(), tt.Clear)
	frame()
	if len(trees) < 2 {
		t.Fatal("change not published")
	}
	last := trees[len(trees)-1]
	if b := last.Nodes[box.ID]; b == nil || !b.Checked {
		t.Fatalf("checkbox after press %+v", last.Nodes[box.ID])
	}

	// Focusing the text field moves the tree's focus to it.
	field := find(last, "textfield")
	tt.Tap(float32(field.X+field.W/2), float32(field.Y+field.H/2))
	frame()
	last = trees[len(trees)-1]
	if last.Focus != field.ID || !last.Nodes[field.ID].Focused {
		t.Fatalf("focus %d, want %d", last.Focus, field.ID)
	}
}
