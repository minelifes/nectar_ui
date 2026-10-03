package text

import (
	"math"
	"unicode/utf8"
)

// CaretStops returns the x position of every caret stop in a single line of
// text: stops[i] is the pen position before the i-th rune, and the last
// entry is the full advance width. offsets[i] is the matching byte offset.
// It uses the same shaping as Layout, so carets line up with drawn glyphs.
//
// For right-to-left or mixed text the stops aren't in ascending order (each
// sits at its character's leading edge).
func CaretStops(line string, style Style) (stops []float32, offsets []int) {
	style = style.Resolved()
	if needsComplex([]run{{text: line, style: style}}, 0, len(line)) {
		p := Layout(line, style, Options{})
		if len(p.Lines) == 0 {
			return []float32{0}, []int{len(line)}
		}
		return p.Lines[0].Stops, p.Lines[0].Offsets
	}
	cl := shape(style, line, 0, nil)
	stops = make([]float32, 0, len(cl)+1)
	offsets = make([]int, 0, len(cl)+1)
	var x float32
	for i, c := range cl {
		if i > 0 {
			x += c.kern
		}
		stops = append(stops, x)
		offsets = append(offsets, c.byteI)
		x += c.adv
	}
	stops = append(stops, x)
	offsets = append(offsets, len(line))
	return stops, offsets
}

// LineMetrics returns the line height and baseline offset for style, the
// same values Layout uses.
func LineMetrics(style Style) (height, baseline float32) {
	style = style.Resolved()
	m := style.Font.Metrics(style.Size)
	h := m.LineHeight()
	lead := m.LineGap / 2
	if style.LineHeight > 0 {
		h = style.LineHeight * style.Size
		lead = (h - m.Ascent - m.Descent) / 2
	}
	return h, lead + m.Ascent
}

// PrevRune / NextRune step a byte offset by one rune.
func PrevRune(s string, i int) int {
	if i <= 0 {
		return 0
	}
	_, n := utf8.DecodeLastRuneInString(s[:i])
	return i - n
}

func NextRune(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	_, n := utf8.DecodeRuneInString(s[i:])
	return i + n
}

// LineBreaks returns the byte offsets where each visual line of line (a
// single hard line, no '\n') starts when wrapped at maxW, using the same
// rules as Layout: break after spaces or '-', mid-word only when a word is
// wider than maxW. The first entry is always 0. maxW <= 0 or +Inf means no
// wrapping.
func LineBreaks(line string, style Style, maxW float32) []int {
	breaks := []int{0}
	if maxW <= 0 || math.IsInf(float64(maxW), 1) || line == "" {
		return breaks
	}
	style = style.Resolved()
	cl := shape(style, line, 0, nil)
	for start := 0; start < len(cl); {
		_, next := breakLine(cl[start:], maxW)
		if next <= 0 {
			next = 1
		}
		start += next
		if start < len(cl) {
			breaks = append(breaks, cl[start].byteI)
		}
	}
	return breaks
}
