package ui

import (
	"os"
	"runtime"
	"sync"

	"github.com/gogpu/gpucontext"
	"github.com/minelifes/nectar_ui/internal/gogpu"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// titleBarKind is how a custom title bar is realized on this platform.
type titleBarKind uint8

const (
	titleBarNative    titleBarKind = iota // OS title bar (default / unsupported)
	titleBarMac                           // content under a transparent title bar, traffic lights kept
	titleBarFrameless                     // no chrome; the app draws buttons, the OS asks us what's where
)

// resizeBorder is how close to the edge of a frameless window a press
// resizes it (logical px).
const resizeBorder = 6

// macTrafficLights is the width the close / minimize / zoom buttons take
// at the left of a macOS title bar, with a gap after them.
const macTrafficLights = 76

func customTitleBarKind(goos string, wayland bool) titleBarKind {
	switch goos {
	case "darwin":
		return titleBarMac
	case "windows":
		return titleBarFrameless
	case "linux", "freebsd", "openbsd", "netbsd":
		// gogpu's Wayland backend doesn't route move/resize through the
		// hit test yet: a frameless window couldn't be moved there.
		if wayland {
			return titleBarNative
		}
		return titleBarFrameless
	}
	return titleBarNative
}

func platformTitleBar(c Config) titleBarKind {
	if !c.CustomTitleBar {
		return titleBarNative
	}
	return customTitleBarKind(runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "")
}

// applyTitleBar configures the gogpu window for the title-bar kind.
func applyTitleBar(cfg gogpu.Config, kind titleBarKind) gogpu.Config {
	switch kind {
	case titleBarMac:
		// FullSizeContentView + transparent title bar; the native title
		// text is kept empty (see nativeWindow.osTitle).
		cfg = cfg.WithHeaderAlignment(gogpu.HeaderAlignLeft).WithTitle("")
	case titleBarFrameless:
		cfg = cfg.WithFrameless(true)
	}
	return cfg
}

// titleBarInfo is what widgets.WindowOf(ctx).TitleBar() reports.
func titleBarInfo(kind titleBarKind, fullscreen bool) widgets.TitleBarInfo {
	switch kind {
	case titleBarMac:
		if fullscreen { // the title bar and its buttons slide away
			return widgets.TitleBarInfo{Custom: true, SystemButtons: true}
		}
		return widgets.TitleBarInfo{Custom: true, SystemButtons: true, Height: 28, Leading: macTrafficLights}
	case titleBarFrameless:
		return widgets.TitleBarInfo{Custom: true}
	}
	return widgets.TitleBarInfo{}
}

// hitTester answers the OS's "what is at this point?" for frameless
// windows (WM_NCHITTEST on Windows, button presses on X11). The OS asks on
// the main thread; the render tree belongs to the frame, which holds mu
// while it builds, lays out and paints. gogpu doesn't run the two at the
// same time, but if it ever does we answer with the previous result rather
// than wait.
type hitTester struct {
	mu     sync.Mutex // held by App.frame
	app    *App
	lastMu sync.Mutex
	last   gpucontext.HitTestResult
}

func (h *hitTester) hit(x, y float64) gpucontext.HitTestResult {
	if !h.mu.TryLock() {
		h.lastMu.Lock()
		defer h.lastMu.Unlock()
		return h.last
	}
	root := h.app.pipeline.Root()
	w, ht := h.app.window.Size()
	border := float32(resizeBorder)
	if h.app.window.IsMaximized() || h.app.window.IsFullscreen() {
		border = 0
	}
	res := toGPUHit(render.WindowHitAt(root, geom.Sz(float32(w), float32(ht)), geom.Pt(float32(x), float32(y)), border))
	h.mu.Unlock()
	h.lastMu.Lock()
	h.last = res
	h.lastMu.Unlock()
	return res
}

func toGPUHit(h render.WindowHit) gpucontext.HitTestResult {
	switch h {
	case render.WindowHitCaption:
		return gpucontext.HitTestCaption
	case render.WindowHitResizeN:
		return gpucontext.HitTestResizeN
	case render.WindowHitResizeS:
		return gpucontext.HitTestResizeS
	case render.WindowHitResizeW:
		return gpucontext.HitTestResizeW
	case render.WindowHitResizeE:
		return gpucontext.HitTestResizeE
	case render.WindowHitResizeNW:
		return gpucontext.HitTestResizeNW
	case render.WindowHitResizeNE:
		return gpucontext.HitTestResizeNE
	case render.WindowHitResizeSW:
		return gpucontext.HitTestResizeSW
	case render.WindowHitResizeSE:
		return gpucontext.HitTestResizeSE
	}
	return gpucontext.HitTestClient
}
