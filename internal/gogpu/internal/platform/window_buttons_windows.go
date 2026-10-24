//go:build windows

package platform

var procGetSystemMenu = user32.NewProc("GetSystemMenu")

const (
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	scClose       = 0xF060
)

// SetWindowButtons implements WindowButtonsSetter for Windows.
func (w *win32Window) SetWindowButtons(close, minimize, maximize bool) {
	w.callbackMu.RLock()
	frameless := w.frameless
	w.callbackMu.RUnlock()

	update := func(style uintptr) uintptr {
		style = setBit(style, wsMinimizeBox, minimize)
		style = setBit(style, wsMaximizeBox, maximize)
		if !frameless {
			// No button at all: without the system menu Windows draws none.
			style = setBit(style, wsSysMenu, close || minimize || maximize)
		}
		return style
	}
	if w.fullscreen {
		// The decorations come back from savedStyle on exit.
		w.savedStyle = uint32(update(uintptr(w.savedStyle)))
	} else {
		style, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), gwlStyle)
		procSetWindowLongPtrW.Call(uintptr(w.hwnd), gwlStyle, update(style))
		procSetWindowPos.Call(uintptr(w.hwnd), 0, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)
	}
	if !frameless {
		// Close can't be hidden on its own: grey it (also disables Alt+F4).
		menu, _, _ := procGetSystemMenu.Call(uintptr(w.hwnd), 0)
		if menu != 0 {
			flags := uintptr(mfByCommand | mfEnabled)
			if !close {
				flags = uintptr(mfByCommand | mfGrayed)
			}
			procEnableMenuItem.Call(menu, scClose, flags)
		}
	}
}

// WindowButtonsInset implements WindowButtonsSetter: Windows' buttons are at
// the trailing edge (or drawn by the app), never over leading content.
func (w *win32Window) WindowButtonsInset() float64 { return 0 }

func setBit(v, bit uintptr, on bool) uintptr {
	if on {
		return v | bit
	}
	return v &^ bit
}

var _ WindowButtonsSetter = (*win32Window)(nil)
