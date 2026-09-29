package ui

import (
	"sync"

	"github.com/gogpu/gogpu"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

// nativeWindow implements widgets.Window on top of gogpu. Widget code runs
// on the render thread, but window operations must run on the platform's
// main thread (AppKit requires it), so setters queue work that apply runs
// from gogpu's OnUpdate, which is on the main thread.
type nativeWindow struct {
	app      *gogpu.App
	titleBar titleBarKind

	mu            sync.Mutex
	pending       []func(*gogpu.App)
	width, height int
	title         string
	fullscreen    bool
	maximized     bool
	subs          map[int]func()
	nextSub       int
}

// Subscribe implements widgets.Listenable: fn runs after the window is
// maximized / restored or enters / leaves fullscreen (widgets.WatchWindow).
func (n *nativeWindow) Subscribe(fn func()) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subs == nil {
		n.subs = map[int]func(){}
	}
	id := n.nextSub
	n.nextSub++
	n.subs[id] = fn
	return func() {
		n.mu.Lock()
		delete(n.subs, id)
		n.mu.Unlock()
	}
}

func newNativeWindow(app *gogpu.App, c Config) *nativeWindow {
	return &nativeWindow{app: app, width: c.Width, height: c.Height, title: c.Title, titleBar: platformTitleBar(c)}
}

// osTitle is the title shown by the OS: empty when a custom macOS title bar
// hides it (the app draws its own).
func (n *nativeWindow) osTitle(t string) string {
	if n.titleBar == titleBarMac {
		return ""
	}
	return t
}

func (n *nativeWindow) TitleBar() widgets.TitleBarInfo {
	return titleBarInfo(n.titleBar, n.IsFullscreen())
}

// do queues op for the main thread and wakes the loop.
func (n *nativeWindow) do(op func(*gogpu.App)) {
	n.mu.Lock()
	n.pending = append(n.pending, op)
	n.mu.Unlock()
	n.app.RequestRedraw()
}

// apply runs queued operations and refreshes the cached state. Main thread
// only.
func (n *nativeWindow) apply() {
	n.mu.Lock()
	ops := n.pending
	n.pending = nil
	n.mu.Unlock()
	for _, op := range ops {
		op(n.app)
	}
	w, h := n.app.Size()
	fs, max := n.app.IsFullscreen(), n.app.IsMaximized()
	n.mu.Lock()
	if w > 0 && h > 0 {
		n.width, n.height = w, h
	}
	changed := n.fullscreen != fs || n.maximized != max
	n.fullscreen, n.maximized = fs, max
	var subs []func()
	if changed {
		for _, fn := range n.subs {
			subs = append(subs, fn)
		}
	}
	n.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// resized records the size seen by the frame (render thread).
func (n *nativeWindow) resized(w, h int) {
	n.mu.Lock()
	n.width, n.height = w, h
	n.mu.Unlock()
}

func (n *nativeWindow) Size() (int, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.width, n.height
}

func (n *nativeWindow) SetSize(w, h int) {
	n.do(func(a *gogpu.App) { a.RequestSize(w, h) })
}

func (n *nativeWindow) SetMinSize(w, h int) {
	n.do(func(a *gogpu.App) { a.SetMinSize(w, h) })
}

func (n *nativeWindow) SetMaxSize(w, h int) {
	n.do(func(a *gogpu.App) { a.SetMaxSize(w, h) })
}

func (n *nativeWindow) Title() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.title
}

func (n *nativeWindow) SetTitle(t string) {
	n.mu.Lock()
	n.title = t
	n.mu.Unlock()
	os := n.osTitle(t)
	n.do(func(a *gogpu.App) { a.SetTitle(os) })
}

func (n *nativeWindow) IsFullscreen() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fullscreen
}

func (n *nativeWindow) SetFullscreen(on bool) {
	n.do(func(a *gogpu.App) { a.SetFullscreen(on) })
}

func (n *nativeWindow) IsMaximized() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.maximized
}

func (n *nativeWindow) Maximize() { n.do(func(a *gogpu.App) { a.Maximize() }) }
func (n *nativeWindow) Minimize() { n.do(func(a *gogpu.App) { a.Minimize() }) }
func (n *nativeWindow) Close()    { n.do(func(a *gogpu.App) { a.Close() }) }
