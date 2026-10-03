package text

import (
	"math"
	"strings"
	"unicode"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// DefaultSize is used when Style.Size is zero.
const DefaultSize float32 = 14

// Decoration is a set of lines drawn with text.
type Decoration uint8

const (
	Underline Decoration = 1 << iota
	Strikethrough
	Overline
)

// Style describes how a run of text looks.
type Style struct {
	Font          *Font      // nil = DefaultFont()
	Size          float32    // font size in logical pixels; 0 = DefaultSize
	Color         geom.Color // zero value = opaque black
	LineHeight    float32    // multiple of Size; 0 = font's natural line height
	LetterSpacing float32    // extra space after each glyph, in logical pixels

	// Background fills the text's box (a highlight); zero = none.
	Background geom.Color
	// Decoration draws underlines, strikethroughs and overlines.
	Decoration Decoration
	// DecorationColor colors the decoration lines; zero = the text color.
	DecorationColor geom.Color
	// TabSize is the distance between tab stops in spaces; 0 draws a tab
	// as one space. Stops are measured from the start of each hard line.
	TabSize int
}

// Resolved fills in defaults.
func (s Style) Resolved() Style {
	if s.Font == nil {
		s.Font = DefaultFont()
	}
	if s.Size <= 0 {
		s.Size = DefaultSize
	}
	if s.Color == (geom.Color{}) {
		s.Color = geom.Black
	}
	return s
}

// Inherit returns s with its unset (zero) fields taken from base.
func (s Style) Inherit(base Style) Style {
	if s.Font == nil {
		s.Font = base.Font
	}
	if s.Size == 0 {
		s.Size = base.Size
	}
	if s.Color == (geom.Color{}) {
		s.Color = base.Color
	}
	if s.LineHeight == 0 {
		s.LineHeight = base.LineHeight
	}
	if s.LetterSpacing == 0 {
		s.LetterSpacing = base.LetterSpacing
	}
	if s.Background == (geom.Color{}) {
		s.Background = base.Background
	}
	if s.Decoration == 0 {
		s.Decoration = base.Decoration
	}
	if s.DecorationColor == (geom.Color{}) {
		s.DecorationColor = base.DecorationColor
	}
	if s.TabSize == 0 {
		s.TabSize = base.TabSize
	}
	return s
}

// Span is a run of text with its own style. Unset fields of Style inherit
// from the paragraph's base style (LineHeight and TabSize are always the
// base's: they belong to lines, not runs).
type Span struct {
	Text  string
	Style Style
}

// Align is horizontal alignment of lines inside the paragraph box.
type Align uint8

const (
	AlignStart Align = iota
	AlignCenter
	AlignEnd
)

// Glyph is a positioned glyph. X is relative to the start of its line, Y is
// the baseline relative to the top of the paragraph.
type Glyph struct {
	ID      GlyphID
	X, Y    float32
	Cluster int // byte offset of the source rune in the original string

	// Font and Size draw the glyph: the run's font, or the fallback that
	// has the rune. Color is set for paragraphs with several colors
	// (Paragraph.Rich).
	Font  *Font
	Size  float32
	Color geom.Color
}

// Line is one visual line of a paragraph.
type Line struct {
	First, Last int     // glyph range [First, Last)
	X           float32 // horizontal offset applied by alignment
	Top         float32
	Baseline    float32
	Width       float32 // advance width, trailing spaces excluded
	Height      float32

	// Start and End are the byte range of the source text on this line
	// (End excludes the '\n' and, for a soft wrap, includes the spaces
	// swallowed at the break).
	Start, End int
	// Stops[i] is the caret x (relative to the line, before X) in front of
	// the byte at Offsets[i]; the last entry is the end of the line.
	Stops   []float32
	Offsets []int
}

// DecorationRect is a background or a decoration line, relative to its
// line's origin (add the line's X, and the paragraph's origin).
type DecorationRect struct {
	Line  int
	Rect  geom.Rect
	Color geom.Color
	// Behind is true for backgrounds (painted under the glyphs).
	Behind bool
}

// Paragraph is the result of laying out a string.
type Paragraph struct {
	Style  Style
	Glyphs []Glyph
	Lines  []Line
	Width  float32 // widest line
	Height float32 // total height of all lines
	// Truncated reports whether MaxLines cut the text.
	Truncated bool
	// Rich is set when glyphs have their own colors (see Glyph.Color).
	Rich bool
	// Decorations are span backgrounds and decoration lines.
	Decorations []DecorationRect
	// Text is the laid-out source text (the spans joined).
	Text string
}

// Options control line breaking.
type Options struct {
	MaxWidth float32 // wrap width; <= 0 or +Inf = no wrapping
	MaxLines int     // 0 = unlimited
	Ellipsis string  // appended when truncated by MaxLines; "" = none
}

type cluster struct {
	r     rune
	g     GlyphID
	adv   float32
	kern  float32 // adjustment relative to the previous cluster
	space bool
	byteI int
	font  *Font // the face that draws g
	size  float32
	span  int // index into the resolved span styles
}

// run is one span's text with its resolved style.
type run struct {
	text  string
	start int // byte offset in the joined text
	style Style
}

// Layout shapes and wraps s. Shaping is simple (cmap + kerning), which is
// correct for Latin, Cyrillic, Greek, CJK and other scripts that don't need
// contextual shaping. Runes the font lacks come from its fallback chain
// (Font.WithFallback, SetGlobalFallback).
func Layout(s string, style Style, opt Options) *Paragraph {
	return LayoutSpans([]Span{{Text: s}}, style, opt)
}

// LayoutSpans lays out styled runs of text as one paragraph. Each span's
// unset style fields inherit from base.
func LayoutSpans(spans []Span, base Style, opt Options) *Paragraph {
	base = base.Resolved()
	runs := make([]run, 0, len(spans))
	var sb strings.Builder
	rich := false
	for _, sp := range spans {
		st := sp.Style.Inherit(base)
		st.LineHeight, st.TabSize = base.LineHeight, base.TabSize
		if st.Color != base.Color {
			rich = true
		}
		runs = append(runs, run{text: sp.Text, start: sb.Len(), style: st})
		sb.WriteString(sp.Text)
	}
	full := sb.String()
	styles := make([]Style, len(runs))
	for i, r := range runs {
		styles[i] = r.style
	}

	f, size := base.Font, base.Size
	m := f.Metrics(size)
	lineH := m.LineHeight()
	lead := m.LineGap / 2
	if base.LineHeight > 0 {
		lineH = base.LineHeight * size
		lead = (lineH - m.Ascent - m.Descent) / 2
	}
	maxW := opt.MaxWidth
	if maxW <= 0 || math.IsInf(float64(maxW), 1) {
		maxW = float32(math.Inf(1))
	}

	p := &Paragraph{Style: base, Rich: rich, Text: full}
	var y float32

	addLine := func(cl []cluster, forceEllipsis bool, start, end int) {
		first := len(p.Glyphs)
		// Trailing whitespace doesn't count toward width.
		vis := len(cl)
		for vis > 0 && cl[vis-1].space {
			vis--
		}
		if forceEllipsis && opt.Ellipsis != "" {
			cl = withEllipsis(base, cl[:vis], opt.Ellipsis, maxW)
			vis = len(cl)
		}
		// Taller runs (a bigger size, a fallback font) grow the line.
		asc, desc := m.Ascent, m.Descent
		for _, c := range cl {
			if c.font != f || c.size != size {
				cm := c.font.Metrics(c.size)
				asc, desc = max(asc, cm.Ascent), max(desc, cm.Descent)
			}
		}
		h := lineH + (asc - m.Ascent) + (desc - m.Descent)
		baseline := y + lead + asc
		li := len(p.Lines)
		var x, width float32
		stops := make([]float32, 0, len(cl)+1)
		offs := make([]int, 0, len(cl)+1)
		for i, c := range cl {
			if i > 0 {
				x += c.kern
			}
			if c.byteI >= 0 {
				stops = append(stops, x)
				offs = append(offs, c.byteI)
			}
			if !c.space {
				g := Glyph{ID: c.g, X: x, Y: baseline, Cluster: c.byteI, Font: c.font, Size: c.size}
				if rich {
					g.Color = styles[c.span].Color
				}
				p.Glyphs = append(p.Glyphs, g)
			}
			x += c.adv
			if i == vis-1 {
				width = x
			}
		}
		stops = append(stops, x)
		offs = append(offs, end)
		p.decorate(li, cl, styles, base, y, h, baseline)
		p.Lines = append(p.Lines, Line{
			First: first, Last: len(p.Glyphs),
			Top: y, Baseline: baseline, Width: width, Height: h,
			Start: start, End: end, Stops: stops, Offsets: offs,
		})
		p.Width = max(p.Width, width)
		y += h
	}
	isFull := func() bool { return opt.MaxLines > 0 && len(p.Lines) >= opt.MaxLines }

	paragraphs := strings.Split(full, "\n")
	var cl []cluster
	pstart := 0
	for pi, para := range paragraphs {
		if pi > 0 {
			pstart += len(paragraphs[pi-1]) + 1
		}
		if isFull() {
			p.Truncated = true
			break
		}
		cl = shapeRuns(runs, pstart, pstart+len(para), cl[:0])
		if len(cl) == 0 {
			addLine(nil, false, pstart, pstart)
			continue
		}

		start := 0
		for start < len(cl) {
			if isFull() {
				p.Truncated = true
				break
			}
			end, next := breakLine(cl[start:], maxW)
			last := opt.MaxLines > 0 && len(p.Lines) == opt.MaxLines-1
			more := start+next < len(cl) || pi < len(paragraphs)-1
			lineEnd := pstart + len(para)
			if start+next < len(cl) {
				lineEnd = cl[start+next].byteI
			}
			if last && more {
				// Last allowed line: take the rest of the paragraph and let the
				// ellipsis logic trim it to fit.
				addLine(cl[start:], true, cl[start].byteI, pstart+len(para))
				p.Truncated = true
				break
			}
			addLine(cl[start:start+end], false, cl[start].byteI, lineEnd)
			start += next
		}
		if p.Truncated {
			break
		}
	}
	p.Height = y
	return p
}

// decorate records the backgrounds and decoration lines of one line.
func (p *Paragraph) decorate(li int, cl []cluster, styles []Style, base Style, top, h, baseline float32) {
	if len(cl) == 0 {
		return
	}
	decorated := base.Background.A > 0 || base.Decoration != 0
	for _, st := range styles {
		if st.Background.A > 0 || st.Decoration != 0 {
			decorated = true
			break
		}
	}
	if !decorated {
		return
	}
	// Walk runs of clusters with the same span.
	var x float32
	i := 0
	for i < len(cl) {
		j := i
		x0 := x
		for j < len(cl) && cl[j].span == cl[i].span {
			if j > 0 {
				x += cl[j].kern
			}
			if j == i {
				x0 = x
			}
			x += cl[j].adv
			j++
		}
		st := base // the ellipsis
		if cl[i].span >= 0 {
			st = styles[cl[i].span]
		}
		w := x - x0
		if st.Background.A > 0 {
			p.Decorations = append(p.Decorations, DecorationRect{Line: li, Rect: geom.Rect{X: x0, Y: top, W: w, H: h}, Color: st.Background, Behind: true})
		}
		if st.Decoration != 0 {
			c := st.DecorationColor
			if c == (geom.Color{}) {
				c = st.Color
			}
			m := st.Font.Metrics(st.Size)
			th := max(1, st.Size/14)
			if st.Decoration&Underline != 0 {
				p.Decorations = append(p.Decorations, DecorationRect{Line: li, Rect: geom.Rect{X: x0, Y: baseline + max(1, m.Descent/3), W: w, H: th}, Color: c})
			}
			if st.Decoration&Strikethrough != 0 {
				p.Decorations = append(p.Decorations, DecorationRect{Line: li, Rect: geom.Rect{X: x0, Y: baseline - m.Ascent*0.3 - th/2, W: w, H: th}, Color: c})
			}
			if st.Decoration&Overline != 0 {
				p.Decorations = append(p.Decorations, DecorationRect{Line: li, Rect: geom.Rect{X: x0, Y: baseline - m.Ascent, W: w, H: th}, Color: c})
			}
		}
		i = j
	}
}

// shapeRuns shapes the bytes [from, to) of the joined runs.
func shapeRuns(runs []run, from, to int, out []cluster) []cluster {
	var prev cluster
	var x float32 // pen position from the start of the hard line (tab stops)
	for si, r := range runs {
		rs, re := max(from, r.start), min(to, r.start+len(r.text))
		if rs >= re {
			continue
		}
		st := r.style
		for i, ch := range r.text[rs-r.start : re-r.start] {
			c := shapeRune(st, ch, rs+i, x)
			c.span = si
			if len(out) > 0 && prev.font == c.font && prev.size == c.size {
				c.kern = c.font.Kern(prev.g, c.g, c.size)
			}
			out = append(out, c)
			prev = c
			x += c.kern + c.adv
		}
	}
	return out
}

// shapeRune makes the cluster of one rune at pen position x.
func shapeRune(st Style, r rune, byteI int, x float32) cluster {
	tab := r == '\t'
	if tab {
		r = ' '
	}
	f := st.Font.fontFor(r)
	face := f.Face()
	g := face.Glyph(r)
	adv := face.Advance(g, st.Size) + st.LetterSpacing
	if tab && st.TabSize > 0 {
		stop := adv * float32(st.TabSize)
		next := (float32(math.Floor(float64((x+0.01)/stop))) + 1) * stop
		adv = next - x
	}
	return cluster{r: r, g: g, adv: adv, space: unicode.IsSpace(r), byteI: byteI, font: face, size: st.Size}
}

// shape converts runes of one style to glyph clusters with advances and
// kerning.
func shape(st Style, s string, base int, out []cluster) []cluster {
	return shapeRuns([]run{{text: s, start: base, style: st}}, base, base+len(s), out)
}

// breakLine finds how many clusters fit on one line. It returns the visual end
// of the line and the index where the next line starts (they differ by the
// whitespace swallowed at a soft break).
func breakLine(cl []cluster, maxW float32) (end, next int) {
	var x float32
	lastBreak := -1 // index of the first cluster after a break opportunity
	for i, c := range cl {
		if i > 0 && !c.space && (cl[i-1].space || cl[i-1].r == '-') {
			lastBreak = i
		}
		w := c.adv
		if i > 0 {
			w += c.kern
		}
		if !c.space && x+w > maxW && i > 0 {
			if lastBreak > 0 {
				return lastBreak, lastBreak
			}
			return i, i // no break opportunity: break mid-word
		}
		x += w
	}
	return len(cl), len(cl)
}

// withEllipsis trims clusters from the end until the ellipsis fits in maxW.
func withEllipsis(base Style, cl []cluster, ell string, maxW float32) []cluster {
	ec := shape(base, ell, 0, nil)
	var ew float32
	for i := range ec {
		ec[i].byteI = -1 // not part of the source text
		ec[i].span = -1
		ew += ec[i].adv + ec[i].kern
	}
	width := func(n int) float32 {
		var x float32
		for i := 0; i < n; i++ {
			x += cl[i].adv
			if i > 0 {
				x += cl[i].kern
			}
		}
		return x
	}
	n := len(cl)
	for n > 0 && width(n)+ew > maxW {
		n--
	}
	for n > 0 && cl[n-1].space {
		n--
	}
	out := append([]cluster(nil), cl[:n]...)
	return append(out, ec...)
}

// SetAlign positions lines horizontally inside a box of the given width.
func (p *Paragraph) SetAlign(a Align, boxWidth float32) {
	for i := range p.Lines {
		l := &p.Lines[i]
		switch a {
		case AlignCenter:
			l.X = (boxWidth - l.Width) / 2
		case AlignEnd:
			l.X = boxWidth - l.Width
		default:
			l.X = 0
		}
	}
}

// Size returns the paragraph's content size, rounded up to whole pixels so
// layout stays on the pixel grid.
func (p *Paragraph) Size() geom.Size {
	return geom.Size{W: ceil(p.Width), H: ceil(p.Height)}
}

func ceil(v float32) float32 { return float32(math.Ceil(float64(v) - 1e-4)) }
