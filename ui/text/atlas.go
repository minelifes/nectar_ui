package text

import (
	"image"
	"math"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/vector"
)

// SubpixelSteps is the number of horizontal subpixel positions a glyph is
// rasterized at. 4 gives visibly better spacing than whole-pixel snapping at
// the cost of up to 4 atlas entries per glyph.
const SubpixelSteps = 4

// GlyphKey identifies one rasterized glyph bitmap.
type GlyphKey struct {
	Font  uint32
	Glyph GlyphID
	Size  uint32 // pixel size * 64 (26.6), physical pixels
	SubX  uint8  // subpixel bin 0..SubpixelSteps-1
}

// AtlasGlyph describes where a glyph bitmap lives in the atlas.
type AtlasGlyph struct {
	X, Y, W, H int     // rectangle in the atlas, in texels
	OffX, OffY float32 // bitmap top-left relative to the (snapped) pen position
	Empty      bool    // nothing to draw (e.g. space)
}

// Atlas is a CPU-side RGBA glyph atlas with a simple shelf packer.
//
// Pixels are stored as white RGB with coverage in alpha, so the same texture
// can later hold color bitmaps (emoji, icons). A small solid-white block is
// reserved at the top-left for untextured quads.
//
// The GPU layer uploads Dirty() regions after each frame's geometry pass.
type Atlas struct {
	size   int
	pixels []byte
	cache  map[GlyphKey]AtlasGlyph

	shelfX, shelfY, shelfH int
	dirty                  image.Rectangle
	generation             int

	raster *vector.Rasterizer
	mask   *image.Alpha
}

const (
	atlasPad   = 1 // gap between entries so linear filtering never bleeds
	whiteBlock = 4 // reserved solid-white square at (0,0)
)

// NewAtlas creates a size×size atlas.
func NewAtlas(size int) *Atlas {
	a := &Atlas{size: size, pixels: make([]byte, size*size*4), raster: vector.NewRasterizer(1, 1)}
	a.Reset()
	return a
}

// Size returns the atlas edge length in texels.
func (a *Atlas) Size() int { return a.size }

// Pixels returns the RGBA backing store (row stride = Size()*4).
func (a *Atlas) Pixels() []byte { return a.pixels }

// Generation increments on every Reset; GPU-side caches can use it.
func (a *Atlas) Generation() int { return a.generation }

// WhiteUV returns a texture coordinate that samples pure white.
func (a *Atlas) WhiteUV() (u, v float32) {
	c := float32(whiteBlock) / 2 / float32(a.size)
	return c, c
}

// Reset drops all glyphs (called when the atlas is full).
func (a *Atlas) Reset() {
	clear(a.pixels)
	a.cache = make(map[GlyphKey]AtlasGlyph, 256)
	for y := 0; y < whiteBlock; y++ {
		for x := 0; x < whiteBlock; x++ {
			i := (y*a.size + x) * 4
			a.pixels[i], a.pixels[i+1], a.pixels[i+2], a.pixels[i+3] = 255, 255, 255, 255
		}
	}
	a.shelfX, a.shelfY, a.shelfH = whiteBlock+atlasPad, 0, whiteBlock
	a.dirty = image.Rect(0, 0, a.size, a.size)
	a.generation++
}

// TakeDirty returns the region modified since the last call and clears it.
func (a *Atlas) TakeDirty() (image.Rectangle, bool) {
	r := a.dirty
	a.dirty = image.Rectangle{}
	return r, !r.Empty()
}

// SubpixelBin splits a physical x coordinate into an integer pixel and a
// subpixel bin.
func SubpixelBin(x float32) (px float32, bin uint8) {
	fl := float32(math.Floor(float64(x)))
	b := int((x-fl)*SubpixelSteps + 0.5)
	if b >= SubpixelSteps {
		return fl + 1, 0
	}
	return fl, uint8(b)
}

