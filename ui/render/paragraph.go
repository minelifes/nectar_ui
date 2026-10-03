package render

import (
	"slices"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// RenderParagraph lays out and paints a string.
type RenderParagraph struct {
	Box
	spans    []text.Span // rich text; nil = text in style
	text     string
	style    text.Style
	align    text.Align
	maxLines int
	ellipsis string

	para      *text.Paragraph
	paraWidth float32 // max width the cached paragraph was laid out with

	selA, selB int // selected byte range (selA == selB: none)
	selColor   geom.Color
}

// SetSelection highlights the bytes between a and b (in any order) with
// color; a == b clears it.
func (r *RenderParagraph) SetSelection(a, b int, color geom.Color) {
	if a > b {
		a, b = b, a
	}
	if r.selA != a || r.selB != b || r.selColor != color {
		r.selA, r.selB, r.selColor = a, b, color
		MarkNeedsPaint(r)
	}
}

// OffsetAt returns the byte offset of the caret position nearest to local.
func (r *RenderParagraph) OffsetAt(local geom.Offset) int {
	if r.para == nil {
		return 0
	}
	return r.para.OffsetAt(local)
}

// NewParagraph creates a paragraph render object.
func NewParagraph(s string, style text.Style, align text.Align) *RenderParagraph {
	return &RenderParagraph{text: s, style: style, align: align}
}

// Text returns the current string (the spans joined, for rich text).
func (r *RenderParagraph) Text() string {
	if r.spans != nil {
		var s string
		for _, sp := range r.spans {
			s += sp.Text
		}
		return s
	}
	return r.text
}

// Paragraph returns the most recent layout (nil before first layout).
func (r *RenderParagraph) Paragraph() *text.Paragraph { return r.para }

// UpdateSpans shows styled runs of text (see text.LayoutSpans), re-laying
// out only when something changed.
func (r *RenderParagraph) UpdateSpans(spans []text.Span, style text.Style, align text.Align, maxLines int, ellipsis string) {
	if r.spans != nil && slices.Equal(r.spans, spans) && r.style == style && r.align == align && r.maxLines == maxLines && r.ellipsis == ellipsis {
		return
	}
	r.spans = append(r.spans[:0:0], spans...)
	if r.spans == nil {
		r.spans = []text.Span{}
	}
	r.text, r.style, r.align, r.maxLines, r.ellipsis = "", style, align, maxLines, ellipsis
	r.para = nil
	MarkNeedsLayout(r)
}

// Update changes content/style, re-laying out only when something changed.
func (r *RenderParagraph) Update(s string, style text.Style, align text.Align, maxLines int, ellipsis string) {
	if r.spans != nil {
		r.spans, r.para = nil, nil
		r.text, r.style, r.align, r.maxLines, r.ellipsis = s, style, align, maxLines, ellipsis
		MarkNeedsLayout(r)
		return
	}
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
		opt := text.Options{MaxWidth: c.MaxW, MaxLines: r.maxLines, Ellipsis: r.ellipsis}
		if r.spans != nil {
			r.para = text.LayoutSpans(r.spans, r.style, opt)
		} else {
			r.para = text.Layout(r.text, r.style, opt)
		}
		r.paraWidth = c.MaxW
		InvalidatePaint(r) // may wrap differently even at the same size
	}
	size := c.Constrain(r.para.Size())
	r.para.SetAlign(r.align, size.W)
	return size
}

func (r *RenderParagraph) Paint(ctx *PaintContext, o geom.Offset) {
	if r.selA != r.selB && r.para != nil {
		for _, rc := range r.para.SelectionRects(r.selA, r.selB) {
			rc.X += o.X
			rc.Y += o.Y
			ctx.Canvas.FillRect(rc, r.selColor)
		}
	}
	ctx.Canvas.DrawParagraph(r.para, o)
}

func (r *RenderParagraph) VisitChildren(func(RenderObject)) {}
