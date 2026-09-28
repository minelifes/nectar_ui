package tests

import (
	"github.com/minelifes/nectar_ui/ui/widgets"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

var builds = map[string]int{}

type theme struct{ Accent string }

// reader depends on Provider[theme]; plain doesn't.
type reader struct{ Name string }

func (r reader) Build(ctx widgets.BuildContext) widgets.Widget {
	builds[r.Name]++
	t, _ := widgets.Of[theme](ctx)
	return widgets.Text{Text: r.Name + ":" + t.Accent}
}

type plain struct{ Name string }

func (p plain) Build(widgets.BuildContext) widgets.Widget {
	builds[p.Name]++
	return widgets.Text{Text: p.Name}
}

type peeker struct{ Name string }

func (p peeker) Build(ctx widgets.BuildContext) widgets.Widget {
	builds[p.Name]++
	t, _ := widgets.Peek[theme](ctx)
	return widgets.Text{Text: p.Name + ":" + t.Accent}
}

func themed(accent string) widgets.Widget {
	return widgets.Provider[theme]{Value: theme{accent}, Child: widgets.Column{Children: []widgets.Widget{
		reader{Name: "r"}, plain{Name: "p"}, peeker{Name: "k"},
	}}}
}

func TestProviderRebuildsOnlyDependents(t *testing.T) {
	clear(builds)
	bo := widgets.NewBuildOwner()
	po := render.NewPipelineOwner()
	root := widgets.Mount(themed("red"), geom.White, bo, po)

	// Column holds a slice, so it's re-updated; its children are equal
	// comparable structs and are skipped unless they depend on the theme.
	root.SetApp(themed("blue"), geom.White)
	bo.FlushBuild()
	if builds["r"] != 2 || builds["p"] != 1 || builds["k"] != 1 {
		t.Fatalf("builds = %v, want r:2 p:1 k:1", builds)
	}
	if got := widgets.paragraphs(po); got[0] != "r:blue" || got[2] != "k:red" {
		t.Fatalf("got %v", got)
	}

	// Same value: nobody rebuilds.
	root.SetApp(themed("blue"), geom.White)
	bo.FlushBuild()
	if builds["r"] != 2 {
		t.Fatalf("unchanged value rebuilt dependent: %v", builds)
	}
}

func TestProviderShadowingAndMissing(t *testing.T) {
	_, _, po := widgets.setup(widgets.Provider[theme]{Value: theme{"outer"}, Child: widgets.Column{Children: []widgets.Widget{
		reader{Name: "a"},
		widgets.Provider[theme]{Value: theme{"inner"}, Child: reader{Name: "b"}},
		widgets.Provider[int]{Value: 7, Child: reader{Name: "c"}}, // different T doesn't shadow
	}}})
	got := widgets.paragraphs(po)
	if got[0] != "a:outer" || got[1] != "b:inner" || got[2] != "c:outer" {
		t.Fatalf("got %v", got)
	}

	_, _, po = widgets.setup(reader{Name: "none"})
	if got := widgets.paragraphs(po); got[0] != "none:" {
		t.Fatalf("missing provider should give zero value, got %v", got)
	}
}

// A hand-written InheritedWidget with a custom notify rule.
type locale struct {
	Lang  string
	Debug int // changes here don't matter to dependents
	Child widgets.Widget
}

func (l locale) ChildWidget() widgets.Widget                { return l.Child }
func (l locale) UpdateShouldNotify(old widgets.Widget) bool { return old.(locale).Lang != l.Lang }

type greeter struct{}

func (greeter) Build(ctx widgets.BuildContext) widgets.Widget {
	builds["g"]++
	l, _ := widgets.DependOn[locale](ctx)
	return widgets.Text{Text: map[string]string{"en": "hello", "uk": "привіт"}[l.Lang]}
}

func TestCustomInheritedWidget(t *testing.T) {
	clear(builds)
	root, bo, po := widgets.setup(locale{Lang: "en", Child: greeter{}})
	root.SetApp(locale{Lang: "en", Debug: 1, Child: greeter{}}, geom.White)
	bo.FlushBuild()
	if builds["g"] != 1 {
		t.Fatalf("UpdateShouldNotify=false still rebuilt: %d", builds["g"])
	}
	root.SetApp(locale{Lang: "uk", Child: greeter{}}, geom.White)
	bo.FlushBuild()
	if got := widgets.paragraphs(po); got[0] != "привіт" || builds["g"] != 2 {
		t.Fatalf("got %v builds %d", got, builds["g"])
	}
}