// Glyph returns the atlas entry for glyph g of font f at sizePx physical
// pixels and subpixel bin sub, rasterizing it on first use. ok is false when
// the atlas is full; the caller should Reset and rebuild the frame.
func (a *Atlas) Glyph(f *Font, g GlyphID, sizePx float32, sub uint8) (AtlasGlyph, bool) {
	key := GlyphKey{Font: f.id, Glyph: g, Size: uint32(sizePx*64 + 0.5), SubX: sub}
	if e, ok := a.cache[key]; ok {
		return e, true
	}
	segs, err := f.outline(g, sizePx)
	if err != nil || len(segs) == 0 {
		e := AtlasGlyph{Empty: true}
		a.cache[key] = e
		return e, true
	}

	dx := float32(sub) / SubpixelSteps
	b := segs.Bounds()
	x0 := int(math.Floor(float64(fx(b.Min.X) + dx)))
	y0 := int(math.Floor(float64(fx(b.Min.Y))))
	x1 := int(math.Ceil(float64(fx(b.Max.X) + dx)))
	y1 := int(math.Ceil(float64(fx(b.Max.Y))))
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		e := AtlasGlyph{Empty: true}
		a.cache[key] = e
		return e, true
	}

	ax, ay, ok := a.alloc(w, h)
	if !ok {
		return AtlasGlyph{}, false
	}
	a.rasterize(segs, float32(x0)-dx, float32(y0), w, h)
	a.blit(ax, ay, w, h)

	e := AtlasGlyph{X: ax, Y: ay, W: w, H: h, OffX: float32(x0), OffY: float32(y0)}
	a.cache[key] = e
	return e, true
}

func (a *Atlas) alloc(w, h int) (x, y int, ok bool) {
	if w+atlasPad > a.size || h+atlasPad > a.size {
		return 0, 0, false
	}
	if a.shelfX+w+atlasPad > a.size { // next shelf
		a.shelfY += a.shelfH + atlasPad
		a.shelfX, a.shelfH = 0, 0
	}
	if a.shelfY+h+atlasPad > a.size {
		return 0, 0, false
	}
	x, y = a.shelfX, a.shelfY
	a.shelfX += w + atlasPad
	a.shelfH = max(a.shelfH, h)
	return x, y, true
}

// rasterize renders the outline into a.mask; (ox, oy) is subtracted from
// every point so the bitmap starts at (0,0).
func (a *Atlas) rasterize(segs sfnt.Segments, ox, oy float32, w, h int) {
	z := a.raster
	z.Reset(w, h)
	pt := func(i int, s sfnt.Segment) (float32, float32) {
		return fx(s.Args[i].X) - ox, fx(s.Args[i].Y) - oy
	}
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			z.MoveTo(pt(0, s))
		case sfnt.SegmentOpLineTo:
			z.LineTo(pt(0, s))
		case sfnt.SegmentOpQuadTo:
			bx, by := pt(0, s)
			cx, cy := pt(1, s)
			z.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := pt(0, s)
			cx, cy := pt(1, s)
			dx, dy := pt(2, s)
			z.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	z.ClosePath()
	if a.mask == nil || a.mask.Rect.Dx() < w || a.mask.Rect.Dy() < h {
		a.mask = image.NewAlpha(image.Rect(0, 0, max(w, 64), max(h, 64)))
	}
	r := image.Rect(0, 0, w, h)
	for y := 0; y < h; y++ { // clear only the part we use
		clear(a.mask.Pix[y*a.mask.Stride : y*a.mask.Stride+w])
	}
	z.Draw(a.mask, r, image.Opaque, image.Point{})
}

func (a *Atlas) blit(ax, ay, w, h int) {
	for y := 0; y < h; y++ {
		src := a.mask.Pix[y*a.mask.Stride : y*a.mask.Stride+w]
		row := ((ay+y)*a.size + ax) * 4
		for x, cov := range src {
			i := row + x*4
			a.pixels[i], a.pixels[i+1], a.pixels[i+2], a.pixels[i+3] = 255, 255, 255, cov
		}
	}
	a.dirty = a.dirty.Union(image.Rect(ax, ay, ax+w, ay+h))
}
