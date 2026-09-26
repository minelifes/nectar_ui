package geom

// Color is a straight (non-premultiplied) sRGB color with components in 0..1.
type Color struct{ R, G, B, A float32 }

// Hex builds an opaque color from 0xRRGGBB.
func Hex(rgb uint32) Color {
	return Color{
		R: float32((rgb>>16)&0xff) / 255,
		G: float32((rgb>>8)&0xff) / 255,
		B: float32(rgb&0xff) / 255,
		A: 1,
	}
}

// HexA builds a color from 0xRRGGBBAA.
func HexA(rgba uint32) Color {
	c := Hex(rgba >> 8)
	c.A = float32(rgba&0xff) / 255
	return c
}

// RGBA8 builds a color from 8-bit components.
func RGBA8(r, g, b, a uint8) Color {
	return Color{float32(r) / 255, float32(g) / 255, float32(b) / 255, float32(a) / 255}
}

// WithAlpha returns the color with a different alpha.
func (c Color) WithAlpha(a float32) Color { c.A = a; return c }

var (
	Transparent = Color{}
	Black       = Color{0, 0, 0, 1}
	White       = Color{1, 1, 1, 1}
)
