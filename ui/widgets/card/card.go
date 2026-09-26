package card

import (
	"nectar_ui/ui/geom"
	"nectar_ui/ui/text"
	w "nectar_ui/ui/widgets"
)

type Card struct {
	Title    string
	Body     string
	MaxLines int
}

func (c Card) Build(w.BuildContext) w.Widget {
	return w.Container{
		Color: card,
		Radius: 10,
		BorderColor: line,
		BorderWidth: 1,
		Padding: geom.Insets(16),
		Child: w.Column{Spacing: 8, Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
			w.Text{Text: c.Title, Style: text.Style{Size: 13, Color: muted, Font: text.DefaultBoldFont()}},
			w.Text{Text: c.Body, MaxLines: c.MaxLines, Ellipsis: c.MaxLines > 0, Style: text.Style{LineHeight: 1.4}},
		}},
	}
}
