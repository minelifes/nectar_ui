package ui

import (
	"testing"

	"github.com/gogpu/gpucontext"

	"github.com/minelifes/nectar_ui/ui/render"
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
	mac := titleBarInfo(titleBarMac, false)
	if !mac.Custom || !mac.SystemButtons || mac.Leading < 70 || mac.Height != 28 {
		t.Errorf("mac: %+v", mac)
	}
	if fs := titleBarInfo(titleBarMac, true); fs.Leading != 0 || !fs.Custom {
		t.Errorf("mac fullscreen: %+v", fs)
	}
	if f := titleBarInfo(titleBarFrameless, false); !f.Custom || f.SystemButtons || f.Leading != 0 {
		t.Errorf("frameless: %+v", f)
	}
	if n := titleBarInfo(titleBarNative, false); n.Custom {
		t.Errorf("native: %+v", n)
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
