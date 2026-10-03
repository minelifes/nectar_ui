// Package tester runs a widget tree headlessly for tests, like Flutter's
// WidgetTester: it drives the same build → layout → paint pipeline as
// ui.App, lets you tap, drag, type and advance a fake clock, and can render
// frames to an image with the wgpu CPU fallback adapter.
//
//	t := tester.New(MyApp{}, 400, 300)
//	t.Tap(100, 40)
//	t.Advance(300 * time.Millisecond) // run animations
//	img, _ := t.Snapshot()
package tester

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends" // CPU fallback adapter

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/gpu"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/resources"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Tester hosts a widget tree without a window.
type Tester struct {
	Build    *widgets.BuildOwner
	Pipeline *render.PipelineOwner
	Root     *widgets.Root
	Size     geom.Size
	Scale    float32
	Clear    geom.Color
	// Window is what widgets.WindowOf returns: SetSize changes Size.
	Window *Window
	// Resources are the app's resources (see WithResources), layered over
	// the global ones.
	Resources *resources.Set

	disp      *render.PointerDispatcher
	now       time.Time
	clipboard string
	composing bool

	dev      *wgpu.Device
	inst     *wgpu.Instance
	adapter  *wgpu.Adapter
	renderer *gpu.Renderer
}

// Option configures New.
type Option func(*Tester)

// WithResources mounts fsys under prefix into the tester's resources
// before the first frame, like ui.Config.WithResources.
func WithResources(prefix string, fsys fs.FS) Option {
	return func(t *Tester) { t.Resources.Mount(prefix, fsys) }
}

// WithTitleBar makes Window.TitleBar report info from the first frame,
// e.g. tester.MacTitleBar or tester.FramelessTitleBar.
func WithTitleBar(info widgets.TitleBarInfo) Option {
	return func(t *Tester) { t.Window.TitleBarInfo = info }
}

