package gogpu

import "github.com/minelifes/nectar_ui/internal/gogpu/internal/platform"

// SetWindowButtons shows or hides this window's native buttons: close,
// minimize and maximize (the green zoom / fullscreen button on macOS).
//
//   - macOS: each traffic light is hidden on its own; the visible ones keep
//     their place. The actions stay available from the keyboard and the
//     Window menu (Cmd+W, Cmd+M).
//   - Windows: minimize / maximize follow the window style, which also
//     controls snapping and double-click maximize; Windows hides both when
//     both are off. Close can only be greyed out (that disables Alt+F4)
//     unless all three are off, which removes them all. In frameless
//     windows only minimize / maximize behavior changes: the buttons are
//     the app's.
//   - Linux: the window manager draws the buttons; not supported.
func (w *Window) SetWindowButtons(close, minimize, maximize bool) {
	w.config = w.config.WithWindowButtons(close, minimize, maximize)
	if w.platWindow != nil {
		applyWindowButtons(w.platWindow, w.config)
	}
}

// WindowButtonsInset is how far (logical px) the visible native buttons
// reach in from the left edge of a window whose content extends under a
// transparent title bar (macOS); 0 elsewhere or when none is shown.
func (w *Window) WindowButtonsInset() float64 { return windowButtonsInset(w.platWindow) }

// SetWindowButtons is Window.SetWindowButtons for the primary window. Before
// Run it's stored and applied when the window is created.
func (a *App) SetWindowButtons(close, minimize, maximize bool) {
	a.config = a.config.WithWindowButtons(close, minimize, maximize)
	if a.primaryWindow != nil {
		a.primaryWindow.SetWindowButtons(close, minimize, maximize)
		return
	}
	if a.platWindow != nil {
		applyWindowButtons(a.platWindow, a.config)
	}
}

// WindowButtonsInset is Window.WindowButtonsInset for the primary window.
func (a *App) WindowButtonsInset() float64 { return windowButtonsInset(a.platWindow) }

func applyWindowButtons(pw platform.PlatformWindow, c Config) {
	bs, ok := pw.(platform.WindowButtonsSetter)
	if !ok {
		return
	}
	// Leave untouched windows alone: some platforms change more than the
	// buttons (Windows' system menu), only do it when asked.
	if !c.HideClose && !c.HideMinimize && !c.HideMaximize && !c.windowButtonsSet {
		return
	}
	bs.SetWindowButtons(!c.HideClose, !c.HideMinimize, !c.HideMaximize)
}

func windowButtonsInset(pw platform.PlatformWindow) float64 {
	if bs, ok := pw.(platform.WindowButtonsSetter); ok && pw != nil {
		return bs.WindowButtonsInset()
	}
	return 0
}
