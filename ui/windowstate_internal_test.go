package ui

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/settings"
)

func TestWindowStateRoundTrip(t *testing.T) {
	store := settings.Memory()
	a := &App{config: DefaultConfig().WithWindowState(store, "window")}
	if _, ok := a.restoreWindowState(); ok {
		t.Fatal("nothing saved yet")
	}
	// The window was resized, then maximized: the normal size is saved.
	a.window = &nativeWindow{normalW: 1200, normalH: 700, width: 1920, height: 1080, maximized: true}
	a.saveWindowState()
	b := &App{config: DefaultConfig().WithWindowState(store, "window")}
	st, ok := b.restoreWindowState()
	if !ok || !st.Maximized || b.config.Width != 1200 || b.config.Height != 700 {
		t.Fatalf("restored %+v, config %dx%d", st, b.config.Width, b.config.Height)
	}
	// Nonsense sizes are ignored.
	settings.Set(store, "window", WindowState{Width: 5, Height: 5})
	c := &App{config: DefaultConfig().WithWindowState(store, "window")}
	c.restoreWindowState()
	if c.config.Width != DefaultConfig().Width {
		t.Fatalf("tiny size applied: %d", c.config.Width)
	}
}
