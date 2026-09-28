package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
)

type Card struct {
	Body       Widget
	Background *geom.Color
	Border     *geom.Border
}

func (c Card) Build(BuildContext) Widget {
	bg := geom.Hex(0xffffff)
	if c.Background != nil {
		bg = *c.Background
	}
	return Container{
		Color:   bg,
		Border:  c.Border,
		Padding: geom.Insets(16),
		Child:   c.Body,
	}
}
