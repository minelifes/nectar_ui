package main

import (
	"fmt"
	"log"
	"time"

	"nectar_ui/ui"
	"nectar_ui/ui/geom"
	"nectar_ui/ui/text"
	w "nectar_ui/ui/widgets"
)

func main() {
	app := ui.NewApp(ui.DefaultConfig().WithTitle("Nectar UI").WithSize(960, 640))
	if err := app.Run(Demo{}); err != nil {
		log.Fatal(err)
	}
}

// --- palette ----------------------------------------------------------------

var (
	ink     = geom.Hex(0x1f2328)
	muted   = geom.Hex(0x6a737d)
	honey   = geom.Hex(0xf2a900)
	card    = geom.White
	line    = geom.Hex(0xe4e0d6)
	sidebar = geom.Hex(0x22252b)
)

// Demo is the root widget (stateless: it only composes other widgets).
type Demo struct{}

func (Demo) Build(ctx w.BuildContext) w.Widget {
	return w.DefaultTextStyle{
		Style: text.Style{Size: 15, Color: ink},
		Child: w.Row{Cross: w.CrossStretch, Children: []w.Widget{
			Sidebar{},
			w.Expanded{Child: w.Container{
				Padding: geom.Insets(28),
				Child: w.Column{Spacing: 18, Cross: w.CrossStretch, Children: []w.Widget{
					w.Text{Text: "Text rendering", Style: text.Style{Size: 28, Font: text.DefaultBoldFont()}},
					w.Row{Spacing: 18, Cross: w.CrossStart, Children: []w.Widget{
						w.Expanded{Child: Card{Title: "Wrapping", Body: loremEN}},
						w.Expanded{Child: Card{Title: "Кирилиця", Body: loremUA}},
					}},
					w.Row{Spacing: 18, Cross: w.CrossStart, Children: []w.Widget{
						w.Expanded{Child: Card{Title: "Ellipsis (MaxLines: 2)", Body: loremEN, MaxLines: 2}},
						w.Expanded{Child: Clock{}},
					}},
					Sizes{},
				}},
			}},
		}},
	}
}

// Sidebar shows alignment and rounded boxes.
type Sidebar struct{}

func (Sidebar) Build(w.BuildContext) w.Widget {
	item := func(label string, active bool) w.Widget {
		bg := geom.Transparent
		fg := geom.Hex(0xc9d1d9)
		if active {
			bg, fg = honey, sidebar
		}
		return w.Container{
			Padding: geom.InsetsHV(12, 8),
			Color:   bg,
			Border: &geom.Border{
				Radius: 6,
			},
			Child: w.Text{Text: label, Style: text.Style{Color: fg}},
		}
	}
	return w.Container{
		Width: 200, Color: sidebar, Padding: geom.Insets(16),
		Child: w.Column{Spacing: 6, Cross: w.CrossStretch, Children: []w.Widget{
			w.Text{Text: "nectar", Style: text.Style{Size: 20, Color: honey, Font: text.DefaultBoldFont()}},
			w.SizedBox{Height: 12},
			item("Text", true),
			item("Layout", false),
			item("Widgets", false),
			w.Spacer{},
			w.Text{Text: "wgpu · gogpu", Style: text.Style{Size: 12, Color: muted}, Align: text.AlignCenter},
		}},
	}
}

// Card is a titled, bordered box.
type Card struct {
	Title    string
	Body     string
	MaxLines int
}

func (c Card) Build(w.BuildContext) w.Widget {
	return w.Container{
		Color: card,
		Border: &geom.Border{
			Radius: 10,
			Color:  line,
			Width:  1,
		},
		Padding: geom.Insets(16),
		Child: w.Column{Spacing: 8, Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
			w.Text{Text: c.Title, Style: text.Style{Size: 13, Color: muted, Font: text.DefaultBoldFont()}},
			w.Text{Text: c.Body, MaxLines: c.MaxLines, Ellipsis: c.MaxLines > 0, Style: text.Style{LineHeight: 1.4}},
		}},
	}
}

// Clock is a StatefulWidget updated from another goroutine via Post.
type Clock struct{}

func (Clock) CreateState() w.State { return &clockState{} }

type clockState struct {
	w.StateBase
	now  time.Time
	stop chan struct{}
}

func (s *clockState) InitState() {
	s.now = time.Now()
	s.stop = make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case now := <-t.C:
				s.Post(func() { s.now = now }) // thread-safe SetState
			}
		}
	}()
}

func (s *clockState) Dispose() { close(s.stop) }

func (s *clockState) Build(w.BuildContext) w.Widget {
	return Card{Title: "StatefulWidget", Body: fmt.Sprintf("Rebuilt every second: %s", s.now.Format("15:04:05"))}
}

// Sizes shows a size ramp.
type Sizes struct{}

func (Sizes) Build(w.BuildContext) w.Widget {
	var kids []w.Widget
	for _, sz := range []float32{10, 12, 14, 18, 24, 32, 44} {
		kids = append(kids, w.Text{Text: "Aa", Style: text.Style{Size: sz}})
	}
	return w.Container{
		Color: card,
		Border: &geom.Border{
			Radius: 10,
			Color:  line,
			Width:  1,
		},
		Padding: geom.Insets(16),
		Child:   w.Row{Spacing: 20, Cross: w.CrossEnd, Children: kids},
	}
}

const loremEN = "Glyphs are rasterized on the CPU into an atlas at physical-pixel size " +
	"with 4 subpixel positions, then drawn as textured quads in a single batched draw call. " +
	"Kerning and word wrapping come from the font tables."

const loremUA = "Гліфи растеризуються в атлас один раз і перевикористовуються. " +
	"Шрифт Go підтримує латиницю, кирилицю та грецьку: ґ, є, і, ї — усе на місці."
