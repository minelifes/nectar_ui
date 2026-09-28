// Input demo: buttons with hover/pressed states, a click counter and a
// draggable box.
//
//	CGO_ENABLED=0 go run ./examples/input
package main

import (
	"fmt"
	"log"

	"github.com/minelifes/nectar_ui/ui"
	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

func main() {
	app := ui.NewApp(ui.DefaultConfig().WithTitle("Nectar UI — input").WithSize(720, 480))
	if err := app.Run(Demo{}); err != nil {
		log.Fatal(err)
	}
}

var (
	ink   = geom.Hex(0x1f2328)
	honey = geom.Hex(0xf2a900)
)

// Demo holds the counter.
type Demo struct{}

func (Demo) CreateState() w.State { return &demoState{} }

type demoState struct {
	w.StateBase
	clicks int
}

func (s *demoState) Build(w.BuildContext) w.Widget {
	return w.DefaultTextStyle{Style: text.Style{Size: 16, Color: ink}, Child: w.Container{
		Padding: geom.Insets(32),
		Child: w.Column{Spacing: 20, Cross: w.CrossStart, Children: []w.Widget{
			w.Text{Text: "Input", Style: text.Style{Size: 28, Font: text.DefaultBoldFont()}},
			w.Row{Spacing: 12, ShrinkMain: true, Children: []w.Widget{
				Button{Label: "Click me", OnPressed: func() { s.SetState(func() { s.clicks++ }) }},
				Button{Label: "Reset", OnPressed: func() { s.SetState(func() { s.clicks = 0 }) }},
			}},
			w.Text{Text: fmt.Sprintf("Clicked %d times", s.clicks)},
			w.Expanded{Child: Draggable{}},
		}},
	}}
}

// Button is a StatefulWidget: hover and pressed state live in its State,
// the click handler comes from the parent.
type Button struct {
	Label     string
	OnPressed func()
}

func (Button) CreateState() w.State { return &buttonState{} }

type buttonState struct {
	w.StateBase
	hovered, pressed bool
}

func (s *buttonState) Build(w.BuildContext) w.Widget {
	b := w.WidgetOf[Button](s)
	bg := honey
	switch {
	case s.pressed:
		bg = geom.Hex(0xc98c00)
	case s.hovered:
		bg = geom.Hex(0xffbf1f)
	}
	return w.MouseRegion{
		Cursor:  w.CursorPointer,
		OnEnter: func(w.PointerEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(w.PointerEvent) { s.SetState(func() { s.hovered = false }) },
		Child: w.GestureDetector{
			OnTapDown:   func(w.TapDetails) { s.SetState(func() { s.pressed = true }) },
			OnTapCancel: func() { s.SetState(func() { s.pressed = false }) },
			OnTap: func() {
				s.SetState(func() { s.pressed = false })
				if b.OnPressed != nil {
					b.OnPressed()
				}
			},
			Child: w.Container{
				Color:   bg,
				Border:  &geom.Border{Radius: 8},
				Padding: geom.InsetsHV(18, 10),
				Child:   w.Text{Text: b.Label, Style: text.Style{Font: text.DefaultBoldFont()}},
			},
		},
	}
}

// Draggable is a box you can drag around its area.
type Draggable struct{}

func (Draggable) CreateState() w.State { return &draggableState{pos: geom.Pt(20, 20)} }

type draggableState struct {
	w.StateBase
	pos      geom.Offset
	dragging bool
}

func (s *draggableState) Build(w.BuildContext) w.Widget {
	color := geom.Hex(0x3b82f6)
	if s.dragging {
		color = geom.Hex(0x1d4ed8)
	}
	return w.Container{
		Color:  geom.Hex(0xeceae4),
		Border: &geom.Border{Radius: 12},
		Child: w.ClipRect{Child: w.Padding{
			Padding: geom.InsetsLTRB(s.pos.X, s.pos.Y, 0, 0),
			Child: w.Align{Alignment: geom.TopLeft, Child: w.MouseRegion{
				Cursor: w.CursorMove,
				Child: w.GestureDetector{
					OnPanStart:  func(w.DragDetails) { s.SetState(func() { s.dragging = true }) },
					OnPanUpdate: func(d w.DragDetails) { s.SetState(func() { s.pos = s.pos.Add(d.Delta) }) },
					OnPanEnd:    func(w.DragDetails) { s.SetState(func() { s.dragging = false }) },
					Child: w.Container{
						Width: 120, Height: 80, Color: color, Border: &geom.Border{Radius: 10},
						Alignment: w.Ptr(geom.Center),
						Child:     w.Text{Text: "drag me", Style: text.Style{Color: geom.White}},
					},
				},
			}},
		}},
	}
}
