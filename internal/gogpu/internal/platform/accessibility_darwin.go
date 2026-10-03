//go:build darwin

package platform

import "github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/darwin"

// SetAccessibilityTree publishes the accessible content of a window to
// VoiceOver (nil clears it). action runs, on the main thread, when
// VoiceOver presses a node.
func SetAccessibilityTree(w PlatformWindow, tree *AXTree, action func(uint64)) {
	dw, ok := w.(*darwinPlatformWindow)
	if !ok || dw.window == nil {
		return
	}
	darwin.SetWindowAccessibility(dw.window, tree, action)
}
