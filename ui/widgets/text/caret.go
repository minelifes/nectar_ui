package text

import "unicode/utf8"

// CaretStops returns the x position of every caret stop in a single line of
// text: stops[i] is the pen position before the i-th rune, and the last
// entry is the full advance width. offsets[i] is the matching byte offset.
// It uses the same shaping as Layout, so carets line up with drawn glyphs.
func CaretStops(line string, style Style) (stops []float32, offsets []int) {
	style = style.Resolved()
	cl := shape(style.Font, style.Size, style.LetterSpacing, line, 0, nil)
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
