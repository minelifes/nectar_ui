package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minelifes/nectar_ui/internal/gogpu"
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// accessibility publishes a window's semantics tree (widgets.Semantics) to
// the OS screen-reader interface after frames that changed it.
type accessibility struct {
	owner   *widgets.BuildOwner
	publish func(*gogpu.AccessibilityTree, func(uint64))
	redraw  func()

	last     string // signature of the published tree
	at       time.Time
	deferred atomic.Bool
	mu       sync.Mutex
	actions  map[uint64]func()
}

// a11yAnimatingInterval limits updates while something animates.
const a11yAnimatingInterval = 250 * time.Millisecond

// update runs after a frame's paint, on the UI goroutine.
func (x *accessibility) update(root render.RenderObject, title string, size geom.Size) {
	if x == nil || x.publish == nil || root == nil {
		return
	}
	if x.owner.HasActiveTickers() && time.Since(x.at) < a11yAnimatingInterval {
		// Come back once the interval is over, in case the animation
		// ends before then and no frame follows.
		if x.redraw != nil && !x.deferred.Swap(true) {
			time.AfterFunc(a11yAnimatingInterval, func() { x.deferred.Store(false); x.redraw() })
		}
		return
	}
	nodes := widgets.SemanticsTree(root)
	focus := widgets.MarkFocus(nodes, x.owner.Focus())
	tree, actions, sig := accessibilityTree(nodes, title, size)
	if focus != nil {
		tree.Focus = focus.ID
	}
	sig += fmt.Sprintf("|%d", tree.Focus)
	if sig == x.last {
		return
	}
	x.last, x.at = sig, time.Now()
	x.mu.Lock()
	x.actions = actions
	x.mu.Unlock()
	x.publish(tree, x.act)
}

// act performs a node's action for assistive technology (any goroutine).
func (x *accessibility) act(id uint64) {
	x.owner.Post(func() {
		x.mu.Lock()
		fn := x.actions[id]
		x.mu.Unlock()
		if fn != nil {
			fn()
		}
	})
	if x.redraw != nil {
		x.redraw()
	}
}

// accessibilityTree converts a semantics forest; it returns the default
// actions by node and a signature that changes whenever the tree does.
func accessibilityTree(nodes []*widgets.SemanticsNode, title string, size geom.Size) (*gogpu.AccessibilityTree, map[uint64]func(), string) {
	t := &gogpu.AccessibilityTree{Title: title, Width: float64(size.W), Height: float64(size.H), Nodes: map[uint64]*gogpu.AccessibilityNode{}}
	actions := map[uint64]func(){}
	var sig strings.Builder
	fmt.Fprintf(&sig, "%q %v %v", title, size.W, size.H)
	var add func(ns []*widgets.SemanticsNode) []uint64
	add = func(ns []*widgets.SemanticsNode) []uint64 {
		ids := make([]uint64, 0, len(ns))
		for _, n := range ns {
			an := &gogpu.AccessibilityNode{
				ID: n.ID, Role: n.Role, Name: n.Label, Value: n.Value, Description: n.Hint,
				X: float64(n.Rect.X), Y: float64(n.Rect.Y), W: float64(n.Rect.W), H: float64(n.Rect.H),
				Checkable: n.Checkable, Checked: n.Checked, Selected: n.Selected, Disabled: n.Disabled,
				Focused: n.Focused, Actionable: n.OnTap != nil,
			}
			if n.OnTap != nil {
				actions[n.ID] = n.OnTap
			}
			t.Nodes[n.ID] = an
			fmt.Fprintf(&sig, "(%d %s %q %q %q %.1f %.1f %.1f %.1f %t%t%t%t%t%t", n.ID, n.Role, n.Label, n.Value, n.Hint,
				n.Rect.X, n.Rect.Y, n.Rect.W, n.Rect.H, n.Checkable, n.Checked, n.Selected, n.Disabled, n.Focused, n.OnTap != nil)
			an.Children = add(n.Children)
			sig.WriteByte(')')
			ids = append(ids, n.ID)
		}
		return ids
	}
	t.Roots = add(nodes)
	return t, actions, sig.String()
}
