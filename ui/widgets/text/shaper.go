package text

import (
	"bytes"
	"math"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	gtfont "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// Complex shaping: text that needs more than one glyph per character
// (Arabic joining, Indic reordering, combining marks, emoji sequences),
// right-to-left text, and fonts with ligatures go through HarfBuzz (the
// pure-Go port in go-text/typesetting). Everything else keeps the simple
// cmap + kerning path.

// hbState is a font's HarfBuzz face, parsed on first use.
type hbState struct {
	once sync.Once
	face *gtfont.Face
	liga bool // the font substitutes ligatures (liga, clig, calt, rlig)
}

func (f *Font) harfbuzz() *hbState {
	face := f.Face()
	hs := face.hb
	if hs == nil {
		return &hbState{}
	}
	hs.once.Do(func() {
		if len(face.data) == 0 {
			return
		}
		ft, err := gtfont.ParseTTF(bytes.NewReader(face.data))
		if err != nil {
			return
		}
		hs.face = ft
		for _, feat := range ft.GSUB.Features {
			switch feat.Tag.String() {
			case "liga", "clig", "calt", "rlig":
				hs.liga = true
			}
		}
	})
	return hs
}

// complexRune reports whether r needs contextual shaping or is a bidi
// control or joiner.
func complexRune(r rune) bool {
	switch {
	case r < 0x0300:
		return false
	case r < 0x0370: // combining diacritics
		return true
	case r >= 0x0590 && r <= 0x08FF: // Hebrew, Arabic, Syriac, Thaana, NKo, ...
		return true
	case r >= 0x0900 && r <= 0x0DFF: // Indic
		return true
	case r >= 0x0E00 && r <= 0x0FFF: // Thai, Lao, Tibetan
		return true
	case r >= 0x1000 && r <= 0x109F, r >= 0x1780 && r <= 0x18AF, r >= 0x1A00 && r <= 0x1B7F:
		return true
	case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r >= 0xA800 && r <= 0xABFF, r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE00 && r <= 0xFE0F, r >= 0xFE20 && r <= 0xFE2F, r >= 0xFE70 && r <= 0xFEFF:
		return true
	case r >= 0x10A00 && r <= 0x10FFF, r >= 0x11000 && r <= 0x11FFF:
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF, r >= 0xE0100 && r <= 0xE01EF:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me)
}

// needsComplex reports whether the bytes [from, to) need HarfBuzz.
func needsComplex(runs []run, from, to int) bool {
	for _, r := range runs {
		rs, re := max(from, r.start), min(to, r.start+len(r.text))
		if rs >= re {
			continue
		}
		if !r.style.NoLigatures && r.style.Font.harfbuzz().liga {
			return true
		}
		for _, ch := range r.text[rs-r.start : re-r.start] {
			if complexRune(ch) {
				return true
			}
		}
	}
	return false
}

var shapers = sync.Pool{New: func() any { return &shaping.HarfbuzzShaper{} }}

var noLiga = []shaping.FontFeature{
	{Tag: ot.MustNewTag("liga"), Value: 0},
	{Tag: ot.MustNewTag("clig"), Value: 0},
	{Tag: ot.MustNewTag("calt"), Value: 0},
}

// item is a piece of a line shaped in one go: same span, font, script and
// direction.
type item struct {
	start, end int // rune indices
	span       int
	font       *Font
	level      uint8
	script     language.Script
}

// shapeComplex shapes one hard line with bidi analysis and HarfBuzz.
func shapeComplex(runs []run, from, to int, out []cluster) ([]cluster, bool) {
	var rs []rune
	var byteAt []int // byte offset of each rune (and the end)
	var spanOf []int
	for si, r := range runs {
		a, b := max(from, r.start), min(to, r.start+len(r.text))
		if a >= b {
			continue
		}
		for i, ch := range r.text[a-r.start : b-r.start] {
			rs = append(rs, ch)
			byteAt = append(byteAt, a+i)
			spanOf = append(spanOf, si)
		}
	}
	byteAt = append(byteAt, to)
	if len(rs) == 0 {
		return out, false
	}

	// Bidi levels per rune.
	levels := make([]uint8, len(rs))
	var para bidi.Paragraph
	bruns := para.Segment(rs, bidi.Neutral)
	rtl := false
	for i := 0; i < bruns.NumRuns(); i++ {
		r := bruns.Run(i)
		for k := r.Start; k < r.End && k < len(rs); k++ {
			levels[k] = uint8(r.Level)
		}
	}
	for i, ch := range rs {
		if d := strongDir(ch); d != 0 {
			rtl = d < 0
			break
		}
		_ = i
	}

	// Items.
	var items []item
	for i, ch := range rs {
		st := runs[spanOf[i]].style
		fnt := st.Font.fontFor(ch)
		sc := language.LookupScript(ch)
		if n := len(items); n > 0 {
			it := &items[n-1]
			weak := sc == language.Common || sc == language.Inherited || sc == language.Unknown
			sameFont := fnt.Face() == it.font.Face() || (weak && it.font.Face().HasGlyph(ch)) || unicode.Is(unicode.Mn, ch)
			if it.span == spanOf[i] && it.level == levels[i] && sameFont && (weak || it.script == sc || it.script == language.Common) {
				if it.script == language.Common && !weak {
					it.script = sc
				}
				it.end = i + 1
				continue
			}
		}
		items = append(items, item{start: i, end: i + 1, span: spanOf[i], font: fnt.Face(), level: levels[i], script: sc})
	}

	sh := shapers.Get().(*shaping.HarfbuzzShaper)
	defer shapers.Put(sh)
	// Tabs shape as spaces; their width is set below.
	text := make([]rune, len(rs))
	for i, ch := range rs {
		if ch == '\t' {
			ch = ' '
		}
		text[i] = ch
	}
	for _, it := range items {
		st := runs[it.span].style
		hs := it.font.harfbuzz()
		if hs.face == nil {
			// Not parseable for HarfBuzz: one glyph per rune.
			for i := it.start; i < it.end; i++ {
				c := shapeRune(st, rs[i], byteAt[i], 0)
				c.span, c.level = it.span, it.level
				out = append(out, c)
			}
			continue
		}
		dir := di.DirectionLTR
		if it.level%2 == 1 {
			dir = di.DirectionRTL
		}
		in := shaping.Input{Text: text, RunStart: it.start, RunEnd: it.end, Direction: dir, Face: hs.face,
			Size: fixed.Int26_6(st.Size*64 + 0.5), Script: it.script}
		if st.NoLigatures {
			in.FontFeatures = noLiga
		}
		res := sh.Shape(in)
		out = appendShaped(out, res, rs, byteAt, it, st)
	}

	// Tab stops (measured along the logical line).
	var x float32
	for i := range out {
		c := &out[i]
		if c.byteI >= from && c.byteI < to && c.r == '\t' {
			st := runs[c.span].style
			sp := c.font.Advance(c.font.Glyph(' '), c.size) + st.LetterSpacing
			c.g, c.more = c.font.Glyph(' '), nil
			if st.TabSize > 0 {
				stop := sp * float32(st.TabSize)
				c.adv = (float32(math.Floor(float64((x+0.01)/stop)))+1)*stop - x
			} else {
				c.adv = sp
			}
		}
		x += c.adv
	}
	return out, rtl
}

// appendShaped turns HarfBuzz output into clusters in logical order.
func appendShaped(out []cluster, res shaping.Output, rs []rune, byteAt []int, it item, st Style) []cluster {
	type group struct {
		ci    int
		first int
		n     int
	}
	var groups []group
	for i, g := range res.Glyphs {
		if n := len(groups); n > 0 && groups[n-1].ci == g.ClusterIndex {
			groups[n-1].n++
			continue
		}
		groups = append(groups, group{ci: g.ClusterIndex, first: i, n: 1})
	}
	if it.level%2 == 1 {
		for a, b := 0, len(groups)-1; a < b; a, b = a+1, b-1 {
			groups[a], groups[b] = groups[b], groups[a]
		}
	}
	px := func(v fixed.Int26_6) float32 { return float32(v) / 64 }
	for k, gr := range groups {
		end := it.end
		if k+1 < len(groups) {
			end = groups[k+1].ci
		}
		if end <= gr.ci { // malformed order: treat as one rune
			end = gr.ci + 1
		}
		ch := rs[gr.ci]
		c := cluster{r: ch, space: unicode.IsSpace(ch), byteI: byteAt[gr.ci], byteEnd: byteAt[min(end, len(byteAt)-1)],
			runes: end - gr.ci, font: it.font, size: st.Size, span: it.span, level: it.level}
		var pen float32
		for j := 0; j < gr.n; j++ {
			g := res.Glyphs[gr.first+j]
			gx, gy := pen+px(g.XOffset), -px(g.YOffset)
			if j == 0 {
				c.g, c.gx, c.gy = GlyphID(g.GlyphID), gx, gy
			} else {
				c.more = append(c.more, clusterGlyph{id: GlyphID(g.GlyphID), x: gx, y: gy})
			}
			pen += px(g.XAdvance)
		}
		c.adv = pen + st.LetterSpacing*float32(c.runes)
		out = append(out, c)
	}
	return out
}

// strongDir returns 1 for a strong left-to-right character, -1 for a strong
// right-to-left one, 0 otherwise.
func strongDir(r rune) int {
	switch {
	case r >= 0x0590 && r <= 0x08FF, r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF, r >= 0x10800 && r <= 0x10FFF, r >= 0x1E800 && r <= 0x1EFFF:
		if unicode.IsLetter(r) {
			return -1
		}
	case unicode.IsLetter(r):
		return 1
	}
	return 0
}
