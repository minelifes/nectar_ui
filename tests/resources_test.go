package tests

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/resources"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func TestResourceSetLayers(t *testing.T) {
	parent := resources.NewSet(nil)
	parent.Mount("", fstest.MapFS{"a.txt": {Data: []byte("parent")}, "only-parent.txt": {Data: []byte("p")}})
	child := resources.NewSet(parent)
	unmount := child.Mount("", fstest.MapFS{"a.txt": {Data: []byte("child")}})
	child.Mount("charts", fstest.MapFS{"bar.json": {Data: []byte("{}")}})

	if d, _ := child.ReadFile("a.txt"); string(d) != "child" {
		t.Fatalf("child should win: %q", d)
	}
	if d, _ := child.ReadFile("only-parent.txt"); string(d) != "p" {
		t.Fatalf("parent files visible: %q", d)
	}
	if _, err := parent.ReadFile("charts/bar.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("child mounts leaked into the parent")
	}
	if err := fstest.TestFS(child, "a.txt", "only-parent.txt", "charts/bar.json"); err != nil {
		t.Fatal(err)
	}
	unmount()
	if d, _ := child.ReadFile("a.txt"); string(d) != "parent" {
		t.Fatalf("after unmount: %q", d)
	}
	if child.ID() == parent.ID() {
		t.Fatal("IDs must be unique")
	}
}

// readsResource shows a file from the resources visible where it's built.
type readsResource struct{ name string }

func (r readsResource) Build(ctx w.BuildContext) w.Widget {
	d, err := w.ResourcesOf(ctx).ReadFile(r.name)
	if err != nil {
		return w.Text{Text: "missing " + r.name}
	}
	return w.Text{Text: string(d)}
}

func TestAppAndScopedResources(t *testing.T) {
	w.Images.Clear()
	resources.Reset()
	defer resources.Reset()
	resources.Mount("", fstest.MapFS{"lib.txt": {Data: []byte("from-global")}})
	app := fstest.MapFS{
		"title.txt":      {Data: []byte("from-app")},
		"logo.png":       {Data: pngOf(20, 10)},
		"charts/bar.txt": {Data: []byte("app-bar")},
	}
	charts := fstest.MapFS{"bar.txt": {Data: []byte("scoped-bar")}, "logo.png": {Data: pngOf(40, 10)}}

	sizes := map[string]geom.Size{}
	img := func(id string) w.Widget {
		return w.Image{Source: w.AssetImage{Name: "logo.png"}, OnLoad: func(wd, h int) {
			sizes[id] = geom.Size{W: float32(wd), H: float32(h)}
		}}
	}
	tt := tester.New(w.Column{Children: []w.Widget{
		readsResource{"title.txt"},
		readsResource{"lib.txt"},
		readsResource{"charts/bar.txt"},
		img("app"),
		// A subtree with its own files, mounted at the root of its view.
		w.Resources{FS: charts, Child: w.Column{Children: []w.Widget{
			w.Resources{Prefix: "charts", FS: charts, Child: readsResource{"charts/bar.txt"}},
			readsResource{"title.txt"}, // outer (app) files still visible
			img("scoped"),
		}}},
	}}, 400, 400, tester.WithResources("", app))

	for _, want := range []string{"from-app", "from-global", "app-bar", "scoped-bar"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("%q not shown: %v", want, tt.Texts())
		}
	}
	// The same asset name resolves to different files in the two scopes,
	// and they don't share a cache entry.
	if !tt.PumpUntil(func() bool { return len(sizes) == 2 }, 5*time.Second) {
		t.Fatalf("images not loaded: %v", sizes)
	}
	if sizes["app"].W != 20 || sizes["scoped"].W != 40 {
		t.Fatalf("scoped asset mix-up: %v", sizes)
	}
	// App mounts don't leak into the global resources.
	if resources.Exists("title.txt") {
		t.Fatal("app resources leaked into the global Set")
	}
	// Evicting an AssetImage clears it from every scope.
	w.Images.Evict(w.AssetImage{Name: "logo.png"})
	if n, _ := w.Images.Stats(); n != 0 {
		t.Fatalf("%d cached images left after Evict", n)
	}
}

func TestResourcesScopeUnmountsOnDispose(t *testing.T) {
	show := true
	var set func(func())
	tt := tester.New(statefulHost{build: func(s func(func())) w.Widget {
		set = s
		if !show {
			return readsResource{"x.txt"}
		}
		return w.Resources{FS: fstest.MapFS{"x.txt": {Data: []byte("mounted")}}, Child: readsResource{"x.txt"}}
	}}, 200, 100)
	if _, ok := tt.Find("mounted"); !ok {
		t.Fatal("scoped file not visible")
	}
	set(func() { show = false })
	tt.Pump()
	if _, ok := tt.Find("missing x.txt"); !ok {
		t.Fatalf("scope files visible outside it: %v", tt.Texts())
	}
}
