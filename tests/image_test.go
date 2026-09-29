package tests

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/resources"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// pngOf encodes a wd×h image: left half red, right half blue.
func pngOf(wd, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, wd, h))
	for y := 0; y < h; y++ {
		for x := 0; x < wd; x++ {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= wd/2 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestResources(t *testing.T) {
	resources.Reset()
	defer resources.Reset()
	lib := fstest.MapFS{"images/a.png": {Data: []byte("lib")}, "x.txt": {Data: []byte("1")}}
	app := fstest.MapFS{"images/a.png": {Data: []byte("app")}, "images/b.png": {Data: []byte("b")}}
	resources.Mount("", lib)
	unmountApp := resources.Mount("", app)
	resources.Mount("plugins/chart", fstest.MapFS{"icon.png": {Data: []byte("p")}})

	if d, _ := resources.ReadFile("images/a.png"); string(d) != "app" {
		t.Fatalf("later mount should win, got %q", d)
	}
	if d, _ := resources.ReadFile("plugins/chart/icon.png"); string(d) != "p" {
		t.Fatalf("prefix mount: %q", d)
	}
	if !resources.Exists("plugins") || resources.Exists("nope.png") {
		t.Fatal("Exists")
	}
	entries, err := resources.ReadDir("images")
	if err != nil || len(entries) != 2 {
		t.Fatalf("merged ReadDir: %v %v", entries, err)
	}
	// The merged view is a correct fs.FS.
	if err := fstest.TestFS(resources.FS(), "images/a.png", "images/b.png", "x.txt", "plugins/chart/icon.png"); err != nil {
		t.Fatal(err)
	}
	unmountApp()
	if d, _ := resources.ReadFile("images/a.png"); string(d) != "lib" {
		t.Fatalf("after unmount: %q", d)
	}
	if _, err := resources.ReadFile("images/b.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unmounted file still there: %v", err)
	}
	if _, err := resources.ReadFile("../etc/passwd"); err == nil {
		t.Fatal("invalid path accepted")
	}
}

func TestApplyFit(t *testing.T) {
	img := geom.Size{W: 200, H: 100}
	box := geom.Rect{X: 0, Y: 0, W: 100, H: 100}
	for _, c := range []struct {
		fit      render.ImageFit
		src, dst geom.Rect
	}{
		{render.FitContain, geom.Rect{W: 200, H: 100}, geom.Rect{Y: 25, W: 100, H: 50}},
		{render.FitCover, geom.Rect{X: 50, W: 100, H: 100}, geom.Rect{W: 100, H: 100}},
		{render.FitFill, geom.Rect{W: 200, H: 100}, geom.Rect{W: 100, H: 100}},
		{render.FitNone, geom.Rect{X: 50, W: 100, H: 100}, geom.Rect{W: 100, H: 100}},
		{render.FitScaleDown, geom.Rect{W: 200, H: 100}, geom.Rect{Y: 25, W: 100, H: 50}},
		{render.FitHeight, geom.Rect{X: 50, W: 100, H: 100}, geom.Rect{W: 100, H: 100}},
	} {
		src, dst := render.ApplyFit(c.fit, img, box, geom.Center)
		if src != c.src || dst != c.dst {
			t.Errorf("fit %d: src %v dst %v, want %v %v", c.fit, src, dst, c.src, c.dst)
		}
	}
	// Alignment moves a letterboxed image.
	_, dst := render.ApplyFit(render.FitContain, img, box, geom.TopLeft)
	if dst.Y != 0 {
		t.Errorf("top-left contain dst %v", dst)
	}
	// Small image with ScaleDown keeps its size.
	_, dst = render.ApplyFit(render.FitScaleDown, geom.Size{W: 20, H: 10}, box, geom.Center)
	if dst.W != 20 || dst.H != 10 {
		t.Errorf("scale-down small %v", dst)
	}
}

// imageSize finds the laid-out size of the first image in the tree.
func imageSize(tt *tester.Tester) (geom.Size, *render.RenderImage) {
	var found *render.RenderImage
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if found != nil {
			return
		}
		if r, ok := ro.(*render.RenderImage); ok {
			found = r
			return
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	if found == nil {
		return geom.Size{}, nil
	}
	return found.Size(), found
}

func loadImage(t *testing.T, img w.Image, wd, h int) (*tester.Tester, geom.Size) {
	t.Helper()
	loaded := false
	img.OnLoad = func(int, int) { loaded = true }
	var failed error
	img.OnError = func(err error) { failed = err }
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: img}, wd, h)
	if !tt.PumpUntil(func() bool { return loaded || failed != nil }, 5*time.Second) {
		t.Fatal("image never loaded")
	}
	if failed != nil {
		t.Fatalf("load failed: %v", failed)
	}
	s, _ := imageSize(tt)
	return tt, s
}

