package tests

import (
	"context"
	"image"
	"image/png"
	"github.com/minelifes/nectar_ui/ui/gpu"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
	"os"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends" // software fallback for CI

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// renderOffscreen draws app into an RGBA texture and reads it back.
// Skips when no GPU adapter (not even the software one) is available.
func renderOffscreen(t *testing.T, app widgets.Widget, w, h int, scale float32) *image.RGBA {
	t.Helper()
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsAll})
	if err != nil {
		t.Skipf("no wgpu instance: %v", err)
	}
	t.Cleanup(inst.Release)
	// Prefer a real GPU; fall back to the CPU (software) adapter in CI.
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: os.Getenv("NECTAR_REAL_GPU") == ""})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	dev, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("no device: %v", err)
	}
	t.Cleanup(dev.Release)

	const format = gputypes.TextureFormatRGBA8Unorm
	r, err := gpu.New(dev, format)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(r.Release)

	pw, ph := uint32(float32(w)*scale), uint32(float32(h)*scale)
	tex, err := dev.CreateTexture(&wgpu.TextureDescriptor{
		Size: wgpu.Extent3D{Width: pw, Height: ph, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: format,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tex.Release()
	view, err := dev.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()

	// widget tree -> render tree -> display list
	bo := widgets.NewBuildOwner()
	po := render.NewPipelineOwner()
	widgets.Mount(app, geom.Transparent, bo, po)
	bo.FlushBuild()
	size := geom.Size{W: float32(w), H: float32(h)}
	po.FlushLayout(size)
	canvas := po.FlushPaint(size)

	enc, err := dev.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Draw(gpu.Frame{Encoder: enc, Target: view, Width: pw, Height: ph, Scale: scale,
		Clear: geom.Hex(0xf4f1ea), Commands: canvas.Commands})
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	enc.TransitionTextures([]wgpu.TextureBarrier{{Texture: tex, Usage: wgpu.TextureUsageTransition{
		OldUsage: gputypes.TextureUsageRenderAttachment, NewUsage: gputypes.TextureUsageCopySrc}}})

	rowBytes := (pw*4 + 255) &^ 255
	staging, err := dev.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(rowBytes * ph), Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Release()
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: rowBytes, RowsPerImage: ph},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: pw, Height: ph, DepthOrArrayLayers: 1},
	}})
	cb, err := enc.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dev.Queue().Submit(cb); err != nil {
		t.Fatal(err)
	}
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, uint64(rowBytes*ph)); err != nil {
		t.Fatal(err)
	}
	rng, err := staging.MappedRange(0, uint64(rowBytes*ph))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, int(pw), int(ph)))
	src := rng.Bytes()
	for y := 0; y < int(ph); y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+int(pw)*4], src[y*int(rowBytes):])
	}
	staging.Unmap()

	if out := os.Getenv("NECTAR_SNAPSHOT"); out != "" {
		f, err := os.Create(out)
		if err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
	}
	return img
}

func TestRenderRectAndText(t *testing.T) {
	app := widgets.Container{
		Padding: geom.Insets(20),
		Child: widgets.Column{Spacing: 12, Cross: widgets.CrossStart, Children: []widgets.Widget{
			widgets.Container{Width: 80, Height: 40, Color: geom.Hex(0xff0000), Border: &geom.Border{Radius: 8}},
			widgets.Text{Text: "Hello", Style: text.Style{Size: 32, Color: geom.Hex(0x0000ff)}},
		}},
	}
	img := renderOffscreen(t, app, 200, 140, 1)

	// Center of the red box.
	if c := img.RGBAAt(60, 40); c.R < 240 || c.G > 20 || c.B > 20 {
		t.Fatalf("expected red at box center, got %v", c)
	}
	// Background.
	if c := img.RGBAAt(190, 130); c.R != 0xf4 || c.G != 0xf1 {
		t.Fatalf("expected clear color, got %v", c)
	}
	// Some bluish text pixels in the text band.
	blue := 0
	for y := 72; y < 120; y++ {
		for x := 20; x < 140; x++ {
			if c := img.RGBAAt(x, y); c.B > 150 && c.R < 100 {
				blue++
			}
		}
	}
	if blue < 50 {
		t.Fatalf("expected rendered text pixels, found %d", blue)
	}
}
