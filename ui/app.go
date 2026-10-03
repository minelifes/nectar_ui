// Package ui is the entry point: it opens a window (via gogpu), owns the
// widget/render trees and runs the frame pipeline:
//
//	posted callbacks → build (dirty elements) → layout → paint → GPU
package ui

import (
	"log/slog"

	"github.com/gogpu/gogpu"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/gpu"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/resources"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// App runs a widget tree in a native window.
type App struct {
	config Config
	gpuApp *gogpu.App

	buildOwner *widgets.BuildOwner
	pipeline   *render.PipelineOwner
	root       *widgets.Root
	renderer   *gpu.Renderer
	input      *inputQueue
	window     *nativeWindow
	resources  *resources.Set
	devStops   []func()
	imeEnabled bool
	imePos     [2]int
	hits       *hitTester // frameless title bars
}

// NewApp creates an application with the given config.
func NewApp(config Config) *App {
	return &App{config: config}
}

// Run mounts root and blocks until the window is closed.
func (a *App) Run(root widgets.Widget) error {
	cfg := gogpu.DefaultConfig().
		WithTitle(a.config.Title).
		WithSize(a.config.Width, a.config.Height)
	if a.config.Icon != nil {
		cfg = cfg.WithIcon(a.config.Icon)
	}
	kind := platformTitleBar(a.config)
	cfg = applyTitleBar(cfg, kind)
	a.gpuApp = gogpu.NewApp(cfg)
	a.hits = &hitTester{app: a}
	if kind == titleBarFrameless {
		a.gpuApp.SetHitTestCallback(a.hits.hit)
	}

	a.buildOwner = widgets.NewBuildOwner()
	a.pipeline = render.NewPipelineOwner()
	a.buildOwner.OnScheduleFrame = a.gpuApp.RequestRedraw
	a.pipeline.OnNeedVisualUpdate = a.gpuApp.RequestRedraw

	a.window = newNativeWindow(a.gpuApp, a.config)
	a.buildOwner.Window = a.window
	// The app's resources: its own mounts over the global ones.
	a.resources = resources.NewSet(resources.Global())
	for _, m := range a.config.mounts {
		a.resources.Mount(m.prefix, m.fsys)
	}
	a.buildOwner.Resources = a.resources
	a.setupDev()
	// Window requests from widgets are applied here, on the main thread.
	a.gpuApp.OnUpdate(func(float64) { a.window.apply() })

	a.root = widgets.Mount(root, a.config.Background, a.buildOwner, a.pipeline)

	a.setupInput()
	// Files dropped from the OS file manager (the position is in physical
	// pixels).
	a.gpuApp.OnDragDrop(func(paths []string, x, y float64) {
		scale := a.gpuApp.ScaleFactor()
		if scale <= 0 {
			scale = 1
		}
		pos := geom.Offset{X: float32(x / scale), Y: float32(y / scale)}
		a.buildOwner.Post(func() { a.buildOwner.DropFiles(paths, pos) })
	})
	a.gpuApp.OnDraw(a.frame)
	a.gpuApp.OnClose(a.close)
	return a.gpuApp.Run()
}

// Window controls the app's window (the same one widgets get from
// widgets.WindowOf). Valid once Run has started.
func (a *App) Window() widgets.Window {
	if a.window == nil {
		return nil
	}
	return a.window
}

// Resources returns the app's resources (the same Set widgets get from
// widgets.ResourcesOf). Valid once Run has started; mount more at any time.
func (a *App) Resources() *resources.Set { return a.resources }

// Post runs fn on the UI goroutine before the next frame. Safe to call from
// any goroutine.
func (a *App) Post(fn func()) { a.buildOwner.Post(fn) }

// RequestRedraw schedules a new frame.
func (a *App) RequestRedraw() {
	if a.gpuApp != nil {
		a.gpuApp.RequestRedraw()
	}
}

// frame is one iteration of the rendering pipeline.
func (a *App) frame(dc *gogpu.Context) {
	if a.renderer == nil {
		dp := a.gpuApp.DeviceProvider()
		if dp == nil {
			return
		}
		r, err := gpu.New(dp.Device(), dp.SurfaceFormat())
		if err != nil {
			slog.Error("nectar-ui: renderer init failed", "err", err)
			return
		}
		a.renderer = r
	}

	fbW, fbH := dc.FramebufferSize()
	if fbW <= 0 || fbH <= 0 {
		return
	}
	scale := float32(dc.ScaleFactor())
	if scale <= 0 {
		scale = 1
	}
	// Logical size derived from the framebuffer so both always agree.
	window := geom.Size{W: float32(fbW) / scale, H: float32(fbH) / scale}
	a.window.resized(int(window.W+0.5), int(window.H+0.5))

	// 0. input  1. build  2. layout  3. paint
	// (the OS hit test for frameless title bars reads the tree: keep it out)
	a.hits.mu.Lock()
	a.processInput()
	a.buildOwner.FlushBuild()
	a.pipeline.FlushLayout(window)
	canvas := a.pipeline.FlushPaint(window)
	a.hits.mu.Unlock()
	a.syncIME(scale)

	// 4. GPU
	view := dc.SurfaceView()
	enc := dc.CommandEncoder()
	if view == nil || enc == nil {
		return
	}
	err := a.renderer.Draw(gpu.Frame{
		Encoder: enc, Target: view,
		Width: uint32(fbW), Height: uint32(fbH), Scale: scale,
		Clear:    a.config.Background,
		Commands: canvas.Commands,
	})
	if err != nil {
		slog.Error("nectar-ui: draw failed", "err", err)
	}
	// Keep frames coming while something animates.
	if a.buildOwner.HasActiveTickers() {
		a.gpuApp.RequestRedraw()
	}
}

func (a *App) close() {
	a.stopDev()
	if a.root != nil {
		a.root.Unmount()
	}
	if a.resources != nil {
		a.resources.Reset()
	}
	if a.renderer != nil {
		a.renderer.Release()
		a.renderer = nil
	}
}
