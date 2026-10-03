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

	// fallback fonts render the runes this one lacks (see WithFallback).
	fallback []*Font
	// face is the font whose glyphs these are: f itself, or for a font made
	// by WithFallback the font it was made from (glyph caches, atlas keys
	// and metrics are shared with it).
	face *Font
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
	f.face = f
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

// WithFallback returns f with a fallback chain: a rune f has no glyph for
// is drawn with the first of fonts that has one (an emoji, CJK or symbol
// font, say). The result shares f's glyphs, metrics and caches; line
// metrics stay f's. Fallbacks may have fallbacks of their own.
//
// Make the font once and keep it: styles compare fonts by pointer, so a
// new WithFallback on every build would re-lay out the text every frame.
func (f *Font) WithFallback(fonts ...*Font) *Font {
	c := *f
	c.fallback = append(append([]*Font(nil), f.fallback...), fonts...)
	return &c
}

// Fallback returns the fallback chain set with WithFallback.
func (f *Font) Fallback() []*Font { return f.fallback }

// Face returns the font whose glyph outlines f draws: f itself, or the
// font a WithFallback font was made from.
func (f *Font) Face() *Font {
	if f.face == nil {
		return f
	}
	return f.face
}

// fontFor returns the font (f, one of its fallbacks, or the global
// fallbacks) that draws r: the first one with a glyph for it, else f.
func (f *Font) fontFor(r rune) *Font {
	if r < 0x80 || f.HasGlyph(r) {
		return f
	}
	if g := findFallback(f.fallback, r, 0); g != nil {
		return g
	}
	if g := findFallback(GlobalFallback(), r, 0); g != nil {
		return g
	}
	return f
}

func findFallback(fonts []*Font, r rune, depth int) *Font {
	if depth > 4 {
		return nil
	}
	for _, c := range fonts {
		if c.HasGlyph(r) {
			return c.Face()
		}
		if g := findFallback(c.fallback, r, depth+1); g != nil {
			return g
		}
	}
	return nil
}

var (
	globalMu       sync.Mutex
	globalFallback []*Font
)

// SetGlobalFallback sets the fonts every font falls back to after its own
// chain: the place for an app-wide emoji or CJK font. Call it at startup.
func SetGlobalFallback(fonts ...*Font) {
	globalMu.Lock()
	globalFallback = append([]*Font(nil), fonts...)
	globalMu.Unlock()
}

// GlobalFallback returns the fonts set with SetGlobalFallback.
func GlobalFallback() []*Font {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalFallback
}

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
