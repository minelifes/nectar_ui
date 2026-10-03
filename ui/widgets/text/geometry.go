package text

import (
	"unicode"
	"unicode/utf8"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// LineAt returns the index of the line at y (clamped to the first / last
// line).
func (p *Paragraph) LineAt(y float32) int {
	for i, l := range p.Lines {
		if y < l.Top+l.Height {
			return i
		}
	}
	return max(len(p.Lines)-1, 0)
}

// OffsetAt returns the byte offset of the caret position closest to pos
// (relative to the paragraph's top-left, after SetAlign).
func (p *Paragraph) OffsetAt(pos geom.Offset) int {
	if len(p.Lines) == 0 {
		return 0
	}
	l := p.Lines[p.LineAt(pos.Y)]
	x := pos.X - l.X
	best, bestD := l.End, float32(-1)
	for i, sx := range l.Stops {
		d := sx - x
		if d < 0 {
			d = -d
		}
		if bestD < 0 || d < bestD {
			best, bestD = l.Offsets[i], d
		}
	}
	return best
}

// CaretAt returns the line and the x (relative to the paragraph, after
// SetAlign) of the caret in front of byte offset i. Offsets inside a
// line's swallowed trailing spaces map to the line's end.
func (p *Paragraph) CaretAt(i int) (line int, x float32) {
	for li, l := range p.Lines {
		last := li == len(p.Lines)-1
		if i < l.Start || (i > l.End && !last) {
			continue
		}
		// A soft-wrapped line ends where the next begins: that offset is
		// the next line's start.
		if i == l.End && !last && p.Lines[li+1].Start == l.End {
			continue
		}
		for k, off := range l.Offsets {
			if off >= i {
				return li, l.X + l.Stops[k]
			}
		}
		return li, l.X + l.Stops[len(l.Stops)-1]
	}
	if len(p.Lines) == 0 {
		return 0, 0
	}
	li := len(p.Lines) - 1
	l := p.Lines[li]
	return li, l.X + l.Stops[len(l.Stops)-1]
}

// CaretRect returns the caret box in front of byte offset i (width 0).
func (p *Paragraph) CaretRect(i int) geom.Rect {
	if len(p.Lines) == 0 {
		return geom.Rect{}
	}
	li, x := p.CaretAt(i)
	l := p.Lines[li]
	return geom.Rect{X: x, Y: l.Top, H: l.Height}
}

// SelectionRects returns the boxes covering the text between byte offsets
// a and b (in any order), one per line.
func (p *Paragraph) SelectionRects(a, b int) []geom.Rect {
	if a > b {
		a, b = b, a
	}
	if a == b || len(p.Lines) == 0 {
		return nil
	}
	var out []geom.Rect
	for li, l := range p.Lines {
		if l.End < a || l.Start > b || (l.End == a && li < len(p.Lines)-1) {
			continue
		}
		x0 := l.X + l.Stops[0]
		if a > l.Start {
			_, x0 = p.caretInLine(li, a)
		}
		x1 := l.X + l.Stops[len(l.Stops)-1]
		if b < l.End {
			_, x1 = p.caretInLine(li, b)
		} else if li < len(p.Lines)-1 && b > l.End {
			x1 += 4 // the selection runs on past the line break
		}
		if x1 > x0 {
			out = append(out, geom.Rect{X: x0, Y: l.Top, W: x1 - x0, H: l.Height})
		}
	}
	return out
}

func (p *Paragraph) caretInLine(li, i int) (int, float32) {
	l := p.Lines[li]
	for k, off := range l.Offsets {
		if off >= i {
			return li, l.X + l.Stops[k]
		}
	}
	return li, l.X + l.Stops[len(l.Stops)-1]
}

// WordAt returns the byte range of the word (a run of letters, digits and
// '_') around offset i; for other characters, the range of that one rune.
func WordAt(s string, i int) (start, end int) {
	if i < 0 {
		i = 0
	}
	if i > len(s) {
		i = len(s)
	}
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	r, _ := utf8.DecodeRuneInString(s[i:])
	if i == len(s) || !isWord(r) {
		if i > 0 {
			if pr, _ := utf8.DecodeLastRuneInString(s[:i]); isWord(pr) {
				return WordAt(s, PrevRune(s, i))
			}
		}
		return i, NextRune(s, i)
	}
	start, end = i, i
	for start > 0 {
		pr, n := utf8.DecodeLastRuneInString(s[:start])
		if !isWord(pr) {
			break
		}
		start -= n
	}
	for end < len(s) {
		nr, n := utf8.DecodeRuneInString(s[end:])
		if !isWord(nr) {
			break
		}
		end += n
	}
	return start, end
}
