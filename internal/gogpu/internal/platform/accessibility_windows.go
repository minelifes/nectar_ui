//go:build windows

package platform

// SetAccessibilityTree publishes the accessible content of a window to UI
// Automation (nil clears it). action runs when a screen reader invokes or
// toggles a node.
func SetAccessibilityTree(w PlatformWindow, tree *AXTree, action func(uint64)) {
	if ww, ok := w.(*win32Window); ok {
		setUIATree(ww, tree, action)
	}
}
