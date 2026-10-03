package render

import (
	"strings"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// RenderEditable draws editable text with a caret and selection. Lines
// split on '\n'; with Wrap set they also soft-wrap at the box width (like
// Text). Single-line fields scroll horizontally to keep the caret visible.
//
// During IME composition the preedit text is already part of Text; the
// range [ComposeStart, ComposeEnd) is underlined and the selection hidden.
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
	Wrap               bool
	// ComposeStart/End mark the IME preedit in Text (-1 = not composing).
	ComposeStart, ComposeEnd int

	lines   []editLine
	lineH   float32
	scrollX float32
	cached  string
	cachedS text.Style
	cachedW float32
	wrapW   float32 // wrap width used for lines (0 = none)
}

// editLine is one visual line.
type editLine struct {
	start   int // byte offset of the line in Text
	s       string
	hard    bool // ends with '\n' (not a soft wrap)
	para    *text.Paragraph
	stops   []float32
	offsets []int
}

func (r *RenderEditable) relayoutText() {
	if r.lines != nil && r.cached == r.Text && r.cachedS == r.Style && r.cachedW == r.wrapW {
		return
	}
	r.cached, r.cachedS, r.cachedW = r.Text, r.Style, r.wrapW
	r.lineH, _ = text.LineMetrics(r.Style)
	r.lines = r.lines[:0]
	start := 0
	hardLines := strings.Split(r.Text, "\n")
	for hi, hl := range hardLines {
		breaks := text.LineBreaks(hl, r.Style, r.wrapW)
		for bi, b := range breaks {
			end := len(hl)
			if bi+1 < len(breaks) {
				end = breaks[bi+1]
			}
			seg := hl[b:end]
			st, off := text.CaretStops(seg, r.Style)
			r.lines = append(r.lines, editLine{
				start: start + b, s: seg,
				hard:  bi == len(breaks)-1 && hi < len(hardLines)-1,
				para:  text.Layout(seg, r.Style, text.Options{}),
				stops: st, offsets: off,
			})
		}
		start += len(hl) + 1
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

// SetWrap turns soft wrapping on or off.
func (r *RenderEditable) SetWrap(wrap bool) {
	if r.Wrap != wrap {
		r.Wrap = wrap
		MarkNeedsLayout(r)
	}
}

// SetComposing marks the IME preedit range in Text (start < 0 = none).
func (r *RenderEditable) SetComposing(start, end int) {
	if start < 0 {
		start, end = -1, -1
	}
	if r.ComposeStart != start || r.ComposeEnd != end {
		r.ComposeStart, r.ComposeEnd = start, end
		MarkNeedsPaint(r)
	}
}

func (r *RenderEditable) composing() bool {
	return r.ComposeStart >= 0 && r.ComposeEnd > r.ComposeStart
}

func (r *RenderEditable) PerformLayout(c geom.Constraints) geom.Size {
	r.wrapW = 0
	if r.Wrap && c.HasBoundedWidth() {
		r.wrapW = max(c.MaxW-2, 1)
	}
	r.relayoutText()
	InvalidatePaint(r) // lines may wrap differently even at the same size
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

// Lines returns the number of visual lines (after wrapping).
func (r *RenderEditable) Lines() int {
	r.relayoutText()
	return len(r.lines)
}

// lineOf returns the visual line containing byte offset i. At a soft wrap
// the offset belongs to the next line (the caret shows at its start).
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
	if len(r.lines) != 1 || r.size.W <= 0 || r.Wrap {
		r.scrollX = 0
		return
	}
	_, x := r.xOf(r.SelExtent)
	var full float32
	for _, st := range r.lines[0].stops {
		full = max(full, st)
	}
	view := r.size.W - 2
	if x-r.scrollX > view {
		r.scrollX = x - view
	}
	if x-r.scrollX < 0 {
		r.scrollX = x
	}
	r.scrollX = max(0, min(r.scrollX, max(0, full-view)))
}

// CaretRect returns the caret's rectangle in local coordinates (e.g. to
// place the IME candidate window next to it).
func (r *RenderEditable) CaretRect() geom.Rect {
	r.relayoutText()
	if len(r.lines) == 0 {
		return geom.Rect{W: 2, H: r.lineH}
	}
	li, x := r.xOf(r.SelExtent)
	return geom.Rect{X: x - r.scrollX - 1, Y: float32(li) * r.lineH, W: 2, H: r.lineH}
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
	stops := l.stops
	// On a soft-wrapped line the last stop is the next line's start;
	// clicking past the end should stay on this line.
	if !l.hard && li < len(r.lines)-1 && len(stops) > 1 {
		stops = stops[:len(stops)-1]
	}
	best, bestD := 0, float32(1e9)
	for k, st := range stops {
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

// VerticalMove returns the offset one visual line up (dir<0) or down from
// i, keeping the x position; ok is false at the first/last line.
func (r *RenderEditable) VerticalMove(i, dir int) (int, bool) {
	r.relayoutText()
	li, x := r.xOf(i)
	nl := li + dir
	if nl < 0 || nl >= len(r.lines) {
		return i, false
	}
	return r.offsetInLine(nl, x), true
}

// LineBounds returns the byte range of the visual line containing i. For a
// soft-wrapped line the end excludes the trailing spaces the wrap swallows,
// so End keeps the caret on that line.
func (r *RenderEditable) LineBounds(i int) (start, end int) {
	r.relayoutText()
	li := r.lineOf(i)
	l := r.lines[li]
	end = l.start + len(l.s)
	if !l.hard && li < len(r.lines)-1 {
		trimmed := strings.TrimRight(l.s, " \t")
		if len(trimmed) < len(l.s) {
			end = l.start + len(trimmed)
		}
	}
	return l.start, end
}

func (r *RenderEditable) Paint(ctx *PaintContext, o geom.Offset) {
	c := ctx.Canvas
	rect := geom.RectFrom(o, r.size)
	c.PushClip(geom.Rect{X: rect.X - 1, Y: rect.Y, W: rect.W + 2, H: rect.H})
	lo, hi := min(r.SelBase, r.SelExtent), max(r.SelBase, r.SelExtent)
	composing := r.composing()
	for li, l := range r.lines {
		y := o.Y + float32(li)*r.lineH
		x0 := o.X - r.scrollX
		ls, le := l.start, l.start+len(l.s)
		if lo != hi && r.SelectionColor.A > 0 && !composing {
			s, e := max(lo, ls), min(hi, le)
			// The selection goes on past this visual line.
			continues := hi > le && li < len(r.lines)-1
			if l.para != nil && l.para.HasRTL() && s < e {
				// Mixed directions: the selection can be several boxes.
				for _, rc := range l.para.SelectionRects(s-ls, e-ls) {
					c.FillRect(geom.Rect{X: x0 + rc.X, Y: y, W: rc.W, H: r.lineH}, r.SelectionColor)
				}
			} else if s < e || continues && lo <= le {
				sx, ex := r.stopAt(l, s), r.stopAt(l, e)
				if continues {
					ex = l.stops[len(l.stops)-1]
					if l.hard {
						ex += r.lineH * 0.3 // show the selected newline
					}
				}
				c.FillRect(geom.Rect{X: x0 + sx, Y: y, W: ex - sx, H: r.lineH}, r.SelectionColor)
			}
		}
		c.DrawParagraph(l.para, geom.Offset{X: x0, Y: y})
		// IME preedit: underline the part of it on this line.
		if composing {
			s, e := max(r.ComposeStart, ls), min(r.ComposeEnd, le)
			if s < e {
				sx := r.stopAt(l, s)
				ex := r.stopAt(l, e)
				thick := max(1, r.lineH/14)
				col := r.CursorColor
				if col.A <= 0 {
					col = r.Style.Resolved().Color
				}
				c.FillRect(geom.Rect{X: x0 + sx, Y: y + r.lineH - thick - 1, W: ex - sx, H: thick}, col)
			}
		}
	}
	if r.Focused && r.CaretVisible && r.CursorColor.A > 0 && len(r.lines) > 0 {
		cr := r.CaretRect()
		c.FillRect(geom.Rect{X: o.X + cr.X, Y: o.Y + cr.Y, W: cr.W, H: cr.H}, r.CursorColor)
	}
	c.PopClip()
}

// stopAt returns the x of byte offset i inside line l.
func (r *RenderEditable) stopAt(l editLine, i int) float32 {
	rel := i - l.start
	for k, off := range l.offsets {
		if off >= rel {
			return l.stops[k]
		}
	}
	return l.stops[len(l.stops)-1]
}

func (r *RenderEditable) VisitChildren(func(RenderObject)) {}
func (r *RenderEditable) HitTestSelf(geom.Offset) bool     { return true }
func (r *RenderEditable) Cursor() Cursor                   { return CursorText }
