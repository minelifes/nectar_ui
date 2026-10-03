package ui

import (
	"errors"
	"slices"
	"sync"

	"github.com/gogpu/gpucontext"
	"github.com/minelifes/nectar_ui/internal/gogpu"

	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// OpenWindow opens another window showing root: a detached tool window, a
// second editor, an inspector. It gets its own widget tree (focus, overlay,
// animations; share state through your models) and the app's resources,
// clipboard and renderer. cfg sets its title, size and background (its
// resource mounts are ignored: windows share the app's).
//
// The window opens on the main thread a moment later; done (optional) then
// gets it, or an error, on the opener's UI goroutine. Closing the main
// window closes the others. Call it after Run has started.
func (a *App) OpenWindow(cfg Config, root widgets.Widget, done func(widgets.Window, error)) {
	if done == nil {
		done = func(widgets.Window, error) {}
	}
	if a.window == nil {
		done(nil, errors.New("ui: OpenWindow before Run"))
		return
	}
	a.window.do(func(g *gogpu.App) {
		w, err := a.newWindow(g, cfg, root)
		a.Post(func() {
			if err != nil {
				done(nil, err)
				return
			}
			done(w, nil)
		})
	})
}

// secondaryWindow is a window opened with OpenWindow. It implements
// widgets.Window.
type secondaryWindow struct {
	app      *App
	gw       *gogpu.Window
	cfg      Config
	owner    *widgets.BuildOwner
	pipeline *render.PipelineOwner
	root     *widgets.Root
	input    *inputQueue

	mu                    sync.Mutex
	width, height         int
	title                 string
	closed                bool
	fullscreen, maximized bool
}

// newWindow creates the platform window and its tree (main thread).
func (a *App) newWindow(g *gogpu.App, cfg Config, root widgets.Widget) (*secondaryWindow, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		cfg.Width, cfg.Height = 640, 480
	}
	gc := gogpu.DefaultConfig().WithTitle(cfg.Title).WithSize(cfg.Width, cfg.Height)
	gw, err := g.NewWindow(gc)
	if err != nil {
		return nil, err
	}
	sw := &secondaryWindow{app: a, gw: gw, cfg: cfg, width: cfg.Width, height: cfg.Height, title: cfg.Title}
	sw.owner = widgets.NewBuildOwner()
	sw.pipeline = render.NewPipelineOwner()
	sw.owner.OnScheduleFrame = g.RequestRedraw
	sw.pipeline.OnNeedVisualUpdate = g.RequestRedraw
	sw.owner.Window = sw
	sw.owner.Resources = a.resources
	sw.owner.Clipboard = appClipboard{a}
	sw.input = &inputQueue{dispatcher: render.NewPointerDispatcher()}
	sw.root = widgets.Mount(root, cfg.Background, sw.owner, sw.pipeline)

	push := func(e queuedEvent) {
		sw.input.enqueue(e)
		g.RequestRedraw()
	}
	gw.SetOnPointer(func(ev gpucontext.PointerEvent) {
		if e, ok := convertPointer(ev); ok {
			push(queuedEvent{pointer: &e})
		}
	})
	gw.SetOnScroll(func(ev gpucontext.ScrollEvent) {
		e := convertScroll(ev)
		push(queuedEvent{pointer: &e})
	})
	gw.SetOnKeyPress(func(k gpucontext.Key, m gpucontext.Modifiers) {
		e := widgets.KeyEvent{Key: convertKey(k), Mods: convertMods(m)}
		push(queuedEvent{key: &e})
	})
	gw.SetOnTextInput(func(s string) { push(queuedEvent{text: s}) })
	gw.SetOnResize(func(w, h int) {
		sw.mu.Lock()
		sw.width, sw.height = w, h
		sw.mu.Unlock()
		sw.refresh()
	})
	gw.SetOnClose(func() bool {
		a.Post(sw.dispose)
		return true
	})
	gw.SetOnDraw(sw.frame)

	a.winMu.Lock()
	a.windows = append(a.windows, sw)
	a.winMu.Unlock()
	g.RequestRedraw()
	return sw, nil
}

