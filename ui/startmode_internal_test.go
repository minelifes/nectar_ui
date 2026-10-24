package ui

import (
	"testing"

	"github.com/minelifes/nectar_ui/internal/gogpu"
)

func pendingOps(n *nativeWindow) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.pending)
}

// The saved window mode is applied once, after the first frame, and only
// the last one set (the `nectar dev` state over WithWindowState's): two
// fullscreen toggles queued back to back cancelled the macOS transition and
// left the window empty and frozen.
func TestStartModeAppliedOnceAfterFirstFrame(t *testing.T) {
	a := &App{window: &nativeWindow{app: &gogpu.App{}}}
	a.restoreMode(startMode{fullscreen: true}) // WithWindowState
	a.restoreMode(startMode{fullscreen: true}) // nectar dev, same mode
	if pendingOps(a.window) != 0 {
		t.Fatal("window switched before the first frame")
	}
	a.applyStartMode()
	if n := pendingOps(a.window); n != 1 {
		t.Fatalf("%d window requests after the first frame, want exactly one", n)
	}
	a.applyStartMode() // later frames
	if n := pendingOps(a.window); n != 1 {
		t.Fatalf("mode applied again on a later frame (%d requests)", n)
	}
}

func TestStartModeLastWins(t *testing.T) {
	a := &App{window: &nativeWindow{app: &gogpu.App{}}}
	a.restoreMode(startMode{fullscreen: true})
	a.restoreMode(startMode{w: 900, h: 600})
	if m := a.startMode; m == nil || m.fullscreen || m.w != 900 {
		t.Fatalf("pending mode %+v, want the later one", m)
	}
	a.applyStartMode()
	if n := pendingOps(a.window); n != 1 {
		t.Fatalf("%d requests, want one resize", n)
	}
}

func TestNoStartModeNoRequests(t *testing.T) {
	a := &App{window: &nativeWindow{app: &gogpu.App{}}}
	a.applyStartMode()
	if pendingOps(a.window) != 0 {
		t.Fatal("requests without a saved mode")
	}
}
