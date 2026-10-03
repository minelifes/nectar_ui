package ui

import (
	"github.com/minelifes/nectar_ui/ui/settings"
)

// WindowState is what WithWindowState saves: the window's normal size and
// whether it was maximized or fullscreen.
type WindowState struct {
	Width, Height         int
	Maximized, Fullscreen bool
}

type windowStateConfig struct {
	store *settings.Store
	key   string
}

// WithWindowState makes the window open the way the user left it: its size
// and maximized / fullscreen state are read from store under key (e.g.
// "window") at startup and written back when the window closes.
//
//	store, _ := settings.Open("myapp")
//	cfg := ui.DefaultConfig().WithWindowState(store, "window")
func (c Config) WithWindowState(store *settings.Store, key string) Config {
	c.windowState = &windowStateConfig{store, key}
	return c
}

// restoreWindowState applies the saved size to the config (before the
// window opens) and returns the saved state.
func (a *App) restoreWindowState() (WindowState, bool) {
	ws := a.config.windowState
	if ws == nil || ws.store == nil || !ws.store.Has(ws.key) {
		return WindowState{}, false
	}
	st := settings.Get(ws.store, ws.key, WindowState{})
	if st.Width >= 100 && st.Height >= 100 {
		a.config.Width, a.config.Height = st.Width, st.Height
	}
	return st, true
}

// saveWindowState writes the window's state (on close).
func (a *App) saveWindowState() {
	ws := a.config.windowState
	if ws == nil || ws.store == nil || a.window == nil {
		return
	}
	n := a.window
	n.mu.Lock()
	st := WindowState{Width: n.normalW, Height: n.normalH, Maximized: n.maximized, Fullscreen: n.fullscreen}
	n.mu.Unlock()
	if st.Width <= 0 || st.Height <= 0 {
		st.Width, st.Height = a.config.Width, a.config.Height
	}
	_ = settings.Set(ws.store, ws.key, st)
}
