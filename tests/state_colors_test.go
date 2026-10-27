package tests

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

var (
	cIdle    = geom.Hex(0x101010)
	cHover   = geom.Hex(0x202020)
	cFocus   = geom.Hex(0x303030)
	cPress   = geom.Hex(0x404040)
	cSel     = geom.Hex(0x505050)
	cErr     = geom.Hex(0x606060)
	cFocErr  = geom.Hex(0x707070)
	cDis     = geom.Hex(0x808080)
	cDefault = geom.Hex(0x909090)
)

func TestStateColorResolveOrder(t *testing.T) {
	all := m.StateColor{Normal: cIdle, Hovered: cHover, Focused: cFocus, Pressed: cPress,
		Selected: cSel, Error: cErr, FocusedError: cFocErr, Disabled: cDis}
	cases := []struct {
		name string
		c    m.StateColor
		s    m.States
		want geom.Color
	}{
		{"idle", all, 0, cIdle},
		{"hovered", all, m.StateHovered, cHover},
		{"focused beats hovered", all, m.StateFocused | m.StateHovered, cFocus},
		{"pressed", all, m.StatePressed | m.StateHovered, cPress},
		{"selected", all, m.StateSelected, cSel},
		{"hovered beats selected", all, m.StateSelected | m.StateHovered, cHover},
		{"error", all, m.StateError | m.StateHovered, cErr},
		{"focused error", all, m.StateError | m.StateFocused, cFocErr},
		{"disabled beats all", all, m.StateDisabled | m.StateError | m.StateFocused, cDis},
		// Unset fields fall back to the next matching state, then the default.
		{"hover falls back to Normal", m.StateColor{Normal: cIdle}, m.StateHovered, cIdle},
		{"focus keeps the default", m.StateColor{Normal: cIdle}, m.StateFocused, cDefault},
		{"error keeps the default", m.StateColor{Normal: cIdle}, m.StateError, cDefault},
		{"disabled keeps the default", m.StateColor{Normal: cIdle}, m.StateDisabled, cDefault},
		{"selected keeps the default", m.StateColor{Normal: cIdle}, m.StateSelected, cDefault},
		{"focused error falls back to Error", m.StateColor{Error: cErr}, m.StateError | m.StateFocused, cErr},
		{"focused selected falls back to Selected", m.StateColor{Selected: cSel}, m.StateSelected | m.StateFocused, cSel},
		{"empty", m.StateColor{}, m.StateHovered, cDefault},
		{"AllStates", m.AllStates(cSel), m.StateDisabled | m.StateError, cSel},
	}
	for _, c := range cases {
		if got := c.c.Resolve(c.s, cDefault); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// strokesIn returns the outlines drawn in color c.
func strokesIn(cv *render.Canvas, c geom.Color) []render.Command {
	var out []render.Command
	for _, cmd := range cv.Commands {
		if cmd.Kind == render.CmdStroke && cmd.Color.A > 0 && sameRGB(cmd.Color, c) {
			out = append(out, cmd)
		}
	}
	return out
}

func inputField(in m.InputDecorationTheme, tf m.TextField) *tester.Tester {
	th := m.NewTheme(m.BaselineSeed, false)
	th.Input = in
	return tester.New(m.App{Theme: th, Home: w.Align{Alignment: geom.TopLeft,
		Child: w.SizedBox{Width: 480, Child: tf}}}, 600, 200)
}

// An outlined field fills when the theme gives it a fill color.
func TestOutlinedFieldFills(t *testing.T) {
	tt := inputField(m.InputDecorationTheme{FillColor: tGreen}, m.TextField{Outlined: true, Hint: "h"})
	if r := rectsIn(tt.Pump(), tGreen); len(r) == 0 || r[0].Rect.H != 56 {
		t.Fatalf("outlined field fill: %v", r)
	}
	// And by state.
	tt = inputField(m.InputDecorationTheme{FillColors: m.StateColor{Focused: tBlue}}, m.TextField{Outlined: true, Hint: "h"})
	if len(rectsIn(tt.Pump(), tBlue)) != 0 {
		t.Fatal("focused fill drawn while idle")
	}
	tt.Tap(240, 28)
	tt.Settle()
	if len(rectsIn(tt.Pump(), tBlue)) == 0 {
		t.Fatal("no fill while focused")
	}
	// Still no fill by default.
	tt = inputField(m.InputDecorationTheme{}, m.TextField{Outlined: true, Hint: "h"})
	for _, c := range tt.Pump().Commands {
		if c.Kind == render.CmdRect && c.Rect.W == 480 && c.Rect.H == 56 {
			t.Fatalf("default outlined field is filled: %+v", c)
		}
	}
}

func TestNoBorder(t *testing.T) {
	count := func(tt *tester.Tester) int {
		n := 0
		for _, c := range tt.Pump().Commands {
			if c.Kind == render.CmdStroke && c.Rect.W == 480 {
				n++
			}
		}
		return n
	}
	// Width 0: no border in any state.
	tt := inputField(m.InputDecorationTheme{BorderWidth: m.Dp(0), FocusedBorderWidth: m.Dp(0)}, m.TextField{Outlined: true, Hint: "h"})
	if n := count(tt); n != 0 {
		t.Fatalf("%d outlines with width 0", n)
	}
	tt.Tap(240, 28)
	tt.Settle()
	if n := count(tt); n != 0 {
		t.Fatalf("%d outlines with focused width 0", n)
	}
	// m.Transparent (not geom.Transparent, which is the zero "unset" color).
	tt = inputField(m.InputDecorationTheme{BorderColor: m.Transparent}, m.TextField{Outlined: true, Hint: "h"})
	if n := count(tt); n != 0 {
		t.Fatalf("%d outlines with a transparent border", n)
	}
	// A thicker border.
	tt = inputField(m.InputDecorationTheme{BorderWidth: m.Dp(3)}, m.TextField{Outlined: true, Hint: "h"})
	for _, c := range tt.Pump().Commands {
		if c.Kind == render.CmdStroke && c.Rect.W == 480 && c.Width != 3 {
			t.Fatalf("border width %v, want 3", c.Width)
		}
	}
}

func TestTextFieldStateColors(t *testing.T) {
	in := m.InputDecorationTheme{
		BorderColors: m.StateColor{Normal: cIdle, Hovered: cHover, Focused: cFocus, Error: cErr, FocusedError: cFocErr, Disabled: cDis},
		LabelColors:  m.StateColor{Normal: tRed, Focused: tGreen},
	}
	tt := inputField(in, m.TextField{Outlined: true, Label: "Name"})
	if len(strokesIn(tt.Pump(), cIdle)) != 1 {
		t.Fatal("idle border")
	}
	tt.Hover(240, 28)
	if len(strokesIn(tt.Pump(), cHover)) != 1 {
		t.Fatal("hovered border")
	}
	tt.Tap(240, 28)
	tt.Settle()
	if len(strokesIn(tt.Pump(), cFocus)) != 1 {
		t.Fatal("focused border")
	}
	if len(textsIn(tt, tGreen)) == 0 {
		t.Fatal("focused label color")
	}
	tt = inputField(in, m.TextField{Outlined: true, Label: "Name", Error: "bad"})
	if len(strokesIn(tt.Pump(), cErr)) != 1 {
		t.Fatal("error border")
	}
	tt.Tap(240, 28)
	tt.Settle()
	if len(strokesIn(tt.Pump(), cFocErr)) != 1 {
		t.Fatal("focused error border")
	}
	tt = inputField(in, m.TextField{Outlined: true, Label: "Name", Disabled: true})
	if len(strokesIn(tt.Pump(), cDis)) != 1 {
		t.Fatal("disabled border")
	}
}

// textsIn returns the text commands drawn in color c.
func textsIn(tt *tester.Tester, c geom.Color) []render.Command {
	var out []render.Command
	for _, cmd := range tt.Pump().Commands {
		if cmd.Kind == render.CmdText && cmd.Text != nil && sameRGB(cmd.Color, c) {
			out = append(out, cmd)
		}
		if cmd.Kind == render.CmdText && cmd.Text != nil && sameRGB(cmd.Text.Style.Color, c) {
			out = append(out, cmd)
		}
	}
	return out
}

// A filled outlined field keeps the page color above the border where the
// floated label's notch is, and its own fill below.
func TestNotchOnFilledOutlinedField(t *testing.T) {
	tt := inputField(m.InputDecorationTheme{FillColor: tGreen}, m.TextField{Outlined: true, Label: "Name"})
	tt.Tap(240, 28)
	tt.Settle()
	for _, c := range rectsIn(tt.Pump(), tGreen) {
		if c.Rect.Y < 0 {
			t.Fatalf("fill color painted above the field: %+v", c.Rect)
		}
	}
}

func TestCheckboxAndSwitchStateColors(t *testing.T) {
	cbTheme := m.CheckboxTheme{FillColors: m.StateColor{Selected: tGreen, Hovered: tBlue}, BorderColors: m.StateColor{Normal: tRed}}
	box := func(v bool) *tester.Tester {
		th := m.NewTheme(m.BaselineSeed, false)
		th.Checkbox = cbTheme
		return tester.New(m.App{Theme: th, Home: w.Align{Alignment: geom.TopLeft,
			Child: m.Checkbox{Value: v, OnChanged: func(bool) {}}}}, 200, 100)
	}
	if len(strokesIn(box(false).Pump(), tRed)) != 1 {
		t.Fatal("unchecked outline color")
	}
	tt := box(true)
	if len(rectsIn(tt.Pump(), tGreen)) == 0 {
		t.Fatal("checked fill color")
	}
	tt.Hover(20, 20)
	if len(rectsIn(tt.Pump(), tBlue)) == 0 {
		t.Fatal("hovered checked fill color")
	}

	sw := func(v bool) *tester.Tester {
		th := m.NewTheme(m.BaselineSeed, false)
		th.Switch = m.SwitchTheme{TrackColors: m.StateColor{Normal: tRed, Selected: tGreen, Disabled: tBlue}}
		var onChanged func(bool)
		if v {
			onChanged = func(bool) {}
		}
		return tester.New(m.App{Theme: th, Home: w.Align{Alignment: geom.TopLeft,
			Child: m.Switch{Value: v, OnChanged: onChanged}}}, 200, 100)
	}
	if len(rectsIn(sw(true).Pump(), tGreen)) == 0 {
		t.Fatal("on track color")
	}
	if len(rectsIn(sw(false).Pump(), tBlue)) == 0 {
		t.Fatal("disabled (off) track color")
	}
}
