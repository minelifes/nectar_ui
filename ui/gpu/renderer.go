// Package gpu turns a render.Canvas display list into wgpu draw calls.
//
// Everything is drawn by one pipeline from one vertex/index buffer pair. The
// only state change between draws is the scissor rect, so a typical frame is
// a handful of DrawIndexed calls regardless of how many widgets there are.
package gpu

import (
	"encoding/binary"
	"fmt"
	text2 "github.com/minelifes/nectar_ui/ui/widgets/text"
	"log/slog"
	"math"
	"reflect"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	xvector "golang.org/x/image/vector"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// AtlasSize is the edge length of the glyph atlas texture.
const AtlasSize = 1024

const (
	floatsPerVertex = 18 // pos2 uv2 color4 local2 params4 extra4
	vertexStride    = floatsPerVertex * 4
	uniformSize     = 16
)

// Renderer owns all GPU resources for UI drawing.
type Renderer struct {
	dev    *wgpu.Device
	queue  *wgpu.Queue
	format gputypes.TextureFormat

	pipeline  *wgpu.RenderPipeline
	layout    *wgpu.PipelineLayout
	bgl       *wgpu.BindGroupLayout
	bindGroup *wgpu.BindGroup
	sampler   *wgpu.Sampler
	atlasTex  *wgpu.Texture
	atlasView *wgpu.TextureView
	uniforms  *wgpu.Buffer
	vbuf      *wgpu.Buffer
	ibuf      *wgpu.Buffer

	atlas *text2.Atlas

	// Images: textures per render.Image (and pre-shrunk copies), group 1.
	imageBGL   *wgpu.BindGroupLayout
	imgLinear  *wgpu.Sampler
	imgNearest *wgpu.Sampler
	blankImage *imageTex
	images     map[texKey]*imageTex
	frame      uint64

	// CPU-side geometry, reused every frame.
	verts   []float32
	indices []uint32
	batches []batch

	scale      float32
	curImage   *wgpu.BindGroup
	fbW, fbH   uint32
	whiteU     float32
	whiteV     float32
	atlasScale float32
}

type batch struct {
	scissor      gputypes.ScissorRect
	image        *wgpu.BindGroup // group 1: the image texture (or the blank one)
	first, count uint32
}

// New creates the renderer for surfaces of the given format.
func New(dev *wgpu.Device, format gputypes.TextureFormat) (*Renderer, error) {
	r := &Renderer{dev: dev, queue: dev.Queue(), format: format, atlas: text2.NewAtlas(AtlasSize)}
	r.whiteU, r.whiteV = r.atlas.WhiteUV()
	r.atlasScale = 1 / float32(AtlasSize)
	if err := r.init(); err != nil {
		r.Release()
		return nil, err
	}
	return r, nil
}

func (r *Renderer) init() error {
	var err error
	shader, err := r.dev.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "nectar-ui", WGSL: shaderWGSL})
	if err != nil {
		return fmt.Errorf("gpu: shader: %w", err)
	}
	defer shader.Release()

	r.bgl, err = r.dev.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "nectar-ui",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: uniformSize}},
			{Binding: 1, Visibility: gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D}},
			{Binding: 2, Visibility: gputypes.ShaderStageFragment,
				Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu: bind group layout: %w", err)
	}
	if err := r.initImages(); err != nil {
		return err
	}
	r.layout, err = r.dev.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "nectar-ui", BindGroupLayouts: []*wgpu.BindGroupLayout{r.bgl, r.imageBGL}})
	if err != nil {
		return fmt.Errorf("gpu: pipeline layout: %w", err)
	}

	premul := gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha, Operation: gputypes.BlendOperationAdd},
	}
	r.pipeline, err = r.dev.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "nectar-ui",
		Layout: r.layout,
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: vertexStride,
				StepMode:    gputypes.VertexStepModeVertex,
				Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},  // pos
					{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1},  // uv
					{Format: gputypes.VertexFormatFloat32x4, Offset: 16, ShaderLocation: 2}, // color
					{Format: gputypes.VertexFormatFloat32x2, Offset: 32, ShaderLocation: 3}, // local
					{Format: gputypes.VertexFormatFloat32x4, Offset: 40, ShaderLocation: 4}, // params
					{Format: gputypes.VertexFormatFloat32x4, Offset: 56, ShaderLocation: 5}, // extra
				},
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{Format: r.format, Blend: &premul, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu: pipeline: %w", err)
	}

	r.atlasTex, err = r.dev.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "nectar-ui-atlas",
		Size:          wgpu.Extent3D{Width: AtlasSize, Height: AtlasSize, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D,
		Format:    gputypes.TextureFormatRGBA8Unorm,
		Usage:     wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("gpu: atlas texture: %w", err)
	}
	if r.atlasView, err = r.dev.CreateTextureView(r.atlasTex, nil); err != nil {
		return fmt.Errorf("gpu: atlas view: %w", err)
	}
	// Glyph quads are pixel-aligned, so nearest == linear; linear keeps
	// fractional-scale rects smooth.
	r.sampler, err = r.dev.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "nectar-ui",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeNearest,
		LodMaxClamp:  32,
	})
	if err != nil {
		return fmt.Errorf("gpu: sampler: %w", err)
	}
	r.uniforms, err = r.dev.CreateBuffer(&wgpu.BufferDescriptor{Label: "nectar-ui-uniforms", Size: uniformSize, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst})
	if err != nil {
		return fmt.Errorf("gpu: uniforms: %w", err)
	}
	r.bindGroup, err = r.dev.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "nectar-ui",
		Layout: r.bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: r.uniforms, Size: uniformSize},
			{Binding: 1, TextureView: r.atlasView},
			{Binding: 2, Sampler: r.sampler},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu: bind group: %w", err)
	}
	return nil
}

