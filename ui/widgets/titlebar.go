package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
)

// TitleBarInfo describes the window's title bar when the app draws its own
// (ui.Config.WithCustomTitleBar). Read it with WindowOf(ctx).TitleBar().
type TitleBarInfo struct {
	// Custom is true when the app's content fills the title-bar area and
	// the app draws the title bar. False on platforms that keep the native
	// one (Wayland), in tests by default, and without WithCustomTitleBar.
	Custom bool
	// SystemButtons is true when the OS still draws the window buttons
	// (macOS traffic lights). When false, add WindowButtons (TitleBar does).
	SystemButtons bool
	// Height is the height of the system title-bar zone the buttons sit in
	// (28 on macOS); 0 where the app picks any height.
	Height float32
	// Leading and Trailing are the widths the system buttons cover at the
	// left and right edges: keep content out of them. On macOS Leading
	// follows the visible traffic lights (0 with NoControls).
	Leading, Trailing float32
	// Controls is which window buttons the window has (Window.Controls):
	// the system draws them when SystemButtons, WindowButtons otherwise.
	Controls WindowControls
}

// WindowDragArea makes its child a handle for moving the window, like a
// native title bar: press and drag on it to move the window, double-click
// to maximize (where the OS does that). Buttons, text fields and other
// interactive widgets inside it still get their clicks; hover-only
// widgets (MouseRegion) don't block dragging.
//
// It only has an effect with ui.Config.WithCustomTitleBar on Windows and
// Linux (X11). On macOS the system title-bar zone drags the window by
// itself, and elsewhere the OS title bar does.
type WindowDragArea struct{ Child Widget }

func (w WindowDragArea) ChildWidget() Widget { return w.Child }
func (w WindowDragArea) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderWindowDragArea{}
}
func (w WindowDragArea) UpdateRenderObject(BuildContext, render.RenderObject) {}

// TitleBar is a custom window title bar: Child fills the space between
// the system window buttons (or WindowButtons it adds itself where the OS
// doesn't draw any), and the empty parts drag the window.
//
//	cfg := ui.DefaultConfig().WithCustomTitleBar(true)
//	...
//	widgets.Column{Children: []widgets.Widget{
//	    widgets.TitleBar{Color: bg, Child: widgets.Row{Cross: widgets.CrossCenter, Children: []widgets.Widget{
//	        widgets.Text{Text: "Aether"}, widgets.Spacer{}, settingsButton,
//	    }}},
//	    widgets.Expanded{Child: body},
//	}}
//
// Without a custom title bar (Wayland, or WithCustomTitleBar off) it's an
// ordinary bar below the native one: no insets and no window buttons.
type TitleBar struct {
	Child Widget
	// Center is drawn in the middle of the window's width, over Child
	// (a document name, a search field), whatever the system buttons take
	// on either side.
	Center Widget
	// Height of the bar; 0 = the system zone's height (28 on macOS) or 32.
	// On macOS the traffic lights stay centered in the top 28 px.
	Height float32
	// Color fills the bar (zero = transparent).
	Color geom.Color
	// Padding around Child, inside the system insets (default 8 left and
	// right).
	Padding *geom.EdgeInsets
	// Buttons styles the window buttons drawn on Windows and Linux.
	Buttons WindowButtons
}

func (t TitleBar) Build(ctx BuildContext) Widget {
	win := WatchWindow(ctx)
	info := win.TitleBar()
	h := t.Height
	if h <= 0 {
		h = info.Height
		if h <= 0 {
			h = 32
		}
	}
	pad := geom.InsetsLTRB(8, 0, 8, 0)
	if t.Padding != nil {
		pad = *t.Padding
	}
	pad.Left += info.Leading
	pad.Right += info.Trailing
	var content Widget = Padding{Padding: pad, Child: t.Child}
	kids := []Widget{Expanded{Child: content}}
	show := t.Buttons.Show
	if show == 0 {
		show = info.Controls
	}
	if info.Custom && !info.SystemButtons && show.Effective() != 0 {
		b := t.Buttons
		if b.Height <= 0 {
			b.Height = h
		}
		kids = append(kids, b)
	}
	var bar Widget = Row{Cross: CrossStretch, Children: kids}
	if t.Center != nil {
		bar = Stack{Expand: true, Children: []Widget{bar, Center{Child: t.Center}}}
	}
	if info.Custom {
		bar = WindowDragArea{Child: bar}
	}
	return SizedBox{Width: geom.Inf, Height: h, Child: DecoratedBox{Color: t.Color, Child: bar}}
}

