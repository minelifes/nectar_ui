//go:build windows && !(amd64 || arm64)

package platform

// On 32-bit Windows the OLE drop target isn't used (POINTL is passed on the
// stack there); WM_DROPFILES reports drops without hover.
func registerDropTarget(*windowsPlatform, *win32Window) bool { return false }

func revokeDropTarget(*win32Window) {}
