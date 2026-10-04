package widgets

import (
	"errors"

	"github.com/minelifes/nectar_ui/ui/geom"
)

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

	// TitleBar describes the title-bar area when the app draws its own
	// (ui.Config.WithCustomTitleBar); the zero value otherwise.
	TitleBar() TitleBarInfo

	// Controls is which window buttons the window shows (close, minimize,
	// maximize); SetControls changes them. Hidden buttons only go away
	// visually: Close, Minimize and Maximize keep working, so a custom
	// design can call them from its own widgets. See ui.Config.WithWindowControls
	// for what each platform does.
	Controls() WindowControls
	SetControls(c WindowControls)
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

// WatchWindow is WindowOf that also rebuilds ctx's element when the
// window's state changes (maximized, fullscreen, and with them the title
// bar insets). Call it from Build, like Listen.
func WatchWindow(ctx BuildContext) Window {
	win := WindowOf(ctx)
	if l, ok := win.(Listenable); ok {
		Listen(ctx, l)
	}
	return win
}

type noWindow struct{}

func (noWindow) Size() (int, int)           { return 0, 0 }
func (noWindow) SetSize(int, int)           {}
func (noWindow) SetMinSize(int, int)        {}
func (noWindow) SetMaxSize(int, int)        {}
func (noWindow) Title() string              { return "" }
func (noWindow) SetTitle(string)            {}
func (noWindow) IsFullscreen() bool         { return false }
func (noWindow) SetFullscreen(bool)         {}
func (noWindow) IsMaximized() bool          { return false }
func (noWindow) Maximize()                  {}
func (noWindow) Minimize()                  {}
func (noWindow) Close()                     {}
func (noWindow) TitleBar() TitleBarInfo     { return TitleBarInfo{} }
func (noWindow) Controls() WindowControls   { return AllControls }
func (noWindow) SetControls(WindowControls) {}

// WindowOptions describe a window opened with OpenWindow.
type WindowOptions struct {
	Title         string
	Width, Height int        // logical pixels; 0 = 640×480
	Background    geom.Color // zero = the app's
	// Controls is which window buttons it has (zero = all; e.g.
	// CloseControl for a tool window, NoControls for none).
	Controls WindowControls
}

// ErrNoWindows is reported where windows can't be opened (tests).
var ErrNoWindows = errors.New("widgets: can't open windows here")

// OpenWindow opens another native window showing root, with its own widget
// tree. done (optional) gets the new window on the UI goroutine.
func OpenWindow(ctx BuildContext, opt WindowOptions, root Widget, done func(Window, error)) {
	if done == nil {
		done = func(Window, error) {}
	}
	if o := ctx.Owner(); o.OpenWindow != nil {
		o.OpenWindow(opt, root, done)
		return
	}
	done(nil, ErrNoWindows)
}
