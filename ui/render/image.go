package render

import (
	"image"
	"image/draw"
	"sync/atomic"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// Image is decoded pixel data ready to draw with Canvas.DrawImage. The
// GPU backend uploads it to a texture the first time it's drawn and keeps
// that texture while the Image is in use, so create an Image once and
// reuse it (widgets.Image does this through its cache).
type Image struct {
	id  uint64
	pix *image.RGBA // premultiplied, bounds at (0,0)
}

var imageIDs atomic.Uint64

// NewImage copies img into a drawable Image.
func NewImage(img image.Image) *Image {
	b := img.Bounds()
	rgba, ok := img.(*image.RGBA)
	if !ok || b.Min != (image.Point{}) {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	}
	return &Image{id: imageIDs.Add(1), pix: rgba}
}

// ID is unique per Image.
func (i *Image) ID() uint64 { return i.id }

// Width / Height are the size in pixels.
func (i *Image) Width() int  { return i.pix.Rect.Dx() }
func (i *Image) Height() int { return i.pix.Rect.Dy() }

// Size returns the pixel size as a geom.Size.
func (i *Image) Size() geom.Size { return geom.Size{W: float32(i.Width()), H: float32(i.Height())} }

// Pixels returns the premultiplied RGBA pixels (don't modify them).
func (i *Image) Pixels() *image.RGBA { return i.pix }

// Bytes is the decoded size in memory.
func (i *Image) Bytes() int { return len(i.pix.Pix) }

// ImageFit says how an image fills its box (like CSS object-fit).
type ImageFit uint8

const (
	FitContain   ImageFit = iota // as large as possible, fully visible (letterboxed)
	FitCover                     // fills the box, cropping what doesn't fit
	FitFill                      // stretches to the box (may distort)
	FitWidth                     // full width; may crop or letterbox vertically
	FitHeight                    // full height; may crop or letterbox horizontally
	FitNone                      // natural size, cropped to the box
	FitScaleDown                 // like FitNone, but shrinks (never grows) to fit
)

// ApplyFit computes which part of an image (src, in image pixels) goes where
// in a box (dst, in the box's coordinates) for fit and alignment.
func ApplyFit(fit ImageFit, img geom.Size, box geom.Rect, align geom.Alignment) (src, dst geom.Rect) {
	if img.W <= 0 || img.H <= 0 || box.W <= 0 || box.H <= 0 {
		return geom.Rect{}, geom.Rect{}
	}
	var out geom.Size // size of the whole image on screen
	switch fit {
	case FitFill:
		out = geom.Size{W: box.W, H: box.H}
	case FitCover:
		s := max(box.W/img.W, box.H/img.H)
		out = geom.Size{W: img.W * s, H: img.H * s}
	case FitWidth:
		s := box.W / img.W
		out = geom.Size{W: box.W, H: img.H * s}
	case FitHeight:
		s := box.H / img.H
		out = geom.Size{W: img.W * s, H: box.H}
	case FitNone:
		out = img
	case FitScaleDown:
		s := min(1, box.W/img.W, box.H/img.H)
		out = geom.Size{W: img.W * s, H: img.H * s}
	default: // FitContain
		s := min(box.W/img.W, box.H/img.H)
		out = geom.Size{W: img.W * s, H: img.H * s}
	}
	// Position the full image by alignment, then clip it to the box.
	ox := box.X + (box.W-out.W)*(align.X+1)/2
	oy := box.Y + (box.H-out.H)*(align.Y+1)/2
	full := geom.Rect{X: ox, Y: oy, W: out.W, H: out.H}
	dst = full.Intersect(box)
	if dst.Empty() {
		return geom.Rect{}, geom.Rect{}
	}
	sx, sy := img.W/out.W, img.H/out.H
	src = geom.Rect{X: (dst.X - ox) * sx, Y: (dst.Y - oy) * sy, W: dst.W * sx, H: dst.H * sy}
	return src, dst
}

// DrawImage draws the src part of img (image pixels; zero = all of it) into
// dst, with rounded corners of radius and opacity (0..1). Nearest sampling
// keeps pixel art crisp; otherwise it's smoothed (with mipmaps when
// shrinking).
func (c *Canvas) DrawImage(img *Image, src, dst geom.Rect, radius, opacity float32, nearest bool) {
	if img == nil || dst.Empty() || opacity <= 0 {
		return
	}
	if src.Empty() {
		src = geom.Rect{W: float32(img.Width()), H: float32(img.Height())}
	}
	radius = min(radius, dst.W/2, dst.H/2)
	cmd := Command{Kind: CmdImage, Rect: dst, Src: src, Image: img, Radius: max(0, radius),
		Color: geom.Color{R: 1, G: 1, B: 1, A: min(opacity, 1)}}
	if nearest {
		cmd.Width = 1
	}
	c.add(cmd)
}

// RenderImage lays out and paints an Image. Without a fixed Width/Height
// it takes the image's pixel size (1 px = 1 logical px), shrunk to the
// constraints with its aspect ratio kept; with one of them set, the other
// follows the aspect ratio. With no image yet (still loading) it takes
// Width×Height, or the smallest size the constraints allow.
type RenderImage struct {
	Box
	Image         *Image
	Width, Height float32 // 0 = from the image
	Fit           ImageFit
	Align         geom.Alignment
	Radius        float32
	Opacity       float32 // 0..1
	Nearest       bool
}

func (r *RenderImage) VisitChildren(func(RenderObject)) {}

func (r *RenderImage) PerformLayout(c geom.Constraints) geom.Size {
	w, h := r.Width, r.Height
	if r.Image != nil {
		iw, ih := float32(r.Image.Width()), float32(r.Image.Height())
		switch {
		case w == 0 && h == 0:
			w, h = iw, ih
			// Shrink into the max constraints, keeping the aspect ratio.
			if s := min(1, c.MaxW/w, c.MaxH/h); s < 1 {
				w, h = w*s, h*s
			}
		case w == 0:
			w = h * iw / ih
		case h == 0:
			h = w * ih / iw
		}
	}
	return c.Constrain(geom.Size{W: w, H: h})
}

func (r *RenderImage) Paint(ctx *PaintContext, o geom.Offset) {
	if r.Image == nil {
		return
	}
	box := geom.RectFrom(o, r.Size())
	src, dst := ApplyFit(r.Fit, r.Image.Size(), box, r.Align)
	ctx.Canvas.DrawImage(r.Image, src, dst, r.Radius, r.Opacity, r.Nearest)
}
