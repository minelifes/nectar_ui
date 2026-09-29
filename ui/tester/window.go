package tester

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
func (w *Window) SetFullscreen(on bool) { w.Fullscreen = on }
func (w *Window) IsMaximized() bool     { return w.Maximized }
func (w *Window) Maximize()             { w.Maximized = !w.Maximized }
func (w *Window) Minimize()             { w.Minimized = true }
func (w *Window) Close()                { w.Closed = true }
