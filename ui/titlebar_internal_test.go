package ui

import (
	"testing"

	"github.com/gogpu/gpucontext"

	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

func TestCustomTitleBarPerPlatform(t *testing.T) {
	cases := []struct {
		goos    string
		wayland bool
		want    titleBarKind
	}{
		{"darwin", false, titleBarMac},
		{"windows", false, titleBarFrameless},
		{"linux", false, titleBarFrameless},
		{"linux", true, titleBarNative}, // gogpu's Wayland can't move frameless windows yet
		{"freebsd", false, titleBarFrameless},
		{"js", false, titleBarNative},
	}
	for _, c := range cases {
		if got := customTitleBarKind(c.goos, c.wayland); got != c.want {
			t.Errorf("%s (wayland %v): %v, want %v", c.goos, c.wayland, got, c.want)
		}
	}
	if platformTitleBar(DefaultConfig()) != titleBarNative {
		t.Error("custom title bar without WithCustomTitleBar")
	}
	cfg := DefaultConfig().WithCustomTitleBar(true)
	if !cfg.CustomTitleBar {
		t.Error("WithCustomTitleBar didn't set it")
	}
}

func TestTitleBarInfoPerKind(t *testing.T) {
	mac := titleBarInfo(titleBarMac, false, 0, 0)
	if !mac.Custom || !mac.SystemButtons || mac.Leading != 76 || mac.Height != 28 {
		t.Errorf("mac: %+v", mac)
	}
	if fs := titleBarInfo(titleBarMac, true, 0, 0); fs.Leading != 0 || !fs.Custom {
		t.Errorf("mac fullscreen: %+v", fs)
	}
	if f := titleBarInfo(titleBarFrameless, false, 0, 0); !f.Custom || f.SystemButtons || f.Leading != 0 {
		t.Errorf("frameless: %+v", f)
	}
	if n := titleBarInfo(titleBarNative, false, 0, 0); n.Custom {
		t.Errorf("native: %+v", n)
	}
}

func TestMacInsetFollowsVisibleButtons(t *testing.T) {
	cases := []struct {
		controls widgets.WindowControls
		inset    float32 // from AppKit (0 = not known yet)
		want     float32
	}{
		{0, 0, 76},                    // all three, standard layout
		{widgets.CloseControl, 0, 36}, // only close
		{widgets.CloseControl | widgets.MinimizeControl, 0, 56},
		{widgets.MaximizeControl, 0, 76}, // hidden buttons keep their slot
		{widgets.NoControls, 0, 0},       // nothing to avoid
		{0, 66, 74},                      // AppKit's real edge + gap wins
		{widgets.NoControls, 66, 0},      // a stale edge doesn't bring it back
	}
	for _, c := range cases {
		got := titleBarInfo(titleBarMac, false, c.controls, c.inset)
		if got.Leading != c.want || got.Controls != c.controls {
			t.Errorf("%v (inset %v): leading %v, want %v", c.controls, c.inset, got.Leading, c.want)
		}
	}
}

func TestControlsReachGogpu(t *testing.T) {
	for _, c := range []struct {
		in                  widgets.WindowControls
		close, minim, maxim bool
	}{
		{0, true, true, true},
		{widgets.CloseControl, true, false, false},
		{widgets.MinimizeControl | widgets.MaximizeControl, false, true, true},
		{widgets.NoControls, false, false, false},
	} {
		cl, mi, mx := windowButtons(c.in)
		if cl != c.close || mi != c.minim || mx != c.maxim {
			t.Errorf("%v → %v %v %v", c.in, cl, mi, mx)
		}
	}
	if DefaultConfig().WithWindowControls(widgets.CloseControl).Controls != widgets.CloseControl {
		t.Error("WithWindowControls didn't set it")
	}
}

func TestHitResultsMapToGogpu(t *testing.T) {
	want := map[render.WindowHit]gpucontext.HitTestResult{
		render.WindowHitClient: gpucontext.HitTestClient, render.WindowHitCaption: gpucontext.HitTestCaption,
		render.WindowHitResizeN: gpucontext.HitTestResizeN, render.WindowHitResizeS: gpucontext.HitTestResizeS,
		render.WindowHitResizeW: gpucontext.HitTestResizeW, render.WindowHitResizeE: gpucontext.HitTestResizeE,
		render.WindowHitResizeNW: gpucontext.HitTestResizeNW, render.WindowHitResizeNE: gpucontext.HitTestResizeNE,
		render.WindowHitResizeSW: gpucontext.HitTestResizeSW, render.WindowHitResizeSE: gpucontext.HitTestResizeSE,
	}
	for h, g := range want {
		if got := toGPUHit(h); got != g {
			t.Errorf("%v → %v, want %v", h, got, g)
		}
	}
}
