package widgets

// WindowControls selects a window's buttons: close, minimize and maximize
// (on macOS the green zoom / fullscreen button). The zero value means all
// three; NoControls means none, for windows whose design has no standard
// buttons at all (the app then calls Window.Close / Minimize / Maximize
// from its own widgets).
//
//	ui.DefaultConfig().WithWindowControls(widgets.CloseControl)  // only close
//	ui.DefaultConfig().WithWindowControls(widgets.NoControls)    // none
//	widgets.WindowOf(ctx).SetControls(widgets.CloseControl | widgets.MinimizeControl)
type WindowControls uint8

const (
	CloseControl WindowControls = 1 << iota
	MinimizeControl
	MaximizeControl

	// AllControls is every button (the default).
	AllControls = CloseControl | MinimizeControl | MaximizeControl
	// NoControls explicitly hides every button (the zero value means all).
	NoControls WindowControls = 1 << 7
)

// Effective resolves the zero value (all) and NoControls (none) to the set
// of buttons shown.
func (c WindowControls) Effective() WindowControls {
	if c == 0 {
		return AllControls
	}
	return c & AllControls
}

// Has reports whether every button in x is shown.
func (c WindowControls) Has(x WindowControls) bool {
	x &= AllControls
	return c.Effective()&x == x
}

func (c WindowControls) String() string {
	e := c.Effective()
	if e == 0 {
		return "none"
	}
	s := ""
	for _, b := range []struct {
		c    WindowControls
		name string
	}{{CloseControl, "close"}, {MinimizeControl, "minimize"}, {MaximizeControl, "maximize"}} {
		if e&b.c != 0 {
			if s != "" {
				s += "+"
			}
			s += b.name
		}
	}
	return s
}

// MacButtonsInset is how far (logical px) the visible traffic lights reach
// in from the left edge of a window whose content goes under the title bar,
// gap included: 76 with all three, 36 with close only, 0 with none. Hidden
// buttons keep their slot, so only the rightmost visible one counts. The
// app reads the real value from AppKit after the first frame; this is the
// standard layout, used until then and in tests.
func MacButtonsInset(c WindowControls) float32 {
	e := c.Effective()
	switch {
	case e&MaximizeControl != 0:
		return 76
	case e&MinimizeControl != 0:
		return 56
	case e&CloseControl != 0:
		return 36
	}
	return 0
}