func TestImageSourcesAndSizing(t *testing.T) {
	w.Images.Clear()
	data := pngOf(40, 20)

	_, s := loadImage(t, w.Image{Source: w.MemoryImage{Data: data}}, 400, 300)
	if s != (geom.Size{W: 40, H: 20}) {
		t.Fatalf("intrinsic size %v", s)
	}

	resources.Reset()
	defer resources.Reset()
	resources.Mount("", fstest.MapFS{"images/logo.png": {Data: data}})
	_, s = loadImage(t, w.Image{Source: w.AssetImage{Name: "images/logo.png"}, Width: 100}, 400, 300)
	if s != (geom.Size{W: 100, H: 50}) {
		t.Fatalf("width-only keeps aspect: %v", s)
	}

	path := filepath.Join(t.TempDir(), "pic.png")
	os.WriteFile(path, pngOf(400, 200), 0o644)
	_, s = loadImage(t, w.Image{Source: w.FileImage{Path: path}}, 100, 300)
	if s != (geom.Size{W: 100, H: 50}) {
		t.Fatalf("shrinks into constraints with aspect: %v", s)
	}
	_, s = loadImage(t, w.Image{Source: w.FileImage{Path: path}, Height: 30}, 400, 300)
	if s != (geom.Size{W: 60, H: 30}) {
		t.Fatalf("height-only: %v", s)
	}

	// Missing file: the Error builder is shown.
	var got error
	tt := tester.New(w.Image{Source: w.FileImage{Path: filepath.Join(t.TempDir(), "missing.png")},
		OnError: func(err error) { got = err },
		Error:   func(err error) w.Widget { return w.Text{Text: "broken image"} }}, 200, 200)
	if !tt.PumpUntil(func() bool { return got != nil }, 5*time.Second) || !errors.Is(got, fs.ErrNotExist) {
		t.Fatalf("missing file error: %v", got)
	}
	if _, ok := tt.Find("broken image"); !ok {
		t.Fatal("error widget not shown")
	}
}

// gateSource blocks until released and counts loads.
type gateSource struct {
	key   string
	data  []byte
	gate  chan struct{}
	loads *atomic.Int32
}

func (g gateSource) CacheKey() string { return g.key }
func (g gateSource) Load(ctx context.Context) ([]byte, error) {
	g.loads.Add(1)
	select {
	case <-g.gate:
		return g.data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestImagePlaceholderSharingAndCache(t *testing.T) {
	w.Images.Clear()
	var loads atomic.Int32
	src := gateSource{key: "gate-1", data: pngOf(30, 30), gate: make(chan struct{}), loads: &loads}
	var mu sync.Mutex
	loadedN := 0
	onLoad := func(int, int) { mu.Lock(); loadedN++; mu.Unlock() }
	tt := tester.New(w.Row{Children: []w.Widget{
		w.Image{Source: src, Width: 30, Height: 30, Placeholder: w.Text{Text: "loading…"}, OnLoad: onLoad},
		w.Image{Source: src, Width: 30, Height: 30, OnLoad: onLoad},
	}}, 200, 100)
	if _, ok := tt.Find("loading…"); !ok {
		t.Fatal("placeholder not shown while loading")
	}
	close(src.gate)
	if !tt.PumpUntil(func() bool { mu.Lock(); defer mu.Unlock(); return loadedN == 2 }, 5*time.Second) {
		t.Fatal("images didn't load")
	}
	if loads.Load() != 1 {
		t.Fatalf("two widgets, same source: %d loads, want 1 (shared)", loads.Load())
	}
	if _, ok := tt.Find("loading…"); ok {
		t.Fatal("placeholder still shown")
	}
	// A new widget with the cached source shows it in its first frame.
	if _, ok := w.Images.Get(src); !ok {
		t.Fatal("not cached")
	}
	tt2 := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.Image{Source: src}}, 100, 100)
	if s, r := imageSize(tt2); r == nil || r.Image == nil || s.W != 30 {
		t.Fatal("cached image not shown synchronously")
	}
	w.Images.Evict(src)
	if _, ok := w.Images.Get(src); ok {
		t.Fatal("Evict didn't remove it")
	}
}