// Atlas exposes the glyph atlas (e.g. for debugging or preloading).
func (r *Renderer) Atlas() *text2.Atlas { return r.atlas }

// Frame describes one frame to draw.
type Frame struct {
	Encoder  *wgpu.CommandEncoder // encoder to record into (not finished here)
	Target   *wgpu.TextureView    // color attachment
	Width    uint32               // target size in physical pixels
	Height   uint32
	Scale    float32    // logical -> physical pixels
	Clear    geom.Color // clear color; A == 0 loads existing contents instead
	Commands []render.Command
}

// Draw records a render pass drawing the display list into f.Target.
func (r *Renderer) Draw(f Frame) error {
	if f.Width == 0 || f.Height == 0 || f.Encoder == nil || f.Target == nil {
		return nil
	}
	r.scale, r.fbW, r.fbH = max(f.Scale, 0.01), f.Width, f.Height
	r.frame++

	// Build geometry; if the atlas overflows mid-frame, reset it and rebuild
	// once (glyphs that still don't fit are skipped).
	if !r.build(f.Commands, true) {
		slog.Debug("nectar-ui: glyph atlas full, resetting")
		r.atlas.Reset()
		r.build(f.Commands, false)
	}

	r.evictImages()
	if err := r.upload(); err != nil {
		return err
	}

	load := gputypes.LoadOpClear
	if f.Clear.A == 0 {
		load = gputypes.LoadOpLoad
	}
	c := f.Clear
	pass, err := f.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "nectar-ui",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: f.Target, LoadOp: load, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: float64(c.R * c.A), G: float64(c.G * c.A), B: float64(c.B * c.A), A: float64(c.A)},
		}},
	})
	if err != nil {
		return fmt.Errorf("gpu: begin pass: %w", err)
	}
	if len(r.indices) > 0 {
		pass.SetPipeline(r.pipeline)
		pass.SetBindGroup(0, r.bindGroup, nil)
		pass.SetVertexBuffer(0, r.vbuf, 0)
		pass.SetIndexBuffer(r.ibuf, gputypes.IndexFormatUint32, 0)
		var bound *wgpu.BindGroup
		for _, b := range r.batches {
			if b.count == 0 || b.scissor.Width == 0 || b.scissor.Height == 0 {
				continue
			}
			if b.image != bound {
				pass.SetBindGroup(1, b.image, nil)
				bound = b.image
			}
			pass.SetScissorRect(b.scissor)
			pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: b.count, InstanceCount: 1, FirstIndex: b.first})
		}
	}
	if err := pass.End(); err != nil {
		return fmt.Errorf("gpu: end pass: %w", err)
	}
	return nil
}

