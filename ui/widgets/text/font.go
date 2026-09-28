// Package text implements the text stack: font loading, glyph metrics,
// paragraph layout (line breaking, alignment) and a GPU-agnostic glyph atlas
// that rasterizes glyph coverage masks on demand.
//
// It knows nothing about wgpu; the gpu package uploads the atlas pixels and
// turns laid-out glyphs into textured quads.
package text

import (
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// GlyphID is an index into a font's glyph table.
type GlyphID = sfnt.GlyphIndex

var nextFontID atomic.Uint32

// Font is a parsed TrueType/OpenType font plus metric caches.
//
// A Font is not safe for concurrent use; the whole UI runs on one goroutine.
type Font struct {
	id   uint32
	name string
	sf   *sfnt.Font
	buf  sfnt.Buffer
	upem float32

	// Font-unit metrics (scaled by size/upem when used).
	ascent, descent, lineGap float32

	glyphs   map[rune]GlyphID
	advances map[GlyphID]float32
	kerns    map[[2]GlyphID]float32
}

// ParseFont parses TTF/OTF data.
func ParseFont(name string, data []byte) (*Font, error) {
	sf, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("text: parse font %q: %w", name, err)
	}
	f := &Font{
		id:       nextFontID.Add(1),
		name:     name,
		sf:       sf,
		upem:     float32(sf.UnitsPerEm()),
		glyphs:   make(map[rune]GlyphID),
		advances: make(map[GlyphID]float32),
		kerns:    make(map[[2]GlyphID]float32),
	}
	// Querying metrics with ppem == upem gives values in font units (26.6).
	m, err := sf.Metrics(&f.buf, f.unitPPEM(), font.HintingNone)
	if err != nil {
		return nil, fmt.Errorf("text: metrics for %q: %w", name, err)
	}
	f.ascent = fx(m.Ascent)
	f.descent = fx(m.Descent)
	f.lineGap = max(0, fx(m.Height)-f.ascent-f.descent)
	return f, nil
}

// ID uniquely identifies the font inside this process (used as atlas key).
func (f *Font) ID() uint32 { return f.id }

// Name returns the name given to ParseFont.
func (f *Font) Name() string { return f.name }

func (f *Font) unitPPEM() fixed.Int26_6 { return fixed.Int26_6(int32(f.upem) << 6) }

func (f *Font) scale(size float32) float32 { return size / f.upem }

// Glyph maps a rune to a glyph index (0 = .notdef when missing).
func (f *Font) Glyph(r rune) GlyphID {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	g, err := f.sf.GlyphIndex(&f.buf, r)
	if err != nil {
		g = 0
	}
	f.glyphs[r] = g
	return g
}

// HasGlyph reports whether the font can render r.
func (f *Font) HasGlyph(r rune) bool { return f.Glyph(r) != 0 }

// Advance returns the horizontal advance of g at the given size in pixels.
func (f *Font) Advance(g GlyphID, size float32) float32 {
	adv, ok := f.advances[g]
	if !ok {
		a, err := f.sf.GlyphAdvance(&f.buf, g, f.unitPPEM(), font.HintingNone)
		if err == nil {
			adv = fx(a)
		}
		f.advances[g] = adv
	}
	return adv * f.scale(size)
}

// Kern returns the kerning adjustment between a and b at the given size.
func (f *Font) Kern(a, b GlyphID, size float32) float32 {
	key := [2]GlyphID{a, b}
	k, ok := f.kerns[key]
	if !ok {
		// ErrNotFound just means "no kerning for this pair".
		if v, err := f.sf.Kern(&f.buf, a, b, f.unitPPEM(), font.HintingNone); err == nil {
			k = fx(v)
		}
		f.kerns[key] = k
	}
	return k * f.scale(size)
}

// Metrics are vertical font metrics at a given size, in pixels.
type Metrics struct {
	Ascent  float32 // distance from baseline to top (positive)
	Descent float32 // distance from baseline to bottom (positive)
	LineGap float32
}

// LineHeight is the default distance between baselines.
func (m Metrics) LineHeight() float32 { return m.Ascent + m.Descent + m.LineGap }

// Metrics returns the vertical metrics at size.
func (f *Font) Metrics(size float32) Metrics {
	s := f.scale(size)
	return Metrics{Ascent: f.ascent * s, Descent: f.descent * s, LineGap: f.lineGap * s}
}

// outline loads the glyph outline at ppem pixels (y-down, origin on the
// baseline). The returned segments are only valid until the next call.
func (f *Font) outline(g GlyphID, ppem float32) (sfnt.Segments, error) {
	return f.sf.LoadGlyph(&f.buf, g, fixed.Int26_6(ppem*64+0.5), nil)
}

func fx(v fixed.Int26_6) float32 { return float32(v) / 64 }

// --- Built-in fonts --------------------------------------------------------

var (
	defaultOnce sync.Once
	defaultReg  *Font
	defaultBold *Font
)

func loadDefaults() {
	var err error
	if defaultReg, err = ParseFont("Go Regular", goregular.TTF); err != nil {
		panic(err)
	}
	if defaultBold, err = ParseFont("Go Bold", gobold.TTF); err != nil {
		panic(err)
	}
}

// DefaultFont returns the embedded Go Regular font (Latin, Cyrillic, Greek).
func DefaultFont() *Font { defaultOnce.Do(loadDefaults); return defaultReg }

// DefaultBoldFont returns the embedded Go Bold font.
func DefaultBoldFont() *Font { defaultOnce.Do(loadDefaults); return defaultBold }
