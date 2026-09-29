package render

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// RenderParagraph lays out and paints a string.
type RenderParagraph struct {
	Box
	text     string
	style    text.Style
	align    text.Align
	maxLines int
	ellipsis string

	para      *text.Paragraph
	paraWidth float32 // max width the cached paragraph was laid out with
}

// NewParagraph creates a paragraph render object.
func NewParagraph(s string, style text.Style, align text.Align) *RenderParagraph {
	return &RenderParagraph{text: s, style: style, align: align}
}

// Text returns the current string.
func (r *RenderParagraph) Text() string { return r.text }

// Paragraph returns the most recent layout (nil before first layout).
func (r *RenderParagraph) Paragraph() *text.Paragraph { return r.para }

// Update changes content/style, re-laying out only when something changed.
func (r *RenderParagraph) Update(s string, style text.Style, align text.Align, maxLines int, ellipsis string) {
	if r.text == s && r.style == style && r.align == align && r.maxLines == maxLines && r.ellipsis == ellipsis {
		return
	}
	colorOnly := r.text == s && r.align == align && r.maxLines == maxLines && r.ellipsis == ellipsis &&
		withColor(r.style, style.Color) == style
	r.text, r.style, r.align, r.maxLines, r.ellipsis = s, style, align, maxLines, ellipsis
	if colorOnly && r.para != nil {
		r.para.Style.Color = style.Resolved().Color
		MarkNeedsPaint(r)
		return
	}
	r.para = nil
	MarkNeedsLayout(r)
}

func withColor(s text.Style, c geom.Color) text.Style { s.Color = c; return s }

func (r *RenderParagraph) PerformLayout(c geom.Constraints) geom.Size {
	if r.para == nil || r.paraWidth != c.MaxW {
		r.para = text.Layout(r.text, r.style, text.Options{
			MaxWidth: c.MaxW, MaxLines: r.maxLines, Ellipsis: r.ellipsis,
		})
		r.paraWidth = c.MaxW
		InvalidatePaint(r) // may wrap differently even at the same size
	}
	size := c.Constrain(r.para.Size())
	r.para.SetAlign(r.align, size.W)
	return size
}

func (r *RenderParagraph) Paint(ctx *PaintContext, o geom.Offset) {
	ctx.Canvas.DrawParagraph(r.para, o)
}

func (r *RenderParagraph) VisitChildren(func(RenderObject)) {}
