package widgets

// Window controls the native window the app runs in. Get it from any
// widget with WindowOf(ctx):
//
//	OnPressed: func() { widgets.WindowOf(ctx).SetSize(1280, 800) }
//
// Sizes are logical pixels of the content area (without the title bar).
// Setters only queue a request: the window changes on the platform's main
// thread before the next frame, and the new size then reaches the layout
// like a user resize. Getters return the state as of the last frame.
// Everything is safe to call from any goroutine.
type Window interface {
	// Size is the content size as of the last frame.
	Size() (width, height int)
	// SetSize requests a new content size. It's clamped to the min/max
	// size and ignored in fullscreen (Wayland may ignore it for tiled
	// windows).
	SetSize(width, height int)
	// SetMinSize / SetMaxSize limit user and SetSize resizes (0, 0 = no
	// limit).
	SetMinSize(width, height int)
	SetMaxSize(width, height int)

	Title() string
	SetTitle(title string)

	IsFullscreen() bool
	SetFullscreen(on bool)
	IsMaximized() bool
	// Maximize toggles between maximized and restored.
	Maximize()
	Minimize()
	// Close asks the window to close (like the close button).
	Close()
}

// WindowOf returns the window hosting ctx's tree. It never returns nil:
// without a window (a bare BuildOwner) it returns one that ignores every
// request.
func WindowOf(ctx BuildContext) Window {
	if o := ctx.Owner(); o != nil && o.Window != nil {
		return o.Window
	}
	return noWindow{}
}

type noWindow struct{}

func (noWindow) Size() (int, int)    { return 0, 0 }
func (noWindow) SetSize(int, int)    {}
func (noWindow) SetMinSize(int, int) {}
func (noWindow) SetMaxSize(int, int) {}
func (noWindow) Title() string       { return "" }
func (noWindow) SetTitle(string)     {}
func (noWindow) IsFullscreen() bool  { return false }
func (noWindow) SetFullscreen(bool)  {}
func (noWindow) IsMaximized() bool   { return false }
func (noWindow) Maximize()           {}
func (noWindow) Minimize()           {}
func (noWindow) Close()              {}