func TestImageCacheLRU(t *testing.T) {
	c := w.NewImageCache(3 * 10 * 10 * 4) // room for three 10×10 images
	img := func() *render.Image { return render.NewImage(image.NewRGBA(image.Rect(0, 0, 10, 10))) }
	srcs := []w.MemoryImage{{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}}
	for _, s := range srcs[:3] {
		c.Put(s, img())
	}
	c.Get(srcs[0]) // a is now most recent
	c.Put(srcs[3], img())
	if _, ok := c.Get(srcs[1]); ok {
		t.Fatal("least recently used (b) should be evicted")
	}
	if _, ok := c.Get(srcs[0]); !ok {
		t.Fatal("recently used (a) was evicted")
	}
	if n, b := c.Stats(); n != 3 || b != 1200 {
		t.Fatalf("stats %d %d", n, b)
	}
}

func TestImageSourceChangeCancelsOld(t *testing.T) {
	w.Images.Clear()
	var loads atomic.Int32
	slow := gateSource{key: "slow", data: pngOf(10, 10), gate: make(chan struct{}), loads: &loads}
	fast := w.MemoryImage{Data: pngOf(50, 20), Key: "fast"}
	var set func(func())
	var src w.ImageSource = slow
	tt := tester.New(w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		return statefulHost{build: func(s func(func())) w.Widget { set = s; return w.Align{Alignment: geom.TopLeft, Child: w.Image{Source: src}} }}
	}}, 200, 200)
	set(func() { src = fast })
	if !tt.PumpUntil(func() bool { s, _ := imageSize(tt); return s.W == 50 }, 5*time.Second) {
		t.Fatal("new source not shown")
	}
	close(slow.gate) // the old load finishing must not replace the new image
	time.Sleep(20 * time.Millisecond)
	tt.Pump()
	if s, _ := imageSize(tt); s.W != 50 {
		t.Fatalf("stale load replaced the image: %v", s)
	}
}

// statefulHost lets a test rebuild with new values.
type statefulHost struct{ build func(set func(func())) w.Widget }

func (statefulHost) CreateState() w.State { return &statefulHostState{} }

type statefulHostState struct{ w.StateBase }

func (s *statefulHostState) Build(w.BuildContext) w.Widget {
	return w.WidgetOf[statefulHost](s).build(func(f func()) { s.SetState(f) })
}

func TestNetworkImage(t *testing.T) {
	w.Images.Clear()
	data := pngOf(16, 8)
	type seen struct {
		method, auth, ctype, body string
	}
	var mu sync.Mutex
	var reqs []seen
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, seen{r.Method, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)})
		mu.Unlock()
		switch r.URL.Path {
		case "/missing":
			http.NotFound(rw, r)
		case "/to-http":
			http.Redirect(rw, r, "http://127.0.0.1:1/pic.png", http.StatusFound)
		default:
			rw.Header().Set("Content-Type", "image/png")
			rw.Write(data)
		}
	})
	plain := httptest.NewServer(handler)
	defer plain.Close()
	secure := httptest.NewTLSServer(handler)
	defer secure.Close()
	ctx := context.Background()

	// Plain http is refused unless allowed.
	if _, err := (w.NetworkImage{URL: plain.URL + "/pic.png"}).Load(ctx); !errors.Is(err, w.ErrInsecureURL) {
		t.Fatalf("http without AllowHTTP: %v", err)
	}
	if got, err := (w.NetworkImage{URL: plain.URL + "/pic.png", AllowHTTP: true}).Load(ctx); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("AllowHTTP: %v", err)
	}
	// Custom headers and a POST body.
	src := w.NetworkImage{URL: secure.URL + "/render", Client: secure.Client(),
		Headers: map[string]string{"Authorization": "Bearer t0k", "Content-Type": "application/json"},
		Body:    []byte(`{"w":16}`)}
	if _, err := src.Load(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	last := reqs[len(reqs)-1]
	mu.Unlock()
	if last != (seen{"POST", "Bearer t0k", "application/json", `{"w":16}`}) {
		t.Fatalf("request %+v", last)
	}
	// Explicit method with a body.
	if _, err := (w.NetworkImage{URL: secure.URL + "/p", Client: secure.Client(), Method: "put", Body: []byte("x")}).Load(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if m := reqs[len(reqs)-1].method; m != "PUT" {
		t.Fatalf("method %s", m)
	}
	mu.Unlock()
	// HTTP errors and https→http redirects fail.
	if _, err := (w.NetworkImage{URL: secure.URL + "/missing", Client: secure.Client()}).Load(ctx); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404: %v", err)
	}
	if _, err := (w.NetworkImage{URL: secure.URL + "/to-http", Client: secure.Client()}).Load(ctx); !errors.Is(err, w.ErrInsecureURL) {
		t.Fatalf("redirect to http: %v", err)
	}
	// Size limit.
	if _, err := (w.NetworkImage{URL: secure.URL + "/pic.png", Client: secure.Client(), MaxBytes: 10}).Load(ctx); err == nil {
		t.Fatal("MaxBytes not enforced")
	}
	// Different headers or bodies are different cache entries.
	a := w.NetworkImage{URL: "https://x/a", Headers: map[string]string{"Authorization": "1"}}
	b := w.NetworkImage{URL: "https://x/a", Headers: map[string]string{"Authorization": "2"}}
	if a.CacheKey() == b.CacheKey() || a.CacheKey() != (w.NetworkImage{URL: "https://x/a", Headers: map[string]string{"authorization": "1"}}).CacheKey() {
		t.Fatal("cache keys")
	}

	// Through the widget.
	_, s := loadImage(t, w.Image{Source: w.NetworkImage{URL: secure.URL + "/pic.png", Client: secure.Client()}}, 200, 200)
	if s != (geom.Size{W: 16, H: 8}) {
		t.Fatalf("network image size %v", s)
	}
}

