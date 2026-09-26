package text

import (
	"math"
	"strings"
	"unicode"

	"nectar_ui/ui/geom"
)

// DefaultSize is used when Style.Size is zero.
const DefaultSize float32 = 14

// Style describes how a run of text looks.
type Style struct {
	Font          *Font      // nil = DefaultFont()
	Size          float32    // font size in logical pixels; 0 = DefaultSize
	Color         geom.Color // zero value = opaque black
	LineHeight    float32    // multiple of Size; 0 = font's natural line height
	LetterSpacing float32    // extra space after each glyph, in logical pixels
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
}

// Line is one visual line of a paragraph.
type Line struct {
	First, Last int     // glyph range [First, Last)
	X           float32 // horizontal offset applied by alignment
	Top         float32
	Baseline    float32
	Width       float32 // advance width, trailing spaces excluded
	Height      float32
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
}

// Layout shapes and wraps s. Shaping is simple (cmap + kerning), which is
// correct for Latin, Cyrillic, Greek, CJK and other scripts that don't need
// contextual shaping.
func Layout(s string, style Style, opt Options) *Paragraph {
	style = style.Resolved()
	f, size := style.Font, style.Size
	m := f.Metrics(size)

	lineH := m.LineHeight()
	lead := m.LineGap / 2
	if style.LineHeight > 0 {
		lineH = style.LineHeight * size
		lead = (lineH - m.Ascent - m.Descent) / 2
	}
	maxW := opt.MaxWidth
	if maxW <= 0 || math.IsInf(float64(maxW), 1) {
		maxW = float32(math.Inf(1))
	}

	p := &Paragraph{Style: style}
	var y float32

	addLine := func(cl []cluster, forceEllipsis bool) {
		first := len(p.Glyphs)
		// Trailing whitespace doesn't count toward width.
		end := len(cl)
		for end > 0 && cl[end-1].space {
			end--
		}
		if forceEllipsis && opt.Ellipsis != "" {
			cl = withEllipsis(f, size, style.LetterSpacing, cl[:end], opt.Ellipsis, maxW)
			end = len(cl)
		}
		baseline := y + lead + m.Ascent
		var x, width float32
		for i, c := range cl {
			if i > 0 {
				x += c.kern
			}
			if !c.space {
				p.Glyphs = append(p.Glyphs, Glyph{ID: c.g, X: x, Y: baseline, Cluster: c.byteI})
			}
			x += c.adv
			if i == end-1 {
				width = x
			}
		}
		p.Lines = append(p.Lines, Line{
			First: first, Last: len(p.Glyphs),
			Top: y, Baseline: baseline, Width: width, Height: lineH,
		})
		p.Width = max(p.Width, width)
		y += lineH
	}
	full := func() bool { return opt.MaxLines > 0 && len(p.Lines) >= opt.MaxLines }

	paragraphs := strings.Split(s, "\n")
	var cl []cluster
	for pi, para := range paragraphs {
		if full() {
			p.Truncated = true
			break
		}
		base := 0
		for i := 0; i < pi; i++ {
			base += len(paragraphs[i]) + 1
		}
		cl = shape(f, size, style.LetterSpacing, para, base, cl[:0])
		if len(cl) == 0 {
			addLine(nil, false)
			continue
		}

		start := 0
		for start < len(cl) {
			if full() {
				p.Truncated = true
				break
			}
			end, next := breakLine(cl[start:], maxW)
			last := opt.MaxLines > 0 && len(p.Lines) == opt.MaxLines-1
			more := start+next < len(cl) || pi < len(paragraphs)-1
			if last && more {
				// Last allowed line: take the rest of the paragraph and let the
				// ellipsis logic trim it to fit.
				addLine(cl[start:], true)
				p.Truncated = true
				break
			}
			addLine(cl[start:start+end], false)
			start += next
		}
		if p.Truncated {
			break
		}
	}
	p.Height = y
	return p
}

// shape converts runes to glyph clusters with advances and kerning.
func shape(f *Font, size, spacing float32, s string, base int, out []cluster) []cluster {
	var prev GlyphID
	for i, r := range s {
		if r == '\t' {
			r = ' '
		}
		g := f.Glyph(r)
		c := cluster{r: r, g: g, adv: f.Advance(g, size) + spacing, space: unicode.IsSpace(r), byteI: base + i}
		if len(out) > 0 {
			c.kern = f.Kern(prev, g, size)
		}
		prev = g
		out = append(out, c)
	}
	return out
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
func withEllipsis(f *Font, size, spacing float32, cl []cluster, ell string, maxW float32) []cluster {
	ec := shape(f, size, spacing, ell, -1, nil)
	var ew float32
	for _, c := range ec {
		ew += c.adv + c.kern
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
