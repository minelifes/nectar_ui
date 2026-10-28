package tests

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

const longText = "This text is far too long to fit inside the widget it was put in, so it must not paint outside"

// visibleText returns the visible part (rect ∩ clip) of every text command.
func visibleText(c *render.Canvas) []geom.Rect {
	var out []geom.Rect
	for _, cmd := range c.Commands {
		if cmd.Kind == render.CmdText {
			if v := cmd.Rect.Intersect(cmd.Clip); !v.Empty() {
				out = append(out, v)
			}
		}
	}
	return out
}

func within(r, box geom.Rect) bool {
	const eps = 0.5
	return r.X >= box.X-eps && r.Y >= box.Y-eps && r.Right() <= box.Right()+eps && r.Bottom() <= box.Bottom()+eps
}

// A wide row in a narrow box: content that can't wrap.
func wideRow() w.Widget {
	return w.Row{Children: []w.Widget{
		w.SizedBox{Width: 300, Height: 20, Child: w.Text{Text: "left part of a wide row"}},
		w.SizedBox{Width: 300, Height: 20, Child: w.Text{Text: "right part far outside"}},
	}}
}

func TestContainersClipTheirContent(t *testing.T) {
	box := geom.Rect{X: 20, Y: 20, W: 200, H: 80}
	place := func(child w.Widget) w.Widget {
		return m.App{Home: w.Stack{Children: []w.Widget{
			w.Positioned{Left: w.At(box.X), Top: w.At(box.Y), Width: w.At(box.W), Height: w.At(box.H), Child: child},
		}}}
	}
	cases := map[string]w.Widget{
		"card":           m.Card{Variant: m.CardOutlined, Child: wideRow()},
		"tappable card":  m.Card{OnTap: func() {}, Child: wideRow()},
		"surface":        m.Surface{Color: geom.Hex(0xEEEEEE), Child: wideRow()},
		"container":      w.Container{Color: geom.Hex(0xEEEEEE), Child: wideRow()},
		"widgets card":   w.Card{Body: wideRow()},
		"button":         m.FilledButton{Label: longText, OnPressed: func() {}},
		"hint":           w.Align{Alignment: geom.TopLeft, Child: m.TextField{Hint: longText}},
		"outlined hint":  w.Align{Alignment: geom.TopLeft, Child: m.TextField{Hint: longText, Outlined: true}},
		"label":          w.Align{Alignment: geom.TopLeft, Child: m.TextField{Label: longText}},
		"outlined label": w.Align{Alignment: geom.TopLeft, Child: m.TextField{Label: longText, Outlined: true}},
	}
	for name, c := range cases {
		tt := tester.New(place(c), 600, 300)
		tt.Settle()
		texts := visibleText(tt.Pump())
		if len(texts) == 0 {
			t.Errorf("%s: no text drawn", name)
		}
		for _, r := range texts {
			if !within(r, box) {
				t.Errorf("%s: text visible at %v, outside the widget %v", name, r, box)
			}
		}
	}
}

// Text fields lay out a long hint or label to fit (ending in "…"), so it
// isn't just cut at the edge.
func TestTextFieldLongHintFits(t *testing.T) {
	for i, tf := range []m.TextField{{Hint: longText}, {Hint: longText, Outlined: true}, {Label: longText}, {Label: longText, Outlined: true}} {
		tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft,
			Child: w.SizedBox{Width: 220, Child: tf}}}, 600, 200)
		tt.Settle()
		n := 0
		for _, cmd := range tt.Pump().Commands {
			if cmd.Kind != render.CmdText || cmd.Text == nil || cmd.Rect.W == 0 {
				continue
			}
			n++
			if cmd.Rect.X < 0 || cmd.Rect.Right() > 220 {
				t.Errorf("case %d: text laid out at %v, wider than the 220px field", i, cmd.Rect)
			}
		}
		if n == 0 {
			t.Errorf("case %d: nothing drawn", i)
		}
	}
}

// Overflow: true keeps the old behavior.
func TestOverflowOptOut(t *testing.T) {
	box := geom.Rect{X: 20, Y: 20, W: 200, H: 80}
	tt := tester.New(m.App{Home: w.Stack{Children: []w.Widget{
		w.Positioned{Left: w.At(box.X), Top: w.At(box.Y), Width: w.At(box.W), Height: w.At(box.H),
			Child: m.Card{Overflow: true, Child: wideRow()}},
	}}}, 900, 300)
	tt.Settle()
	outside := false
	for _, r := range visibleText(tt.Pump()) {
		if !within(r, box) {
			outside = true
		}
	}
	if !outside {
		t.Fatal("Overflow: true still clipped")
	}
}

// Buttons narrower than their label shrink it to fit with "…"; with room
// they keep their natural size.
func TestButtonLabelFits(t *testing.T) {
	buttons := map[string]w.Widget{
		"filled":   m.FilledButton{Label: longText, OnPressed: func() {}},
		"outlined": m.OutlinedButton{Label: longText, OnPressed: func() {}},
		"text":     m.TextButton{Label: longText, OnPressed: func() {}},
		"icon":     m.FilledButton{Label: longText, Icon: icons.Add, OnPressed: func() {}},
		"fab":      m.FloatingActionButton{Label: longText, Icon: icons.Add, OnPressed: func() {}},
	}
	for name, b := range buttons {
		tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: 200, Child: b}}}, 900, 200)
		tt.Settle()
		n := 0
		for _, cmd := range tt.Pump().Commands {
			if cmd.Kind == render.CmdText && cmd.Rect.W > 0 {
				n++
				if cmd.Rect.X < 0 || cmd.Rect.Right() > 200 {
					t.Errorf("%s: label laid out at %v, past the 200px button", name, cmd.Rect)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: no label drawn", name)
		}
		// Unconstrained: the whole label, on one line.
		tt = tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: m.FilledButton{Label: "Save", OnPressed: func() {}}}}, 900, 200)
		tt.Settle()
		if r, ok := tt.Find("Save"); !ok || r.W < 20 || r.H > 30 {
			t.Fatalf("short label: %v %v", r, ok)
		}
	}
}