// frame runs the window's pipeline (render thread).
func (sw *secondaryWindow) frame(dc *gogpu.Context) {
	a := sw.app
	sw.mu.Lock()
	closed := sw.closed
	sw.mu.Unlock()
	if closed || !a.ensureRenderer() {
		return
	}
	fbW, fbH, scale, size, ok := frameSize(dc)
	if !ok {
		return
	}
	sw.input.process(sw.pipeline.Root(), sw.owner.Focus(), a.gpuApp.SetCursor)
	sw.owner.FlushBuild()
	sw.pipeline.FlushLayout(size)
	canvas := sw.pipeline.FlushPaint(size)
	a.draw(dc, fbW, fbH, scale, sw.cfg.Background, canvas)
	if sw.owner.HasActiveTickers() {
		a.gpuApp.RequestRedraw()
	}
}

// dispose unmounts the tree once the window is gone.
func (sw *secondaryWindow) dispose() {
	sw.mu.Lock()
	if sw.closed {
		sw.mu.Unlock()
		return
	}
	sw.closed = true
	sw.mu.Unlock()
	sw.root.Unmount()
	a := sw.app
	a.winMu.Lock()
	a.windows = slices.DeleteFunc(a.windows, func(x *secondaryWindow) bool { return x == sw })
	a.winMu.Unlock()
}

// closeWindows closes the secondary windows (the main one is closing).
func (a *App) closeWindows() {
	a.winMu.Lock()
	ws := slices.Clone(a.windows)
	a.winMu.Unlock()
	for _, sw := range ws {
		sw.dispose()
	}
}

// Windows returns the open secondary windows.
func (a *App) Windows() []widgets.Window {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	out := make([]widgets.Window, len(a.windows))
	for i, w := range a.windows {
		out[i] = w
	}
	return out
}

// --- widgets.Window ------------------------------------------------------

func (sw *secondaryWindow) Size() (int, int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.width, sw.height
}

func (sw *secondaryWindow) SetSize(w, h int) {
	sw.app.window.do(func(*gogpu.App) { sw.gw.SetSize(w, h) })
}

func (sw *secondaryWindow) SetMinSize(w, h int) {
	sw.app.window.do(func(*gogpu.App) { sw.gw.SetMinSize(w, h) })
}

func (sw *secondaryWindow) SetMaxSize(w, h int) {
	sw.app.window.do(func(*gogpu.App) { sw.gw.SetMaxSize(w, h) })
}

func (sw *secondaryWindow) Title() string {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.title
}

func (sw *secondaryWindow) SetTitle(t string) {
	sw.mu.Lock()
	sw.title = t
	sw.mu.Unlock()
	sw.app.window.do(func(*gogpu.App) { sw.gw.SetTitle(t) })
}

func (sw *secondaryWindow) IsFullscreen() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.fullscreen
}

func (sw *secondaryWindow) SetFullscreen(on bool) {
	sw.app.window.do(func(*gogpu.App) { sw.gw.SetFullscreen(on); sw.refresh() })
}

func (sw *secondaryWindow) IsMaximized() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.maximized
}

func (sw *secondaryWindow) Maximize() {
	sw.app.window.do(func(*gogpu.App) { sw.gw.Maximize(); sw.refresh() })
}

func (sw *secondaryWindow) Minimize() {
	sw.app.window.do(func(*gogpu.App) { sw.gw.Minimize() })
}

// refresh caches the window state (main thread).
func (sw *secondaryWindow) refresh() {
	fs, mx := sw.gw.IsFullscreen(), sw.gw.IsMaximized()
	sw.mu.Lock()
	sw.fullscreen, sw.maximized = fs, mx
	sw.mu.Unlock()
}

func (sw *secondaryWindow) Close() {
	sw.app.window.do(func(*gogpu.App) {
		sw.gw.Close()
		sw.app.Post(sw.dispose)
	})
}

func (sw *secondaryWindow) TitleBar() widgets.TitleBarInfo { return widgets.TitleBarInfo{} }

// --- Opening windows from widgets -------------------------------------------

// windowOpener lets widgets open windows (see widgets.OpenWindow).
func (a *App) openFromWidgets(opt widgets.WindowOptions, root widgets.Widget, done func(widgets.Window, error)) {
	cfg := a.config
	cfg.Title, cfg.Width, cfg.Height = opt.Title, opt.Width, opt.Height
	if opt.Background.A > 0 {
		cfg.Background = opt.Background
	}
	a.OpenWindow(cfg, root, done)
}