// TestImagePixels renders through the GPU path (CPU adapter): scaled,
// cropped, rounded and pixelated images must land where expected.
func TestImagePixels(t *testing.T) {
	w.Images.Clear()
	big := w.MemoryImage{Data: pngOf(400, 200), Key: "big"} // shrunk a lot
	odd := w.MemoryImage{Data: pngOf(37, 23), Key: "odd"}   // odd row size
	tt, _ := loadImage(t, w.Image{Source: big, Width: 40, Height: 20}, 160, 160)
	tt.Close()
	w.Images.Clear()
	loaded := 0
	on := func(int, int) { loaded++ }
	tt = tester.New(w.Column{Cross: w.CrossStart, Children: []w.Widget{
		w.Row{Children: []w.Widget{
			w.Image{Source: big, Width: 40, Height: 20, OnLoad: on},
			w.SizedBox{Width: 10},
			w.Image{Source: odd, Width: 74, Height: 46, Pixelated: true, OnLoad: on},
		}},
		w.SizedBox{Height: 10},
		// Cover-cropped into a circle: a 200×100 area of the middle.
		w.Image{Source: big, Width: 80, Height: 80, Fit: w.FitCover, Radius: 40, OnLoad: on},
	}}, 160, 160)
	defer tt.Close()
	if !tt.PumpUntil(func() bool { return loaded == 3 }, 5*time.Second) {
		t.Fatal("not loaded")
	}
	snap, err := tt.Snapshot()
	if err != nil {
		t.Skip("no GPU adapter:", err)
	}
	red, blue, white := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{255, 255, 255, 255}
	check := func(name string, x, y int, want color.RGBA) {
		g := snap.RGBAAt(x, y)
		d := func(a, b uint8) int {
			if a > b {
				return int(a - b)
			}
			return int(b - a)
		}
		if d(g.R, want.R) > 40 || d(g.G, want.G) > 40 || d(g.B, want.B) > 40 {
			t.Errorf("%s at (%d,%d) = %v, want ~%v", name, x, y, g, want)
		}
	}
	for _, p := range []struct {
		name string
		x, y int
		c    color.RGBA
	}{
		{"big top-left", 2, 2, red}, {"big bottom-left", 2, 18, red},
		{"big top-right", 38, 2, blue}, {"big bottom-right", 38, 18, blue},
		{"odd top-left", 52, 2, red}, {"odd bottom-left", 52, 44, red},
		{"odd top-right", 122, 2, blue}, {"odd bottom-right", 122, 44, blue},
		{"circle corner (clipped)", 2, 32, white},
		{"circle left", 20, 70, red}, {"circle right", 60, 70, blue},
	} {
		check(p.name, p.x, p.y, p.c)
	}
}
