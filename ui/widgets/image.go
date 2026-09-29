package widgets

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// Image fit modes (how the picture fills the widget's box).
const (
	FitContain   = render.FitContain   // fully visible, letterboxed (default)
	FitCover     = render.FitCover     // fills the box, cropped
	FitFill      = render.FitFill      // stretched
	FitWidth     = render.FitWidth     // full width
	FitHeight    = render.FitHeight    // full height
	FitNone      = render.FitNone      // natural size, cropped
	FitScaleDown = render.FitScaleDown // natural size, shrunk to fit
)

// Image shows a picture from a file, the app's resources, memory or the
// network. Loading and decoding happen in the background; decoded images
// are cached (Images) and shared by every widget showing the same source.
//
//	widgets.Image{Source: widgets.AssetImage{Name: "images/logo.png"}, Width: 120}
//	widgets.Image{Source: widgets.FileImage{Path: path}, Fit: widgets.FitCover, Radius: 12}
//	widgets.Image{Source: widgets.NetworkImage{URL: u}, Width: 64, Height: 64,
//	    Placeholder: material.CircularProgressIndicator{}}
//
// Size: with Width and Height both 0 the widget takes the image's pixel
// size (1 px = 1 logical px), shrunk to fit its constraints; with one set,
// the other follows the aspect ratio. Set both to reserve space while
// loading.
type Image struct {
	Source ImageSource
	Width  float32
	Height float32
	Fit    render.ImageFit
	// Alignment places the image inside its box (default center).
	Alignment *geom.Alignment
	// Radius rounds the corners of the visible picture.
	Radius float32
	// Opacity 0..1; 0 means fully opaque (the default).
	Opacity float32
	// Pixelated uses nearest-neighbour sampling (pixel art, QR codes).
	Pixelated bool
	// FadeIn animates the picture in once it's loaded (0 = appear at once;
	// images already in the cache never fade).
	FadeIn time.Duration

	// Placeholder is shown (at Width×Height) while loading.
	Placeholder Widget
	// Error builds what to show if loading fails (default: nothing).
	Error func(err error) Widget
	// OnLoad / OnError report the result.
	OnLoad  func(width, height int)
	OnError func(err error)

	// Cache to use; default Images.
	Cache *ImageCache
}

func (Image) CreateState() State { return &imageState{} }

type imageState struct {
	StateBase
	key    string
	img    *render.Image
	err    error
	cancel func()
	gen    int
	fade   *Animated
}

func (s *imageState) widget() Image { return WidgetOf[Image](s) }

func (s *imageState) cache() *ImageCache {
	if c := s.widget().Cache; c != nil {
		return c
	}
	return Images
}

func (s *imageState) InitState() {
	s.fade = NewAnimated(s, s.widget().FadeIn, EaseOut, 1)
	s.start()
}

// source returns the widget's source, bound to this place in the tree.
func (s *imageState) source() ImageSource {
	src := s.widget().Source
	if cs, ok := src.(ContextImageSource); ok {
		return cs.Bind(s.Context())
	}
	return src
}

func (s *imageState) DidUpdateWidget(Widget) {
	key := ""
	if src := s.source(); src != nil {
		key = src.CacheKey()
	}
	if key != s.key {
		s.start()
	}
}

func (s *imageState) Dispose() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// start (re)loads the current source.
func (s *imageState) start() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	src := s.source()
	s.gen++
	s.img, s.err, s.key = nil, nil, ""
	if src == nil {
		return
	}
	s.key = src.CacheKey()
	// Already decoded: show it in this very frame, without fading.
	if img, ok := s.cache().get(s.key); ok {
		s.img = img
		s.notify(img, nil)
		return
	}
	gen := s.gen
	s.cancel = s.cache().Load(src, func(img *render.Image, err error) {
		s.Post(func() {
			if gen != s.gen {
				return // a newer source replaced this one
			}
			s.cancel = nil
			s.img, s.err = img, err
			if img != nil && s.widget().FadeIn > 0 {
				s.fade.Jump(0)
				s.fade.Set(1)
			}
			s.notify(img, err)
		})
	})
}

func (s *imageState) notify(img *render.Image, err error) {
	w := s.widget()
	if img != nil && w.OnLoad != nil {
		w.OnLoad(img.Width(), img.Height())
	}
	if err != nil && w.OnError != nil {
		w.OnError(err)
	}
}

func (s *imageState) Build(BuildContext) Widget {
	w := s.widget()
	if s.err != nil && w.Error != nil {
		return sizedIf(w.Width, w.Height, w.Error(s.err))
	}
	if s.img == nil && s.err == nil && w.Placeholder != nil {
		return sizedIf(w.Width, w.Height, w.Placeholder)
	}
	align := geom.Center
	if w.Alignment != nil {
		align = *w.Alignment
	}
	opacity := w.Opacity
	if opacity <= 0 {
		opacity = 1
	}
	if w.FadeIn > 0 {
		opacity *= s.fade.Value()
	}
	return imageBox{img: s.img, width: w.Width, height: w.Height, fit: w.Fit, align: align,
		radius: w.Radius, opacity: opacity, nearest: w.Pixelated}
}

func sizedIf(w, h float32, child Widget) Widget {
	if w == 0 && h == 0 {
		return child
	}
	return SizedBox{Width: w, Height: h, Child: child}
}

// imageBox is the render-object widget behind Image.
type imageBox struct {
	img           *render.Image
	width, height float32
	fit           render.ImageFit
	align         geom.Alignment
	radius        float32
	opacity       float32
	nearest       bool
}

func (b imageBox) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderImage{}
	b.UpdateRenderObject(nil, r)
	return r
}

func (b imageBox) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderImage)
	relayout := r.Image != b.img || r.Width != b.width || r.Height != b.height
	repaint := r.Fit != b.fit || r.Align != b.align || r.Radius != b.radius || r.Opacity != b.opacity || r.Nearest != b.nearest
	r.Image, r.Width, r.Height = b.img, b.width, b.height
	r.Fit, r.Align, r.Radius, r.Opacity, r.Nearest = b.fit, b.align, b.radius, b.opacity, b.nearest
	switch {
	case relayout:
		render.MarkNeedsLayout(r)
	case repaint:
		render.MarkNeedsPaint(r)
	}
}
