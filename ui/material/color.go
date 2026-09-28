package material

import (
	"math"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// Material 3 builds every color from tonal palettes: a hue and chroma with
// "tone" 0 (black) … 100 (white). M3's HCT tone is CIE L*, so we build the
// palettes in CIE LCh(ab): same lightness axis, hue from the seed, and
// chroma reduced as needed to stay inside sRGB.

// TonalPalette produces colors of one hue/chroma at any tone.
type TonalPalette struct {
	Hue, Chroma float64
}

// Tone returns the palette color at tone t (0..100).
func (p TonalPalette) Tone(t float64) geom.Color {
	return lchToSRGB(t, p.Chroma, p.Hue)
}

// PaletteOf returns the hue and chroma of c.
func PaletteOf(c geom.Color) TonalPalette {
	_, ch, h := srgbToLCh(c)
	return TonalPalette{Hue: h, Chroma: ch}
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSRGB(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// D65 reference white.
const xn, yn, zn = 0.95047, 1.0, 1.08883

func labF(t float64) float64 {
	if t > 216.0/24389 {
		return math.Cbrt(t)
	}
	return (24389.0/27*t + 16) / 116
}

func labFInv(t float64) float64 {
	if t3 := t * t * t; t3 > 216.0/24389 {
		return t3
	}
	return (116*t - 16) / (24389.0 / 27)
}

func srgbToLCh(c geom.Color) (l, ch, h float64) {
	r, g, b := srgbToLinear(float64(c.R)), srgbToLinear(float64(c.G)), srgbToLinear(float64(c.B))
	x := 0.4124564*r + 0.3575761*g + 0.1804375*b
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := 0.0193339*r + 0.1191920*g + 0.9503041*b
	fx, fy, fz := labF(x/xn), labF(y/yn), labF(z/zn)
	l = 116*fy - 16
	a := 500 * (fx - fy)
	bb := 200 * (fy - fz)
	ch = math.Hypot(a, bb)
	h = math.Atan2(bb, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return
}

// labToLinear converts CIE Lab to linear sRGB (may be out of gamut).
func labToLinear(l, a, b float64) (float64, float64, float64) {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	x, y, z := xn*labFInv(fx), yn*labFInv(fy), zn*labFInv(fz)
	r := 3.2404542*x - 1.5371385*y - 0.4985314*z
	g := -0.9692660*x + 1.8760108*y + 0.0415560*z
	bl := 0.0556434*x - 0.2040259*y + 1.0572252*z
	return r, g, bl
}

// lchToSRGB converts LCh to sRGB, lowering chroma until the color fits.
func lchToSRGB(l, ch, h float64) geom.Color {
	l = math.Max(0, math.Min(100, l))
	hr := h * math.Pi / 180
	in := func(c float64) (bool, float64, float64, float64) {
		r, g, b := labToLinear(l, c*math.Cos(hr), c*math.Sin(hr))
		const eps = 1e-4
		ok := r >= -eps && r <= 1+eps && g >= -eps && g <= 1+eps && b >= -eps && b <= 1+eps
		return ok, r, g, b
	}
	ok, r, g, b := in(ch)
	if !ok {
		lo, hi := 0.0, ch
		for i := 0; i < 24; i++ {
			mid := (lo + hi) / 2
			if good, _, _, _ := in(mid); good {
				lo = mid
			} else {
				hi = mid
			}
		}
		_, r, g, b = in(lo)
	}
	cl := func(v float64) float32 { return float32(math.Max(0, math.Min(1, linearToSRGB(v)))) }
	return geom.Color{R: cl(r), G: cl(g), B: cl(b), A: 1}
}

// Blend returns base with overlay composited at the given opacity (used for
// state layers and tinted surfaces).
func Blend(base, overlay geom.Color, opacity float32) geom.Color {
	a := overlay.A * opacity
	return geom.Color{
		R: base.R + (overlay.R-base.R)*a,
		G: base.G + (overlay.G-base.G)*a,
		B: base.B + (overlay.B-base.B)*a,
		A: base.A + (1-base.A)*a,
	}
}

// Lerp interpolates two colors.
func LerpColor(a, b geom.Color, t float32) geom.Color {
	return geom.Color{R: a.R + (b.R-a.R)*t, G: a.G + (b.G-a.G)*t, B: a.B + (b.B-a.B)*t, A: a.A + (b.A-a.A)*t}
}
