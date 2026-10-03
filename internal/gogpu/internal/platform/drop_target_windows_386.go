//go:build windows && !(amd64 || arm64)

package platform

import "golang.org/x/sys/windows"

// On 32-bit Windows the OLE drop target isn't used (POINTL is passed on the
// stack there); WM_DROPFILES reports drops without hover.
func registerDropTarget(*windowsPlatform, *win32Window) bool { return false }

func revokeDropTarget(*win32Window) {}

// UI Automation providers need the 64-bit calling conventions.
func setUIATree(*win32Window, *AXTree, func(uint64)) {}

func uiaGetObject(windows.HWND, uintptr, uintptr) (uintptr, bool) { return 0, false }
