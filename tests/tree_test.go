package tests

import (
	"fmt"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// rowY is the center of visible row i (default 28px rows from the top).
func rowY(i int) float32 { return float32(i)*28 + 14 }

func visible(tt *tester.Tester, labels ...string) bool {
	for _, l := range labels {
		if _, ok := tt.Find(l); !ok {
			return false
		}
	}
	return true
}

func TestTreeTapAndKeys(t *testing.T) {
	var selected, activated []string
	var toggled []string
	roots := []w.TreeNode{
		{Label: "src", Children: []w.TreeNode{
			{Label: "main.go"},
			{Label: "ui", Children: []w.TreeNode{{Label: "app.go"}}},
		}},
		{Label: "README.md"},
	}
	tt := tester.New(w.TreeView{Roots: roots,
		OnSelect:   func(n w.TreeNode) { selected = append(selected, n.Label) },
		OnActivate: func(n w.TreeNode) { activated = append(activated, n.Label) },
		OnToggle:   func(n w.TreeNode, open bool) { toggled = append(toggled, fmt.Sprintf("%s %v", n.Label, open)) },
	}, 300, 400)

	if visible(tt, "main.go") || !visible(tt, "src", "README.md") {
		t.Fatalf("initial rows: %v", tt.Texts())
	}
	// Click expands a branch and selects it.
	tt.Tap(100, rowY(0))
	if !visible(tt, "main.go", "ui") || !slices.Equal(selected, []string{"src"}) {
		t.Fatalf("after tap: %v, selected %v", tt.Texts(), selected)
	}
	// README moved down to row 3.
	if r, _ := tt.Find("README.md"); r.Y < 3*28 || r.Y > 4*28 {
		t.Fatalf("README at %v", r)
	}
	// Keyboard: ↓ to main.go, ↓ to ui, → expands ui, → goes to app.go.
	tt.Key(w.KeyDown)
	tt.Key(w.KeyDown)
	tt.Key(w.KeyRight)
	if !visible(tt, "app.go") {
		t.Fatalf("→ didn't expand ui: %v", tt.Texts())
	}
	tt.Key(w.KeyRight)
	tt.Key(w.KeyEnter)
	if !slices.Equal(activated, []string{"app.go"}) {
		t.Fatalf("activated %v", activated)
	}
	// ← on a leaf goes to its parent, ← again collapses it.
	tt.Key(w.KeyLeft)
	tt.Key(w.KeyLeft)
	if visible(tt, "app.go") {
		t.Fatal("← didn't collapse ui")
	}
	want := []string{"src", "main.go", "ui", "app.go", "ui"}
	if !slices.Equal(selected, want) {
		t.Fatalf("selected %v, want %v", selected, want)
	}
	if !slices.Equal(toggled, []string{"src true", "ui true", "ui false"}) {
		t.Fatalf("toggled %v", toggled)
	}
	// Double-click activates (and doesn't toggle twice).
	tt.Advance(time.Second)
	tt.Tap(100, rowY(1)) // main.go
	tt.Tap(100, rowY(1))
	if activated[len(activated)-1] != "main.go" {
		t.Fatalf("double-click: %v", activated)
	}
	// Home, then Space collapses src.
	tt.Key(w.KeyHome)
	tt.Key(w.KeySpace)
	if visible(tt, "main.go") {
		t.Fatal("space didn't collapse")
	}
}

func TestTreeTapSelectsOnly(t *testing.T) {
	roots := []w.TreeNode{{Label: "docs", Children: []w.TreeNode{{Label: "a.md"}}}}
	tt := tester.New(w.TreeView{Roots: roots, TapSelectsOnly: true}, 300, 200)
	tt.Tap(150, rowY(0))
	if visible(tt, "a.md") {
		t.Fatal("tap on the label shouldn't expand")
	}
	tt.Tap(4+10, rowY(0)) // the arrow
	if !visible(tt, "a.md") {
		t.Fatal("arrow didn't expand")
	}
	tt.Advance(time.Second)
	tt.Tap(150, rowY(0))
	tt.Tap(150, rowY(0)) // double-click toggles
	if visible(tt, "a.md") {
		t.Fatal("double-click didn't collapse")
	}
}

func TestTreeLazyLoadAndController(t *testing.T) {
	loads := 0
	ctrl := w.NewTreeController()
	roots := []w.TreeNode{{Key: "big", Label: "big", Load: func() []w.TreeNode {
		loads++
		var kids []w.TreeNode
		for i := range 1000 {
			kids = append(kids, w.TreeNode{Label: fmt.Sprintf("item %d", i)})
		}
		return kids
	}}}
	tt := tester.New(w.TreeView{Roots: roots, Controller: ctrl}, 300, 280)
	if loads != 0 {
		t.Fatal("loaded before expanding")
	}
	ctrl.Expand("big")
	tt.Pump()
	if loads != 1 || !visible(tt, "item 0") {
		t.Fatalf("loads %d, rows %v", loads, tt.Texts())
	}
	// Only the visible rows (plus slack) are built.
	if n := len(tt.Texts()); n > 60 {
		t.Fatalf("built %d rows", n)
	}
	// Collapse / expand again uses the cache; Reload loads again.
	ctrl.Collapse("big")
	tt.Pump()
	ctrl.Expand("big")
	tt.Pump()
	if loads != 1 {
		t.Fatalf("reloaded without Reload: %d", loads)
	}
	ctrl.Reload("big")
	tt.Pump()
	if loads != 2 {
		t.Fatalf("Reload: %d", loads)
	}
	// End scrolls the last row into view. (Clicking a branch toggles it,
	// so click twice, far apart, to focus it and leave it expanded.)
	tt.Tap(100, rowY(0))
	tt.Advance(time.Second)
	tt.Tap(100, rowY(0))
	tt.Key(w.KeyEnd)
	tt.Pump()
	if !visible(tt, "item 999") || ctrl.Selected() != "big/item 999" {
		t.Fatalf("End: selected %q, rows %v", ctrl.Selected(), tt.Texts())
	}
	tt.Key(w.KeyHome)
	tt.Pump()
	if !visible(tt, "big") {
		t.Fatal("Home didn't scroll back")
	}
}

func TestFileTree(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":        {Data: []byte("hi")},
		"go.mod":           {Data: []byte("module x")},
		".git/config":      {Data: []byte("")},
		"src/main.go":      {Data: []byte("")},
		"src/ui/app.go":    {Data: []byte("")},
		"assets/logo.png":  {Data: []byte("")},
		"Zeta/empty/.keep": {Data: []byte("")},
		"build/out.bin":    {Data: []byte("")},
	}
	var opened, sel string
	var selEntry fs.DirEntry
	ctrl := w.NewTreeController()
	tt := run(func(w.BuildContext, func(func())) w.Widget {
		return w.SizedBox{Width: 300, Height: 500, Child: m.FileTree{FS: fsys, Controller: ctrl,
			Filter:   func(p string, d fs.DirEntry) bool { return p != "build" },
			OnOpen:   func(p string) { opened = p },
			OnSelect: func(p string, d fs.DirEntry) { sel, selEntry = p, d },
		}}
	})
	// Folders first (case-insensitive), then files; hidden and filtered out.
	got := tt.Texts()
	want := []string{"assets", "src", "Zeta", "go.mod", "README.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	// Expand by path.
	ctrl.Expand("src")
	ctrl.Expand("src/ui")
	tt.Pump()
	if !visible(tt, "main.go", "app.go") {
		t.Fatalf("expand by path: %v", tt.Texts())
	}
	// Double-click a file opens it.
	r, _ := tt.Find("app.go")
	tt.Tap(r.X+5, r.Y+r.H/2)
	tt.Tap(r.X+5, r.Y+r.H/2)
	if opened != "src/ui/app.go" || sel != "src/ui/app.go" || selEntry == nil || selEntry.IsDir() {
		t.Fatalf("opened %q, selected %q", opened, sel)
	}
	// Enter on a folder doesn't "open" it.
	tt.Key(w.KeyUp)
	tt.Key(w.KeyEnter)
	if opened != "src/ui/app.go" || visible(tt, "app.go") {
		t.Fatalf("enter on folder: opened %q rows %v", opened, tt.Texts())
	}
}

func TestFileTreeShowRoot(t *testing.T) {
	fsys := fstest.MapFS{"proj/a.txt": {Data: []byte("")}, "proj/lib/b.go": {Data: []byte("")}}
	tt := run(func(w.BuildContext, func(func())) w.Widget {
		return m.FileTree{FS: fsys, Root: "proj", ShowRoot: true, ShrinkWrap: true, ShowGuides: true}
	})
	if got := tt.Texts(); !slices.Equal(got, []string{"proj", "lib", "a.txt"}) {
		t.Fatalf("rows %v", got)
	}
}
