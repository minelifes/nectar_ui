package ui

import (
	"image"
	"io/fs"
	"slices"

	"github.com/minelifes/nectar_ui/ui/geom"
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

	mounts []resourceMount
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
