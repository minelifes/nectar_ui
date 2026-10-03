//go:build !linux && !darwin && !windows

package platform

// SetAccessibilityTree is a no-op where no screen-reader bridge exists.
func SetAccessibilityTree(PlatformWindow, *AXTree, func(uint64)) {}
