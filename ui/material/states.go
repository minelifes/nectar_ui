package material

import "github.com/minelifes/nectar_ui/ui/geom"

// States is the set of interaction states a component is in (Flutter's
// WidgetState): hovered, focused, pressed, selected (checked, on), error
// and disabled. Components resolve their StateColor fields with it.
type States uint8

const (
	StateHovered States = 1 << iota
	StateFocused
	StatePressed
	StateSelected // checked checkbox, selected radio, switch on
	StateError
	StateDisabled
)

// Has reports whether every state in x is set.
func (s States) Has(x States) bool { return s&x == x }

// StateColor is a color that depends on the component's state (Flutter's
// WidgetStateColor / WidgetStateProperty<Color>). Each field is the color
// in that state; unset (zero) fields fall back to the component's M3
// default for its current state, so only the states you care about need a
// value:
//
//	BorderColors: material.StateColor{
//	    Normal:  geom.Hex(0xD0D0D0), // idle
//	    Hovered: geom.Hex(0x909090),
//	    Focused: geom.Hex(0x1E88E5),
//	    Error:   geom.Hex(0xE53935),
//	}
//
// When several states hold at once, the first set field for a state the
// component is in wins, in this order: Disabled; FocusedError (error while
// focused), then Error; Focused; Pressed; Hovered; Selected; Normal.
// Normal applies only at rest (not focused, in error or disabled; hovering
// and pressing fall back to it), so setting it alone keeps the default
// focus and error colors. Use m.Transparent for "no color": zero, and with
// it geom.Transparent, means unset. AllStates gives one color everywhere.
//
// Which states a component reports is documented on its theme field (a
// text field is never "selected"; a checkbox is never "focused" yet).
type StateColor struct {
	Normal       geom.Color
	Hovered      geom.Color
	Focused      geom.Color
	Pressed      geom.Color
	Selected     geom.Color
	Error        geom.Color
	FocusedError geom.Color
	Disabled     geom.Color
}

// AllStates returns a StateColor with c in every state.
func AllStates(c geom.Color) StateColor {
	return StateColor{c, c, c, c, c, c, c, c}
}

// Resolve returns the color for states, or def (the component's own color
// for those states) when no field set for them applies.
func (c StateColor) Resolve(s States, def geom.Color) geom.Color {
	set := func(v geom.Color) bool { return v != (geom.Color{}) }
	switch {
	case s.Has(StateDisabled) && set(c.Disabled):
		return c.Disabled
	case s.Has(StateDisabled):
		return def
	case s.Has(StateError|StateFocused) && set(c.FocusedError):
		return c.FocusedError
	case s.Has(StateError) && set(c.Error):
		return c.Error
	case s.Has(StateError):
		return def
	case s.Has(StateFocused) && set(c.Focused):
		return c.Focused
	case s.Has(StatePressed) && set(c.Pressed):
		return c.Pressed
	case s.Has(StateHovered) && set(c.Hovered):
		return c.Hovered
	case s.Has(StateSelected) && set(c.Selected):
		return c.Selected
	case s.Has(StateFocused):
		return def
	case set(c.Normal) && !s.Has(StateSelected):
		return c.Normal
	}
	return def
}

// IsZero reports whether no field is set.
func (c StateColor) IsZero() bool { return c == StateColor{} }
