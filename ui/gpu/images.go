package gpu

import (
	"fmt"
	"image"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	xdraw "golang.org/x/image/draw"

	"github.com/minelifes/nectar_ui/ui/render"
)

// imageTex is a render.Image uploaded to the GPU at one scale. Images
// drawn much smaller than their pixel size get a pre-shrunk copy (level n
// = 1/2^n size, box-filtered on the CPU) instead of hardware mipmaps: it
// works on every backend (the software one has no mips) and a big photo
// shown as a thumbnail only costs a thumbnail's texture memory.
type imageTex struct {
	tex      *wgpu.Texture
	view     *wgpu.TextureView
	linear   *wgpu.BindGroup
	nearest  *wgpu.BindGroup
	w, h     int // texture size
	lastUsed uint64
	bytes    int
}

type texKey struct {
	id    uint64
	level int
}

func (t *imageTex) release() {
	for _, res := range []interface{ Release() }{t.linear, t.nearest, t.view, t.tex} {
		if res != nil {
			res.Release()
		}
	}
}

// imageEvictFrames: textures not drawn for this many frames are freed.
const imageEvictFrames = 600

func (r *Renderer) initImages() error {
	var err error
	r.imageBGL, err = r.dev.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "nectar-ui-image",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D}},
			{Binding: 1, Visibility: gputypes.ShaderStageFragment,
				Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu: image bind group layout: %w", err)
	}
	mk := func(label string, f, mip gputypes.FilterMode) (*wgpu.Sampler, error) {
		return r.dev.CreateSampler(&wgpu.SamplerDescriptor{
			Label:        label,
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MagFilter:    f, MinFilter: f, MipmapFilter: mip,
			LodMaxClamp: 32,
		})
	}
	if r.imgLinear, err = mk("nectar-ui-image-linear", gputypes.FilterModeLinear, gputypes.FilterModeLinear); err != nil {
		return fmt.Errorf("gpu: image sampler: %w", err)
	}
	if r.imgNearest, err = mk("nectar-ui-image-nearest", gputypes.FilterModeNearest, gputypes.FilterModeNearest); err != nil {
		return fmt.Errorf("gpu: image sampler: %w", err)
	}
	// 1×1 placeholder bound for everything that isn't an image.
	blank := image.NewRGBA(image.Rect(0, 0, 1, 1))
	r.blankImage, err = r.upload1(blank)
	if err != nil {
		return err
	}
	r.images = map[texKey]*imageTex{}
	return nil
}

// upload1 creates a texture and its bind groups from pix.
func (r *Renderer) upload1(pix *image.RGBA) (*imageTex, error) {
	w, h := pix.Rect.Dx(), pix.Rect.Dy()
	tex, err := r.dev.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "nectar-ui-image",
		Size:          wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D,
		Format:    gputypes.TextureFormatRGBA8Unorm,
		Usage:     wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("gpu: image texture: %w", err)
	}
	t := &imageTex{tex: tex, w: w, h: h, bytes: w * h * 4}
	data, row := padRows(pix)
	err = r.queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectAll},
		data,
		&wgpu.ImageDataLayout{BytesPerRow: uint32(row), RowsPerImage: uint32(h)},
		&wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
	)
	if err != nil {
		t.release()
		return nil, fmt.Errorf("gpu: write image: %w", err)
	}
	if t.view, err = r.dev.CreateTextureView(tex, nil); err != nil {
		t.release()
		return nil, fmt.Errorf("gpu: image view: %w", err)
	}
	for _, v := range []struct {
		dst **wgpu.BindGroup
		smp *wgpu.Sampler
	}{{&t.linear, r.imgLinear}, {&t.nearest, r.imgNearest}} {
		*v.dst, err = r.dev.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label: "nectar-ui-image", Layout: r.imageBGL,
			Entries: []wgpu.BindGroupEntry{{Binding: 0, TextureView: t.view}, {Binding: 1, Sampler: v.smp}},
		})
		if err != nil {
			t.release()
			return nil, fmt.Errorf("gpu: image bind group: %w", err)
		}
	}
	return t, nil
}

