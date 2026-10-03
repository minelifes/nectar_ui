// Package axtypes holds the accessible-tree types shared by the platform
// layer and its screen-reader bridges.
package axtypes

// Node is one accessible element of a window, as the app describes it.
// Positions are logical pixels relative to the window's content.
type Node struct {
	ID          uint64
	Role        string // "button", "checkbox", "radio", "switch", "textfield", "slider", "tab", "menuitem", "header", "image", "link", "text", "group", ...
	Name        string
	Value       string
	Description string
	X, Y, W, H  float64
	Children    []uint64

	Checkable, Checked bool
	Selected, Disabled bool
	Focused            bool
	Actionable         bool // has a default action (press)
}

// Tree is the accessible content of one window.
type Tree struct {
	Title         string
	Width, Height float64
	Nodes         map[uint64]*Node
	Roots         []uint64
	Focus         uint64 // 0 = none
}
