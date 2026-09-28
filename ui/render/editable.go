package render

import (
	"strings"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// RenderEditable draws editable text with a caret and selection. Lines are
// split on '\n' only (no soft wrapping); single-line fields scroll
// horizontally to keep the caret visible.
type RenderEditable struct {
	Box
	Text           string
	Style          text.Style
	CursorColor    geom.Color
	SelectionColor geom.Color
	Focused        bool
	CaretVisible   bool
	// Selection as byte offsets into Text; Base == Extent is a caret.
	SelBase, SelExtent int
	MinLines           int // height in lines (at least 1)

	lines   []editLine
	lineH   float32
	scrollX float32
	cached  string
	cachedS text.Style
}

type editLine struct {
	start   int // byte offset of the line in Text
	s       string
	para    *text.Paragraph
	stops   []float32
	offsets []int
}

func (r *RenderEditable) relayoutText() {
	if r.lines != nil && r.cached == r.Text && r.cachedS == r.Style {
		return
	}
	r.cached, r.cachedS = r.Text, r.Style
	r.lineH, _ = text.LineMetrics(r.Style)
	r.lines = r.lines[:0]
	start := 0
	for _, ln := range strings.Split(r.Text, "\n") {
		st, off := text.CaretStops(ln, r.Style)
		r.lines = append(r.lines, editLine{
			start: start, s: ln,
			para:  text.Layout(ln, r.Style, text.Options{}),
			stops: st, offsets: off,
		})
		start += len(ln) + 1
	}
}

// Update sets everything that affects drawing; relayouts only if needed.
func (r *RenderEditable) Update(s string, style text.Style, base, extent int, focused, caret bool, cursor, selection geom.Color, minLines int) {
	layoutChanged := s != r.Text || style != r.Style || minLines != r.MinLines
	r.Text, r.Style, r.SelBase, r.SelExtent = s, style, base, extent
	r.Focused, r.CaretVisible, r.CursorColor, r.SelectionColor = focused, caret, cursor, selection
	r.MinLines = minLines
	if layoutChanged {
		MarkNeedsLayout(r)
	} else {
		r.keepCaretVisible()
		MarkNeedsPaint(r)
	}
}

func (r *RenderEditable) PerformLayout(c geom.Constraints) geom.Size {
	r.relayoutText()
	n := max(len(r.lines), r.MinLines, 1)
	var w float32
	for _, l := range r.lines {
		w = max(w, l.stops[len(l.stops)-1])
	}
	size := geom.Size{W: w + 2, H: float32(n) * r.lineH}
	if c.HasBoundedWidth() {
		size.W = c.MaxW
	}
	size = c.Constrain(size)
	r.size = size
	r.keepCaretVisible()
	return size
}

// lineOf returns the line index containing byte offset i.
func (r *RenderEditable) lineOf(i int) int {
	for li := len(r.lines) - 1; li >= 0; li-- {
		if i >= r.lines[li].start {
			return li
		}
	}
	return 0
}

// xOf returns the x of byte offset i within its line.
func (r *RenderEditable) xOf(i int) (line int, x float32) {
	li := r.lineOf(i)
	l := r.lines[li]
	rel := i - l.start
	for k, off := range l.offsets {
		if off >= rel {
			return li, l.stops[k]
		}
	}
	return li, l.stops[len(l.stops)-1]
}

func (r *RenderEditable) keepCaretVisible() {
	if len(r.lines) != 1 || r.size.W <= 0 {
		r.scrollX = 0
		return
	}
	_, x := r.xOf(r.SelExtent)
	full := r.lines[0].stops[len(r.lines[0].stops)-1]
	view := r.size.W - 2
	if x-r.scrollX > view {
		r.scrollX = x - view
	}
	if x-r.scrollX < 0 {
		r.scrollX = x
	}
	r.scrollX = max(0, min(r.scrollX, max(0, full-view)))
}

// OffsetAt returns the byte offset of the caret stop nearest to local.
func (r *RenderEditable) OffsetAt(local geom.Offset) int {
	r.relayoutText()
	if len(r.lines) == 0 {
		return 0
	}
	li := int(local.Y / max(r.lineH, 1))
	li = max(0, min(li, len(r.lines)-1))
	return r.offsetInLine(li, local.X+r.scrollX)
}

func (r *RenderEditable) offsetInLine(li int, x float32) int {
	l := r.lines[li]
	best, bestD := 0, float32(1e9)
	for k, st := range l.stops {
		d := st - x
		if d < 0 {
			d = -d
		}
		if d < bestD {
			best, bestD = k, d
		}
	}
	return l.start + l.offsets[best]
}

// VerticalMove returns the offset one line up (dir<0) or down from i,
// keeping the x position; ok is false at the first/last line.
func (r *RenderEditable) VerticalMove(i, dir int) (int, bool) {
	r.relayoutText()
	li, x := r.xOf(i)
	nl := li + dir
	if nl < 0 || nl >= len(r.lines) {
		return i, false
	}
	return r.offsetInLine(nl, x), true
}

// LineBounds returns the byte range of the line containing i.
func (r *RenderEditable) LineBounds(i int) (start, end int) {
	r.relayoutText()
	l := r.lines[r.lineOf(i)]
	return l.start, l.start + len(l.s)
}

func (r *RenderEditable) Paint(ctx *PaintContext, o geom.Offset) {
	c := ctx.Canvas
	rect := geom.RectFrom(o, r.size)
	c.PushClip(geom.Rect{X: rect.X - 1, Y: rect.Y, W: rect.W + 2, H: rect.H})
	lo, hi := min(r.SelBase, r.SelExtent), max(r.SelBase, r.SelExtent)
	for li, l := range r.lines {
		y := o.Y + float32(li)*r.lineH
		x0 := o.X - r.scrollX
		if lo != hi && r.SelectionColor.A > 0 {
			ls, le := l.start, l.start+len(l.s)
			s, e := max(lo, ls), min(hi, le)
			if s < e || (lo <= le && hi > le && li < len(r.lines)-1) {
				_, sx := r.xOf(max(s, ls))
				_, ex := r.xOf(min(e, le))
				if hi > le && li < len(r.lines)-1 {
					ex += r.lineH * 0.3 // show the selected newline
				}
				c.FillRect(geom.Rect{X: x0 + sx, Y: y, W: ex - sx, H: r.lineH}, r.SelectionColor)
			}
		}
		c.DrawParagraph(l.para, geom.Offset{X: x0, Y: y})
	}
	if r.Focused && r.CaretVisible && r.CursorColor.A > 0 && len(r.lines) > 0 {
		li, x := r.xOf(r.SelExtent)
		c.FillRect(geom.Rect{X: o.X - r.scrollX + x - 1, Y: o.Y + float32(li)*r.lineH, W: 2, H: r.lineH}, r.CursorColor)
	}
	c.PopClip()
}

func (r *RenderEditable) VisitChildren(func(RenderObject)) {}
func (r *RenderEditable) HitTestSelf(geom.Offset) bool     { return true }
func (r *RenderEditable) Cursor() Cursor                   { return CursorText }
