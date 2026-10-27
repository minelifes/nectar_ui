package tests

import (
	"math"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// fieldAtTop puts one text field at the top-left, so its 56px box is y 0..56.
func fieldAtTop(tf m.TextField) *tester.Tester {
	return tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft,
		Child: w.SizedBox{Width: 480, Child: tf}}}, 600, 200)
}

// editCenterY is the vertical center of the field's editable line (the
// typed text's line box).
func editCenterY(t *testing.T, tt *tester.Tester, want string) float32 {
	t.Helper()
	tt.Pump()
	var r geom.Rect
	found := false
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if e, ok := ro.(*render.RenderEditable); ok && e.Text == want {
			r, found = geom.RectFrom(render.GlobalOrigin(ro), ro.Base().Size()), true
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	if !found {
		t.Fatalf("no editable with %q (%v)", want, tt.Texts())
	}
	return r.Y + r.H/2
}

func centerY(t *testing.T, tt *tester.Tester, s string) float32 {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("%q not drawn (%v)", s, tt.Texts())
	}
	return r.Y + r.H/2
}

// Text without a label above it is centered in the field: hint and typed
// text alike (they were 4px low: 16 above the line, 8 below).
func TestTextFieldTextVerticallyCentered(t *testing.T) {
	for name, tf := range map[string]m.TextField{
		"outlined, icon":   {Outlined: true, Prefix: icons.Search, Hint: "Filter by name"},
		"outlined":         {Outlined: true, Hint: "Filter by name"},
		"filled, no label": {Hint: "Filter by name"},
	} {
		tt := fieldAtTop(tf)
		if y := centerY(t, tt, "Filter by name"); math.Abs(float64(y-28)) > 0.5 {
			t.Errorf("%s: hint centered at y %v, want 28", name, y)
		}
		tt.Tap(240, 28)
		tt.Type("dfdf")
		tt.Settle()
		if y := editCenterY(t, tt, "dfdf"); math.Abs(float64(y-28)) > 0.5 {
			t.Errorf("%s: text centered at y %v, want 28", name, y)
		}
	}
}

// A filled field with a label keeps M3's layout: label on top, text below.
func TestFilledLabelFieldKeepsTextBelowLabel(t *testing.T) {
	tt := fieldAtTop(m.TextField{Label: "Name"})
	tt.Tap(240, 28)
	tt.Type("Ada")
	tt.Settle()
	if y := editCenterY(t, tt, "Ada"); math.Abs(float64(y-36)) > 0.5 {
		t.Fatalf("text centered at y %v, want 36 (24 + 24/2)", y)
	}
	if y := centerY(t, tt, "Name"); y > 20 {
		t.Fatalf("floated label at y %v, want in the upper part", y)
	}
}

// The outlined label still rests centered and floats onto the border.
func TestOutlinedLabelPositions(t *testing.T) {
	tt := fieldAtTop(m.TextField{Outlined: true, Label: "Search"})
	if y := centerY(t, tt, "Search"); math.Abs(float64(y-28)) > 0.5 {
		t.Fatalf("resting label centered at y %v, want 28", y)
	}
	tt.Tap(240, 28)
	tt.Settle()
	if y := centerY(t, tt, "Search"); math.Abs(float64(y)) > 0.5 {
		t.Fatalf("floated label centered at y %v, want 0 (on the border)", y)
	}
}

// fieldHeight is the height of the field's box (the first text field).
func fieldHeight(tt *tester.Tester) float32 {
	tt.Pump()
	var h float32
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if _, ok := ro.(*render.RenderCustomPaint); ok && h == 0 {
			if s := ro.Base().Size(); s.W == 480 {
				h = s.H
				return
			}
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	return h
}

func TestTextFieldContentPadding(t *testing.T) {
	compact := geom.InsetsLTRB(10, 6, 10, 6)
	// From the theme.
	th := m.NewTheme(m.BaselineSeed, false)
	th.Input.ContentPadding = &compact
	tt := tester.New(m.App{Theme: th, Home: w.Align{Alignment: geom.TopLeft,
		Child: w.SizedBox{Width: 480, Child: m.TextField{Outlined: true, Hint: "Filter"}}}}, 600, 200)
	if h := fieldHeight(tt); h != 6+24+6 {
		t.Fatalf("compact field is %vpx tall, want 36 (padding + one line)", h)
	}
	r, _ := tt.Find("Filter")
	if r.X != 10 || r.Y != 6 {
		t.Fatalf("hint at %v, want at (10, 6)", r)
	}
	// From the widget's Style, with an icon: the padding starts after it.
	wide := geom.InsetsLTRB(4, 16, 4, 16)
	tt = fieldAtTop(m.TextField{Outlined: true, Prefix: icons.Search, Hint: "Filter",
		Style: m.InputDecorationTheme{ContentPadding: &wide}})
	if r, _ := tt.Find("Filter"); r.X != 12+24+4 {
		t.Fatalf("hint at x %v, want 40 (12 + 24 icon + 4)", r.X)
	}
	// Typed text follows the same padding.
	tt.Tap(240, 28)
	tt.Type("abc")
	tt.Settle()
	if y := editCenterY(t, tt, "abc"); math.Abs(float64(y-28)) > 0.5 {
		t.Fatalf("text centered at y %v, want 28", y)
	}
}

func TestContentPaddingKeepsLabelsAligned(t *testing.T) {
	// Outlined: the resting label is centered on the text line, and the
	// floated one still sits on the border, 16px from the edge.
	p := geom.InsetsLTRB(24, 10, 24, 10)
	tt := fieldAtTop(m.TextField{Outlined: true, Label: "Name", Prefix: icons.Search,
		Style: m.InputDecorationTheme{ContentPadding: &p}})
	if y := centerY(t, tt, "Name"); math.Abs(float64(y-(10+12))) > 0.5 {
		t.Fatalf("resting label centered at y %v, want 22 (text line 10..34)", y)
	}
	if r, _ := tt.Find("Name"); r.X != 12+24+24 {
		t.Fatalf("resting label at x %v, want 60 (after the icon and padding)", r.X)
	}
	tt.Tap(240, 20)
	tt.Settle()
	r, _ := tt.Find("Name")
	if math.Abs(float64(r.Y+r.H/2)) > 0.5 || r.X != 16 {
		t.Fatalf("floated label at %v, want centered on the border at x 16", r)
	}
	// Filled with a label: the label floats into the band above the text.
	f := geom.InsetsLTRB(12, 28, 12, 8)
	tt = fieldAtTop(m.TextField{Label: "Name", Style: m.InputDecorationTheme{ContentPadding: &f}})
	tt.Tap(240, 30)
	tt.Type("Ada")
	tt.Settle()
	if y := editCenterY(t, tt, "Ada"); math.Abs(float64(y-(28+12))) > 0.5 {
		t.Fatalf("text centered at y %v, want 40", y)
	}
	if r, _ := tt.Find("Name"); r.Y+r.H > 28 || r.X != 12 {
		t.Fatalf("floated label at %v, want above the text (y < 28) at x 12", r)
	}
}