// New mounts app in a w×h (logical px) surface and runs the first frame.
func New(app widgets.Widget, w, h int, opts ...Option) *Tester {
	t := &Tester{
		Build:    widgets.NewBuildOwner(),
		Pipeline: render.NewPipelineOwner(),
		Size:     geom.Size{W: float32(w), H: float32(h)},
		Scale:    1,
		Clear:    geom.White,
		disp:     render.NewPointerDispatcher(),
		now:      time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	t.Build.Now = func() time.Time { return t.now }
	t.Build.Clipboard = (*memClipboard)(t)
	t.Window = &Window{t: t}
	t.Build.Window = t.Window
	t.Resources = resources.NewSet(resources.Global())
	t.Build.Resources = t.Resources
	for _, o := range opts {
		o(t)
	}
	t.Root = widgets.Mount(app, geom.Transparent, t.Build, t.Pipeline)
	t.Pump()
	return t
}

type memClipboard Tester

func (c *memClipboard) ReadText() (string, error) { return c.clipboard, nil }
func (c *memClipboard) WriteText(s string) error  { c.clipboard = s; return nil }

// Clipboard returns what copy/cut put on the clipboard.
func (t *Tester) Clipboard() string { return t.clipboard }

// Pump runs one frame: build, layout, paint.
func (t *Tester) Pump() *render.Canvas {
	t0 := time.Now()
	t.Build.FlushBuild()
	t1 := time.Now()
	t.Pipeline.FlushLayout(t.Size)
	t2 := time.Now()
	c := t.Pipeline.FlushPaint(t.Size)
	t.Build.RecordFrame(t1.Sub(t0), t2.Sub(t1), time.Since(t2), len(c.Commands))
	return c
}

// Advance moves the clock forward in 16ms frames, running animations.
func (t *Tester) Advance(d time.Duration) {
	const frame = 16 * time.Millisecond
	for d > 0 {
		step := min(frame, d)
		t.now = t.now.Add(step)
		d -= step
		t.Pump()
	}
	t.Pump()
}

// Settle runs a frame (building what was just inserted, like a popup) and
// advances until no animation is running (up to 5s).
func (t *Tester) Settle() {
	t.Pump()
	for i := 0; i < 320 && t.Build.HasActiveTickers(); i++ {
		t.Advance(16 * time.Millisecond)
	}
}

// Pointer dispatches one pointer event and runs a frame.
func (t *Tester) Pointer(kind render.PointerKind, x, y float32) {
	t.Dispatch(render.PointerEvent{Kind: kind, ID: 1, Position: geom.Pt(x, y), Button: render.ButtonPrimary})
}

// Dispatch delivers a pointer event as the app would (any button, with
// modifiers) and runs a frame.
func (t *Tester) Dispatch(e render.PointerEvent) {
	fm := t.Build.Focus()
	if e.Kind == render.PointerDown {
		fm.BeginPointerDown()
	}
	t.disp.Dispatch(t.Pipeline.Root(), e)
	if e.Kind == render.PointerDown {
		fm.EndPointerDown()
	}
	t.Pump()
}

// SecondaryTap right-clicks at (x, y).
func (t *Tester) SecondaryTap(x, y float32) {
	e := render.PointerEvent{Kind: render.PointerDown, ID: 1, Position: geom.Pt(x, y), Button: render.ButtonSecondary}
	t.Dispatch(e)
	e.Kind = render.PointerUp
	t.Dispatch(e)
}

// Tap presses and releases at (x, y).
func (t *Tester) Tap(x, y float32) {
	t.Pointer(render.PointerDown, x, y)
	t.Pointer(render.PointerUp, x, y)
}

// Drag presses at from, moves in steps to to, and releases.
func (t *Tester) Drag(from, to geom.Offset) {
	t.Pointer(render.PointerDown, from.X, from.Y)
	const steps = 8
	for i := 1; i <= steps; i++ {
		p := geom.Pt(widgets.Lerp(from.X, to.X, float32(i)/steps), widgets.Lerp(from.Y, to.Y, float32(i)/steps))
		t.Pointer(render.PointerMove, p.X, p.Y)
	}
	t.Pointer(render.PointerUp, to.X, to.Y)
}

// Hover moves the mouse (no buttons) to (x, y).
func (t *Tester) Hover(x, y float32) { t.Pointer(render.PointerHover, x, y) }

// Scroll sends a vertical wheel event at (x, y).
func (t *Tester) Scroll(x, y, dy float32) { t.ScrollXY(x, y, 0, dy) }

// ScrollXY sends a wheel / trackpad event with both deltas at (x, y).
func (t *Tester) ScrollXY(x, y, dx, dy float32) {
	t.disp.Dispatch(t.Pipeline.Root(), render.PointerEvent{Kind: render.PointerScroll, Position: geom.Pt(x, y), Scroll: geom.Pt(dx, dy), Button: -1})
	t.Pump()
}

// Key presses a key.
func (t *Tester) Key(k widgets.KeyCode, mods ...widgets.Modifiers) {
	var m widgets.Modifiers
	for _, x := range mods {
		m |= x
	}
	t.Build.Focus().HandleKey(widgets.KeyEvent{Key: k, Mods: m})
	t.Pump()
}

// DragFiles simulates files from the OS file manager dragged in at the
// first point, moved through the others, and dropped at the last (with
// drop false they leave the window instead).
func (t *Tester) DragFiles(paths []string, drop bool, points ...geom.Offset) {
	if len(points) == 0 {
		return
	}
	t.Build.FileDragEnter(paths, points[0])
	t.Pump()
	for _, p := range points[1:] {
		t.Build.FileDragMove(p)
		t.Pump()
	}
	if drop {
		t.DropFiles(paths, points[len(points)-1].X, points[len(points)-1].Y)
	} else {
		t.Build.FileDragLeave()
		t.Pump()
	}
}

// DropFiles simulates files dropped from the OS file manager at (x, y).
func (t *Tester) DropFiles(paths []string, x, y float32) bool {
	ok := t.Build.DropFiles(paths, geom.Pt(x, y))
	t.Pump()
	return ok
}

// Type sends text input to the focused widget.
func (t *Tester) Type(s string) {
	t.Build.Focus().HandleText(s)
	t.Pump()
}

// Texts returns the strings of all Text/paragraphs currently painted.
func (t *Tester) Texts() []string {
	var out []string
	t.Pump()
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		switch r := ro.(type) {
		case *render.RenderOffstage:
			if r.Offstage {
				return
			}
		case *render.RenderParagraph:
			out = append(out, r.Text())
		case *render.RenderEditable:
			out = append(out, r.Text)
		}
		ro.VisitChildren(walk)
	}
	if root := t.Pipeline.Root(); root != nil {
		walk(root)
	}
	return out
}

// Find returns the window rect of the topmost (last painted) visible
// paragraph whose text is s: the one a user would click. Text that is
// clipped away (collapsed, scrolled out, zero opacity) is skipped.
func (t *Tester) Find(s string) (geom.Rect, bool) {
	var found geom.Rect
	ok := false
	var walk func(ro render.RenderObject, clip geom.Rect)
	walk = func(ro render.RenderObject, clip geom.Rect) {
		r := geom.RectFrom(render.GlobalOrigin(ro), ro.Base().Size())
		switch x := ro.(type) {
		case *render.RenderOpacity:
			if x.Opacity <= 0 {
				return
			}
		case *render.RenderOffstage:
			if x.Offstage {
				return
			}
		case *render.RenderSizeFactor, *render.RenderClipRect, *render.RenderViewport:
			clip = clip.Intersect(r)
		case *render.RenderParagraph:
			if x.Text() == s && !clip.Intersect(r).Empty() {
				found, ok = r, true
			}
		}
		if clip.Empty() {
			return
		}
		ro.VisitChildren(func(c render.RenderObject) { walk(c, clip) })
	}
	if root := t.Pipeline.Root(); root != nil {
		walk(root, geom.Rect{W: t.Size.W, H: t.Size.H})
	}
	return found, ok
}

