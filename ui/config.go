package ui

import (
	"image"
	"io/fs"
	"slices"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Config configures the application window.
type Config struct {
	Title      string
	Width      int // logical pixels
	Height     int
	Background geom.Color
	// Icon is the window icon at runtime (Linux/X11 taskbars and title
	// bars). macOS and Windows take the icon from the app bundle / .exe
	// instead; `nectar build` puts it there.
	Icon image.Image
	// CustomTitleBar lets the app draw the title bar: the content fills
	// the whole window, including the strip with the window buttons. See
	// WithCustomTitleBar.
	CustomTitleBar bool
	// Controls is which window buttons the window shows (zero = all). See
	// WithWindowControls.
	Controls widgets.WindowControls
	// NoAccessibility keeps the window's semantics (widgets.Semantics)
	// from the OS screen-reader interface (AT-SPI, NSAccessibility, UI
	// Automation), which is otherwise updated after frames that change it.
	NoAccessibility bool

	mounts      []resourceMount
	devMounts   []devMount
	windowState *windowStateConfig
}

type devMount struct {
	prefix, dir string
}

type resourceMount struct {
	prefix string
	fsys   fs.FS
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Title:      "Nectar UI",
		Width:      960,
		Height:     640,
		Background: geom.Hex(0xf6f4ef),
	}
}

func (c Config) WithTitle(t string) Config           { c.Title = t; return c }
func (c Config) WithSize(w, h int) Config            { c.Width, c.Height = w, h; return c }
func (c Config) WithBackground(bg geom.Color) Config { c.Background = bg; return c }
func (c Config) WithIcon(img image.Image) Config     { c.Icon = img; return c }

// WithCustomTitleBar makes the app's content fill the window's title-bar
// area, so it can put its own title, tabs or buttons next to the window
// buttons (widgets.TitleBar does the layout):
//
//   - macOS: the title bar becomes transparent and the traffic lights stay,
//     drawn by the system over the top-left of the content. The system
//     zone (top 28 px) still drags the window. The native title text is
//     hidden; Window.Title/SetTitle keep working for the app.
//   - Windows and Linux (X11): the window is frameless. The app draws
//     minimize / maximize / close (widgets.WindowButtons, added by TitleBar),
//     widgets.WindowDragArea marks what drags the window, and the outer
//     6 px resize it.
//   - Wayland: not supported by the windowing layer yet; the native title
//     bar stays and TitleBar is an ordinary bar below it.
//
// Widgets can check what they got with widgets.WindowOf(ctx).TitleBar().
func (c Config) WithCustomTitleBar(on bool) Config { c.CustomTitleBar = on; return c }

// WithWindowControls picks the window buttons: any mix of
// widgets.CloseControl, MinimizeControl and MaximizeControl, or
// widgets.NoControls for none (default: all three). Combined with
// WithCustomTitleBar this gives a completely custom window: hide what the
// design doesn't have and draw the rest yourself with Window.Close /
// Minimize / Maximize. Widgets can change it later with
// widgets.WindowOf(ctx).SetControls.
//
//   - macOS: the traffic lights are hidden one by one, with a native or a
//     custom title bar; the visible ones keep their place, and TitleBar's
//     left inset shrinks to what's still shown. Hiding only hides: Cmd+W,
//     Cmd+M and the Window menu keep working.
//   - Windows: with WithCustomTitleBar, widgets.TitleBar draws only the
//     chosen buttons. Without maximize, snapping and double-click don't
//     maximize either. With the native title bar, Windows hides
//     minimize and maximize when both are off (it greys one otherwise)
//     and can only grey close (which also blocks Alt+F4), unless all
//     three are off, which removes them all.
//   - Linux: with WithCustomTitleBar (X11) TitleBar draws only the chosen
//     buttons; the native title bars of X11 and Wayland keep theirs.
func (c Config) WithWindowControls(controls widgets.WindowControls) Config {
	c.Controls = controls
	return c
}

// WithResources mounts fsys under prefix ("" = the root) into the app's
// resources when the window opens (unmounted when it closes). Widgets read
// them with widgets.ResourcesOf(ctx) and widgets.AssetImage. Call it once
// per mount; later mounts win for the same name.
//
//	//go:embed assets
//	var files embed.FS
//	sub, _ := fs.Sub(files, "assets")
//	cfg := ui.DefaultConfig().WithResources("", sub).WithResources("charts", chartFiles)
func (c Config) WithResources(prefix string, fsys fs.FS) Config {
	c.mounts = append(slices.Clip(c.mounts), resourceMount{prefix, fsys})
	return c
}

// WithDevResources mounts the folder dir (relative to the working
// directory) under prefix, but only while the app runs under `nectar dev`
// (see package hotreload); otherwise it does nothing. The folder is read
// from disk and watched: edit an image or any other file there and the
// app shows the new version right away, without a restart. Pair it with
// the embedded copy of the same folder, which release builds use:
//
//	cfg.WithResources("", assets.FS).WithDevResources("", "assets")
//
// It's mounted after (so it wins over) every WithResources mount.
func (c Config) WithDevResources(prefix, dir string) Config {
	c.devMounts = append(slices.Clip(c.devMounts), devMount{prefix, dir})
	return c
}
