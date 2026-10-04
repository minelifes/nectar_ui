package tester

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Window is the tester's stand-in for the native window (what
// widgets.WindowOf returns in tests). SetSize resizes the test surface
// (Tester.Size) for the next Pump, clamped like a real window; the other
// calls just record state for assertions.
type Window struct {
	t *Tester

	title                  string
	MinW, MinH, MaxW, MaxH int
	Fullscreen, Maximized  bool
	Minimized, Closed      bool
	// Resizes counts SetSize calls that changed the size.
	Resizes int

	// TitleBarInfo is what TitleBar returns; set it to test custom title
	// bars (see MacTitleBar / FramelessTitleBar), then check presses with
	// HitTest.
	TitleBarInfo widgets.TitleBarInfo

	// OpenResult / SaveResult are what the next file dialogs "choose"
	// (nil / "" = cancelled); Dialogs records the options of each dialog.
	OpenResult []string
	SaveResult string
	Dialogs    []widgets.FileDialogOptions

	// ControlSet is what SetControls was given (zero = all buttons).
	ControlSet widgets.WindowControls

	subs    map[int]func()
	nextSub int
}

func (w *Window) Controls() widgets.WindowControls { return w.ControlSet.Effective() }

// SetControls records the buttons and rebuilds WatchWindow users (the title
// bar), like the real window.
func (w *Window) SetControls(c widgets.WindowControls) {
	if w.ControlSet.Effective() == c.Effective() {
		w.ControlSet = c
		return
	}
	w.ControlSet = c
	w.changed()
}

// Subscribe implements widgets.Listenable (widgets.WatchWindow): fn runs
// when Maximize or SetFullscreen change the state.
func (w *Window) Subscribe(fn func()) func() {
	if w.subs == nil {
		w.subs = map[int]func(){}
	}
	id := w.nextSub
	w.nextSub++
	w.subs[id] = fn
	return func() { delete(w.subs, id) }
}

func (w *Window) changed() {
	for _, fn := range w.subs {
		fn()
	}
}

func (w *Window) Size() (int, int) {
	return int(w.t.Size.W + 0.5), int(w.t.Size.H + 0.5)
}

func (w *Window) SetSize(width, height int) {
	if width <= 0 || height <= 0 || w.Fullscreen {
		return
	}
	width, height = clamp(width, w.MinW, w.MaxW), clamp(height, w.MinH, w.MaxH)
	if cw, ch := w.Size(); cw == width && ch == height {
		return
	}
	w.t.Size.W, w.t.Size.H = float32(width), float32(height)
	w.Resizes++
}

func clamp(v, lo, hi int) int {
	if lo > 0 && v < lo {
		v = lo
	}
	if hi > 0 && v > hi {
		v = hi
	}
	return v
}

func (w *Window) SetMinSize(width, height int) {
	w.MinW, w.MinH = width, height
	cw, ch := w.Size()
	w.SetSize(cw, ch) // re-clamp the current size
}

func (w *Window) SetMaxSize(width, height int) {
	w.MaxW, w.MaxH = width, height
	cw, ch := w.Size()
	w.SetSize(cw, ch)
}

func (w *Window) Title() string         { return w.title }
func (w *Window) SetTitle(title string) { w.title = title }
func (w *Window) IsFullscreen() bool    { return w.Fullscreen }
func (w *Window) SetFullscreen(on bool) {
	if w.Fullscreen != on {
		w.Fullscreen = on
		w.changed()
	}
}
func (w *Window) IsMaximized() bool { return w.Maximized }
func (w *Window) Maximize()         { w.Maximized = !w.Maximized; w.changed() }
func (w *Window) Minimize()         { w.Minimized = true }
func (w *Window) Close()            { w.Closed = true }

// TitleBar returns TitleBarInfo with the window's Controls; on a macOS-style
// bar (SystemButtons with a Leading inset) the inset follows the visible
// traffic lights like on a Mac.
func (w *Window) TitleBar() widgets.TitleBarInfo {
	info := w.TitleBarInfo
	info.Controls = w.ControlSet
	if info.SystemButtons && info.Leading > 0 {
		info.Leading = widgets.MacButtonsInset(w.ControlSet)
	}
	return info
}

// Title bars of the platforms, for TitleBarInfo.
var (
	// MacTitleBar: content under a transparent title bar, traffic lights
	// drawn by the OS.
	MacTitleBar = widgets.TitleBarInfo{Custom: true, SystemButtons: true, Height: 28, Leading: 76}
	// FramelessTitleBar: Windows / X11, the app draws the window buttons.
	FramelessTitleBar = widgets.TitleBarInfo{Custom: true}
)

// HitTest reports what the OS would do with a press at (x, y) — drag the
// window ("caption"), resize it, or pass it to the app ("client") — the way
// the real window answers the OS hit test with a custom title bar.
func (w *Window) HitTest(x, y float32) render.WindowHit {
	border := float32(0)
	if w.TitleBarInfo.Custom && !w.TitleBarInfo.SystemButtons && !w.Maximized && !w.Fullscreen {
		border = ResizeBorder
	}
	return render.WindowHitAt(w.t.Pipeline.Root(), w.t.Size, geom.Pt(x, y), border)
}

// ResizeBorder is how close to the edge of a frameless window a press
// resizes it (logical px).
const ResizeBorder float32 = 6

// OpenFileDialog implements widgets.FileDialogs with OpenResult.
func (w *Window) OpenFileDialog(opt widgets.FileDialogOptions, done func([]string, error)) {
	w.Dialogs = append(w.Dialogs, opt)
	res := w.OpenResult
	w.t.Build.Post(func() { done(res, nil) })
}

// SaveFileDialog implements widgets.FileDialogs with SaveResult.
func (w *Window) SaveFileDialog(opt widgets.FileDialogOptions, done func(string, error)) {
	w.Dialogs = append(w.Dialogs, opt)
	res := w.SaveResult
	w.t.Build.Post(func() { done(res, nil) })
}