// upload writes uniforms, dirty atlas rows and geometry to the GPU.
func (r *Renderer) upload() error {
	lin := float32(0)
	if r.format.IsSrgb() {
		lin = 1
	}
	u := r.f32bytes([]float32{float32(r.fbW), float32(r.fbH), lin, 0})
	if err := r.queue.WriteBuffer(r.uniforms, 0, u); err != nil {
		return fmt.Errorf("gpu: write uniforms: %w", err)
	}

	if dirty, ok := r.atlas.TakeDirty(); ok {
		// Upload whole rows: simple and BytesPerRow stays aligned.
		y0, y1 := dirty.Min.Y, dirty.Max.Y
		stride := AtlasSize * 4
		err := r.queue.WriteTexture(
			&wgpu.ImageCopyTexture{Texture: r.atlasTex, Origin: wgpu.Origin3D{Y: uint32(y0)}, Aspect: gputypes.TextureAspectAll},
			r.atlas.Pixels()[y0*stride:y1*stride],
			&wgpu.ImageDataLayout{BytesPerRow: uint32(stride), RowsPerImage: uint32(y1 - y0)},
			&wgpu.Extent3D{Width: AtlasSize, Height: uint32(y1 - y0), DepthOrArrayLayers: 1},
		)
		if err != nil {
			return fmt.Errorf("gpu: write atlas: %w", err)
		}
	}

	if len(r.indices) == 0 {
		return nil
	}
	var err error
	vb := r.f32bytes(r.verts)
	if r.vbuf, err = r.ensureBuffer(r.vbuf, uint64(len(vb)), wgpu.BufferUsageVertex, "nectar-ui-vertices"); err != nil {
		return err
	}
	if err := r.queue.WriteBuffer(r.vbuf, 0, vb); err != nil {
		return fmt.Errorf("gpu: write vertices: %w", err)
	}
	ib := make([]byte, len(r.indices)*4)
	for i, v := range r.indices {
		binary.LittleEndian.PutUint32(ib[i*4:], v)
	}
	if r.ibuf, err = r.ensureBuffer(r.ibuf, uint64(len(ib)), wgpu.BufferUsageIndex, "nectar-ui-indices"); err != nil {
		return err
	}
	if err := r.queue.WriteBuffer(r.ibuf, 0, ib); err != nil {
		return fmt.Errorf("gpu: write indices: %w", err)
	}
	return nil
}

// ensureBuffer grows buf (to the next power of two) when it's too small.
func (r *Renderer) ensureBuffer(buf *wgpu.Buffer, size uint64, usage gputypes.BufferUsage, label string) (*wgpu.Buffer, error) {
	if buf != nil && buf.Size() >= size {
		return buf, nil
	}
	c := uint64(4096)
	for c < size {
		c *= 2
	}
	if buf != nil {
		buf.Release()
	}
	nb, err := r.dev.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Size: c, Usage: usage | wgpu.BufferUsageCopyDst})
	if err != nil {
		return nil, fmt.Errorf("gpu: create %s: %w", label, err)
	}
	return nb, nil
}