// WindowButtons are minimize / maximize (restore) / close buttons for
// windows whose title bar the app draws on Windows and Linux (TitleBar adds
// them). They draw nothing where the OS shows its own buttons.
type WindowButtons struct {
	// Color of the glyphs (default mid-gray, readable on light and dark).
	Color geom.Color
	// Hover fills a hovered button (default Color at 12%).
	Hover geom.Color
	// Close fills the hovered close button (default Windows red), with
	// CloseColor for its glyph (default white).
	Close, CloseColor geom.Color
	// Width of each button (default 46) and height (default 32).
	Width, Height float32
	// Show picks the buttons (zero = the window's Controls).
	Show WindowControls
}

func (b WindowButtons) Build(ctx BuildContext) Widget {
	win := WatchWindow(ctx)
	info := win.TitleBar()
	if !info.Custom || info.SystemButtons {
		return SizedBox{}
	}
	if b.Color == (geom.Color{}) {
		b.Color = geom.Hex(0x808080)
	}
	if b.Hover == (geom.Color{}) {
		b.Hover = b.Color.WithAlpha(0.12)
	}
	if b.Close == (geom.Color{}) {
		b.Close = geom.Hex(0xC42B1C)
	}
	if b.CloseColor == (geom.Color{}) {
		b.CloseColor = geom.Hex(0xFFFFFF)
	}
	if b.Width <= 0 {
		b.Width = 46
	}
	if b.Height <= 0 {
		b.Height = 32
	}
	maxGlyph := glyphMaximize
	if win.IsMaximized() {
		maxGlyph = glyphRestore
	}
	show := b.Show
	if show == 0 {
		show = win.Controls()
	}
	var kids []Widget
	if show.Has(MinimizeControl) {
		kids = append(kids, captionButton{b: b, glyph: glyphMinimize, label: "minimize", onTap: win.Minimize})
	}
	if show.Has(MaximizeControl) {
		kids = append(kids, captionButton{b: b, glyph: maxGlyph, label: "maximize", onTap: win.Maximize})
	}
	if show.Has(CloseControl) {
		kids = append(kids, captionButton{b: b, glyph: glyphClose, label: "close", close: true, onTap: win.Close})
	}
	if len(kids) == 0 {
		return SizedBox{}
	}
	return Row{ShrinkMain: true, Children: kids}
}

// Caption glyphs (10×10, the thin Windows 11 style).
var (
	glyphMinimize = vector.NewIcon("window-minimize", 10, "M0 4.5h10v1H0z")
	glyphMaximize = vector.NewIcon("window-maximize", 10, "M0 0h10v10H0zM1 1v8h8V1z")
	glyphRestore  = vector.NewIcon("window-restore", 10,
		"M2 0h8v8H8v-1h1V1H3v1H2zM0 2h8v8H0zM1 3v6h6V3z")
	glyphClose = vector.NewIcon("window-close", 10,
		"M.7 0 5 4.3 9.3 0l.7.7L5.7 5l4.3 4.3-.7.7L5 5.7.7 10 0 9.3 4.3 5 0 .7z")
)

type captionButton struct {
	b     WindowButtons
	glyph *vector.Icon
	label string
	close bool
	onTap func()
}

func (captionButton) CreateState() State { return &captionButtonState{} }

type captionButtonState struct {
	StateBase
	hover bool
}

func (s *captionButtonState) Build(BuildContext) Widget {
	w := WidgetOf[captionButton](s)
	bg, fg := geom.Color{}, w.b.Color
	if s.hover {
		bg = w.b.Hover
		if w.close {
			bg, fg = w.b.Close, w.b.CloseColor
		}
	}
	return MouseRegion{
		OnEnter: func(PointerEvent) { s.SetState(func() { s.hover = true }) },
		OnExit:  func(PointerEvent) { s.SetState(func() { s.hover = false }) },
		Child: GestureDetector{OnTap: w.onTap, Child: SizedBox{Width: w.b.Width, Height: w.b.Height,
			Child: DecoratedBox{Color: bg, Child: Center{Child: Icon{Icon: w.glyph, Size: 10, Color: fg}}}}},
	}
}