// texture returns img's texture at level (1/2^level size), uploading it
// on first use.
func (r *Renderer) texture(img *render.Image, level int) *imageTex {
	key := texKey{img.ID(), level}
	if t, ok := r.images[key]; ok {
		t.lastUsed = r.frame
		return t
	}
	pix := img.Pixels()
	// Clamp to the device limit (large photos).
	if lim := int(r.dev.Limits().MaxTextureDimension2D); lim > 0 {
		if w, h := pix.Rect.Dx(), pix.Rect.Dy(); w > lim || h > lim {
			s := float64(lim) / float64(max(w, h))
			small := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*s)), max(1, int(float64(h)*s))))
			xdraw.ApproxBiLinear.Scale(small, small.Bounds(), pix, pix.Rect, xdraw.Src, nil)
			pix = small
		}
	}
	for l := 0; l < level; l++ {
		pix = halve(pix)
	}
	t, err := r.upload1(pix)
	if err != nil {
		return nil
	}
	t.lastUsed = r.frame
	r.images[key] = t
	return t
}

// evictImages frees textures that haven't been drawn for a while.
func (r *Renderer) evictImages() {
	for id, t := range r.images {
		if r.frame-t.lastUsed > imageEvictFrames {
			t.release()
			delete(r.images, id)
		}
	}
}

// ImageMemory reports GPU texture memory used by images (bytes, count).
func (r *Renderer) ImageMemory() (bytes, count int) {
	for _, t := range r.images {
		bytes += t.bytes
	}
	return bytes, len(r.images)
}

// padRows returns the pixels with rows padded to 256 bytes, the copy
// alignment WebGPU backends expect (the pure-Go one requires it).
func padRows(img *image.RGBA) ([]byte, int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	row := (w*4 + 255) &^ 255
	if row == img.Stride {
		return img.Pix[:row*h], row
	}
	out := make([]byte, row*h)
	for y := 0; y < h; y++ {
		copy(out[y*row:y*row+w*4], img.Pix[y*img.Stride:])
	}
	return out, row
}

// halve box-filters a premultiplied image to half size (odd edges keep
// the last row/column).
func halve(src *image.RGBA) *image.RGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := max(1, sw/2), max(1, sh/2)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := min(2*y, sh-1), min(2*y+1, sh-1)
		for x := 0; x < dw; x++ {
			x0, x1 := min(2*x, sw-1), min(2*x+1, sw-1)
			a := src.Pix[y0*src.Stride+x0*4:]
			b := src.Pix[y0*src.Stride+x1*4:]
			c := src.Pix[y1*src.Stride+x0*4:]
			d := src.Pix[y1*src.Stride+x1*4:]
			o := dst.Pix[y*dst.Stride+x*4:]
			for k := 0; k < 4; k++ {
				o[k] = uint8((uint32(a[k]) + uint32(b[k]) + uint32(c[k]) + uint32(d[k]) + 2) / 4)
			}
		}
	}
	return dst
}

// drawImage appends an image quad (and switches the batch's texture).
func (r *Renderer) drawImage(c *render.Command) {
	s := r.scale
	dst, src := c.Rect, c.Src
	w, h := dst.W*s, dst.H*s
	// Pick the pre-shrunk copy that is still at least as detailed as the
	// screen (bilinear handles the last <2x step). Nearest keeps full size.
	level := 0
	if c.Width != 1 {
		texPerPx := max(src.W/max(w, 1e-3), src.H/max(h, 1e-3))
		for texPerPx >= 2 && level < 12 {
			texPerPx /= 2
			level++
		}
	}
	t := r.texture(c.Image, level)
	if t == nil {
		return
	}
	bg := t.linear
	if c.Width == 1 {
		bg = t.nearest
	}
	r.setImage(bg)
	iw, ih := float32(c.Image.Width()), float32(c.Image.Height())
	// The texture may be a scaled copy: UVs are relative, so that's fine.
	u0, v0 := src.X/iw, src.Y/ih
	u1, v1 := (src.X+src.W)/iw, (src.Y+src.H)/ih
	x0, y0 := dst.X*s, dst.Y*s
	// Pad by a pixel for anti-aliased edges; extrapolate the UVs.
	const pad = 1
	du, dv := (u1-u0)/max(w, 1e-3), (v1-v0)/max(h, 1e-3)
	hw, hh := w/2, h/2
	local := [4]float32{-hw - pad, -hh - pad, hw + pad, hh + pad}
	params := [4]float32{hw, hh, c.Radius * s, modeImage}
	r.quad(x0-pad, y0-pad, x0+w+pad, y0+h+pad,
		u0-du*pad, v0-dv*pad, u1+du*pad, v1+dv*pad,
		c.Color, local, params, [4]float32{})
}
