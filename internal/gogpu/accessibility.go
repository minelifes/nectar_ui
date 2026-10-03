package gogpu

import "github.com/minelifes/nectar_ui/internal/gogpu/internal/platform"

// AccessibilityNode is one accessible element of a window: its role, name,
// value, state and bounds (logical pixels, relative to the window content)
// and its children.
//
// Roles: "button", "checkbox", "radio", "switch", "textfield", "password",
// "slider", "progressbar", "tab", "tablist", "menu", "menuitem", "header",
// "image", "link", "text", "list", "listitem", "treeitem"; anything else is
// a group.
type AccessibilityNode = platform.AXNode

// AccessibilityTree is the accessible content of a window. Nodes are keyed
// by ID; IDs must be stable across updates so screen readers keep their
// place. Focus names the focused node (0 for none).
type AccessibilityTree = platform.AXTree

// SetAccessibilityTree hands the window's accessible content to the
// platform's screen-reader interface: AT-SPI on Linux, NSAccessibility on
// macOS, UI Automation on Windows. Pass nil to withdraw it. action is
// called (on an arbitrary goroutine) with a node's ID when assistive
// technology activates it. The tree must not be modified after the call;
// build a new one for each update. Safe to call from any goroutine.
func (w *Window) SetAccessibilityTree(tree *AccessibilityTree, action func(nodeID uint64)) {
	if w == nil || w.platWindow == nil {
		return
	}
	platform.SetAccessibilityTree(w.platWindow, tree, action)
}

// SetAccessibilityTree sets the primary window's accessible content; see
// Window.SetAccessibilityTree.
func (a *App) SetAccessibilityTree(tree *AccessibilityTree, action func(nodeID uint64)) {
	if a.platWindow == nil {
		return
	}
	platform.SetAccessibilityTree(a.platWindow, tree, action)
}