// TapText taps the center of the topmost paragraph showing s.
func (t *Tester) TapText(s string) error {
	r, ok := t.Find(s)
	if !ok {
		return fmt.Errorf("tester: text %q not found", s)
	}
	t.Tap(r.X+r.W/2, r.Y+r.H/2)
	return nil
}

// ---------------------------------------------------------------------------
// Rendering

func (t *Tester) initGPU() error {
	if t.renderer != nil {
		return nil
	}
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsAll})
	if err != nil {
		return err
	}
	ad, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: os.Getenv("NECTAR_REAL_GPU") == ""})
	if err != nil {
		inst.Release()
		return err
	}
	dev, err := ad.RequestDevice(nil)
	if err != nil {
		ad.Release()
		inst.Release()
		return err
	}
	r, err := gpu.New(dev, gputypes.TextureFormatRGBA8Unorm)
	if err != nil {
		return err
	}
	t.inst, t.adapter, t.dev, t.renderer = inst, ad, dev, r
	return nil
}

// Snapshot renders the current frame to an image.
func (t *Tester) Snapshot() (*image.RGBA, error) {
	if err := t.initGPU(); err != nil {
		return nil, err
	}
	canvas := t.Pump()
	dev := t.dev
	pw, ph := uint32(t.Size.W*t.Scale), uint32(t.Size.H*t.Scale)
	tex, err := dev.CreateTexture(&wgpu.TextureDescriptor{
		Size: wgpu.Extent3D{Width: pw, Height: ph, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		return nil, err
	}
	defer tex.Release()
	view, err := dev.CreateTextureView(tex, nil)
	if err != nil {
		return nil, err
	}
	defer view.Release()
	enc, err := dev.CreateCommandEncoder(nil)
	if err != nil {
		return nil, err
	}
	if err := t.renderer.Draw(gpu.Frame{Encoder: enc, Target: view, Width: pw, Height: ph, Scale: t.Scale,
		Clear: t.Clear, Commands: canvas.Commands}); err != nil {
		return nil, err
	}
	enc.TransitionTextures([]wgpu.TextureBarrier{{Texture: tex, Usage: wgpu.TextureUsageTransition{
		OldUsage: gputypes.TextureUsageRenderAttachment, NewUsage: gputypes.TextureUsageCopySrc}}})
	row := (pw*4 + 255) &^ 255
	staging, err := dev.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(row * ph), Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst})
	if err != nil {
		return nil, err
	}
	defer staging.Release()
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: row, RowsPerImage: ph},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: pw, Height: ph, DepthOrArrayLayers: 1},
	}})
	cb, err := enc.Finish()
	if err != nil {
		return nil, err
	}
	if _, err := dev.Queue().Submit(cb); err != nil {
		return nil, err
	}
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, uint64(row*ph)); err != nil {
		return nil, err
	}
	rng, err := staging.MappedRange(0, uint64(row*ph))
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, int(pw), int(ph)))
	src := rng.Bytes()
	for y := 0; y < int(ph); y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+int(pw)*4], src[y*int(row):])
	}
	_ = staging.Unmap()
	return img, nil
}

// SavePNG renders and writes the frame to path.
func (t *Tester) SavePNG(path string) error {
	img, err := t.Snapshot()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// Close releases GPU resources.
func (t *Tester) Close() {
	if t.renderer != nil {
		t.renderer.Release()
		t.dev.Release()
		t.adapter.Release()
		t.inst.Release()
		t.renderer = nil
	}
}

// Semantics returns every annotated part of the screen (see
// widgets.SemanticsTree), depth first.
func (t *Tester) Semantics() []*widgets.SemanticsNode {
	t.Pump()
	return widgets.FlattenSemantics(widgets.SemanticsTree(t.Pipeline.Root()))
}

// FindSemantics returns the topmost node with the given role ("" = any)
// and label.
func (t *Tester) FindSemantics(role, label string) (*widgets.SemanticsNode, bool) {
	var found *widgets.SemanticsNode
	for _, n := range t.Semantics() {
		if (role == "" || n.Role == role) && n.Label == label {
			found = n
		}
	}
	return found, found != nil
}