// f32bytes encodes floats as little-endian bytes. A fresh slice is returned
// because queue writes may be staged until submit.
func (r *Renderer) f32bytes(fs []float32) []byte {
	b := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// --- geometry --------------------------------------------------------------

// build converts commands into vertices. Returns false if the atlas filled
// up (only when strict; otherwise missing glyphs are skipped).
func (r *Renderer) build(cmds []render.Command, strict bool) bool {
	r.verts = r.verts[:0]
	r.indices = r.indices[:0]
	r.batches = r.batches[:0]
	r.curImage = r.blankImage.linear
	for i := range cmds {
		c := &cmds[i]
		r.setScissor(c.Clip)
		switch c.Kind {
		case render.CmdRect:
			r.rect(c.Rect, c.Radius, c.Color)
		case render.CmdText:
			if !r.text(c.Text, c.Rect.Origin(), c.Color) && strict {
				return false
			}
		case render.CmdShadow:
			r.sdfQuad(c.Rect, c.Radius, c.Color, modeShadow, c.Blur*r.scale*2, [4]float32{c.Blur * r.scale})
		case render.CmdStroke:
			r.sdfQuad(c.Rect, c.Radius, c.Color, modeStroke, 1, [4]float32{0, c.Width * r.scale})
		case render.CmdArc:
			r.sdfQuad(c.Rect, c.Rect.W/2, c.Color, modeArc, 1, [4]float32{0, c.Width * r.scale, c.Start, c.Sweep})
		case render.CmdRipple:
			cx := (c.Center.X - (c.Rect.X + c.Rect.W/2)) * r.scale
			cy := (c.Center.Y - (c.Rect.Y + c.Rect.H/2)) * r.scale
			r.sdfQuad(c.Rect, c.Radius, c.Color, modeRipple, 1, [4]float32{cx, cy, c.Blur * r.scale})
		case render.CmdIcon:
			if !r.icon(c) && strict {
				return false
			}
		case render.CmdImage:
			r.drawImage(c)
		}
	}
	if n := len(r.batches); n > 0 {
		b := &r.batches[n-1]
		b.count = uint32(len(r.indices)) - b.first
	}
	return true
}

func (r *Renderer) setScissor(clip geom.Rect) {
	s := r.scale
	x0 := clampU(math.Floor(float64(clip.X*s)), r.fbW)
	y0 := clampU(math.Floor(float64(clip.Y*s)), r.fbH)
	x1 := clampU(math.Ceil(float64(clip.Right()*s)), r.fbW)
	y1 := clampU(math.Ceil(float64(clip.Bottom()*s)), r.fbH)
	sc := gputypes.ScissorRect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
	r.setBatch(sc, r.curImage)
}

// setImage switches the image texture for the following quads. Other
// commands keep whatever is bound (they don't sample it), so only images
// split batches.
func (r *Renderer) setImage(bg *wgpu.BindGroup) {
	r.curImage = bg
	if n := len(r.batches); n > 0 {
		r.setBatch(r.batches[n-1].scissor, bg)
	}
}

// setBatch starts a new batch unless the current one has the same state.
func (r *Renderer) setBatch(sc gputypes.ScissorRect, image *wgpu.BindGroup) {
	if n := len(r.batches); n > 0 {
		b := &r.batches[n-1]
		if b.scissor == sc && b.image == image {
			return
		}
		b.count = uint32(len(r.indices)) - b.first
		if b.count == 0 { // nothing drawn yet: just retarget it
			b.scissor, b.image = sc, image
			return
		}
	}
	r.batches = append(r.batches, batch{scissor: sc, image: image, first: uint32(len(r.indices))})
}

func clampU(v float64, hi uint32) uint32 {
	if v < 0 {
		return 0
	}
	if v > float64(hi) {
		return hi
	}
	return uint32(v)
}

// quad appends 4 vertices + 6 indices.
func (r *Renderer) quad(x0, y0, x1, y1, u0, v0, u1, v1 float32, c geom.Color, local, params, extra [4]float32) {
	base := uint32(len(r.verts) / floatsPerVertex)
	corner := func(x, y, u, v, lx, ly float32) {
		r.verts = append(r.verts, x, y, u, v, c.R, c.G, c.B, c.A, lx, ly,
			params[0], params[1], params[2], params[3], extra[0], extra[1], extra[2], extra[3])
	}
	corner(x0, y0, u0, v0, local[0], local[1])
	corner(x1, y0, u1, v0, local[2], local[1])
	corner(x1, y1, u1, v1, local[2], local[3])
	corner(x0, y1, u0, v1, local[0], local[3])
	r.indices = append(r.indices, base, base+1, base+2, base, base+2, base+3)
}

// Shader modes (params.w).
const (
	modeTexture = 0
	modeFill    = 1
	modeShadow  = 2
	modeStroke  = 3
	modeArc     = 4
	modeRipple  = 5
	modeImage   = 6
)

func (r *Renderer) rect(rc geom.Rect, radius float32, c geom.Color) {
	r.sdfQuad(rc, radius, c, modeFill, 1, [4]float32{})
}

// sdfQuad draws an analytic shape (rounded rect fill / shadow / stroke / arc)
// covering rc grown by pad physical pixels.
func (r *Renderer) sdfQuad(rc geom.Rect, radius float32, c geom.Color, mode int, pad float32, extra [4]float32) {
	s := r.scale
	x0, y0 := rc.X*s, rc.Y*s
	w, h := rc.W*s, rc.H*s
	hw, hh := w/2, h/2
	local := [4]float32{-hw - pad, -hh - pad, hw + pad, hh + pad}
	params := [4]float32{hw, hh, radius * s, float32(mode)}
	r.quad(x0-pad, y0-pad, x0+w+pad, y0+h+pad, r.whiteU, r.whiteV, r.whiteU, r.whiteV, c, local, params, extra)
}

// icon rasterizes (once per size) and draws a vector icon.
func (r *Renderer) icon(c *render.Command) bool {
	ic := c.Icon
	path := ic.Path()
	if path == nil {
		return true
	}
	s := r.scale
	px := float32(math.Round(float64(c.Rect.W * s)))
	if px < 1 {
		return true
	}
	x0 := float32(math.Round(float64(c.Rect.X * s)))
	y0 := float32(math.Round(float64(c.Rect.Y * s)))
	n := int(px)
	key := text2.MaskKey{Kind: 1, ID: ic.ID(), Size: uint32(px * 64)}
	e, ok := r.atlas.Mask(key, n, n, func(z *xvector.Rasterizer) {
		path.Rasterize(z, px/ic.View, 0, 0)
	})
	if !ok {
		return false
	}
	if e.Empty {
		return true
	}
	inv := r.atlasScale
	u0, v0 := float32(e.X)*inv, float32(e.Y)*inv
	u1, v1 := float32(e.X+e.W)*inv, float32(e.Y+e.H)*inv
	r.quad(x0, y0, x0+px, y0+px, u0, v0, u1, v1, c.Color, [4]float32{}, [4]float32{}, [4]float32{})
	return true
}

func (r *Renderer) text(p *text2.Paragraph, origin geom.Offset, c geom.Color) bool {
	if p == nil {
		return true
	}
	s := r.scale
	font := p.Style.Font
	sizePx := p.Style.Size * s
	inv := r.atlasScale
	ok := true
	col := c
	for _, line := range p.Lines {
		lx := (origin.X + line.X) * s
		for _, g := range p.Glyphs[line.First:line.Last] {
			penX, sub := text2.SubpixelBin(lx + g.X*s)
			penY := float32(math.Round(float64((origin.Y + g.Y) * s)))
			gf, gs := font, sizePx
			if g.Font != nil {
				gf = g.Font
			}
			if g.Size > 0 {
				gs = g.Size * s
			}
			if p.Rich {
				// Rich paragraphs color each glyph; c carries the opacity.
				col = g.Color
				col.A *= c.A
			}
			e, fits := r.atlas.Glyph(gf, g.ID, gs, sub)
			if !fits {
				ok = false
				continue
			}
			if e.Empty {
				continue
			}
			x0, y0 := penX+e.OffX, penY+e.OffY
			x1, y1 := x0+float32(e.W), y0+float32(e.H)
			u0, v0 := float32(e.X)*inv, float32(e.Y)*inv
			u1, v1 := float32(e.X+e.W)*inv, float32(e.Y+e.H)*inv
			r.quad(x0, y0, x1, y1, u0, v0, u1, v1, col, [4]float32{}, [4]float32{}, [4]float32{})
		}
	}
	return ok
}

// Release frees all GPU resources.
func (r *Renderer) Release() {
	for id, t := range r.images {
		t.release()
		delete(r.images, id)
	}
	if r.blankImage != nil {
		r.blankImage.release()
		r.blankImage = nil
	}
	for _, res := range []interface{ Release() }{
		r.bindGroup, r.pipeline, r.layout, r.bgl, r.sampler,
		r.atlasView, r.atlasTex, r.uniforms, r.vbuf, r.ibuf,
		r.imgLinear, r.imgNearest, r.imageBGL,
	} {
		if res != nil && !reflect.ValueOf(res).IsNil() {
			res.Release()
		}
	}
}
