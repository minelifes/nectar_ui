package widgets

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

var builds = map[string]int{}

type theme struct{ Accent string }

// reader depends on Provider[theme]; plain doesn't.
type reader struct{ Name string }

func (r reader) Build(ctx BuildContext) Widget {
	builds[r.Name]++
	t, _ := Of[theme](ctx)
	return Text{Text: r.Name + ":" + t.Accent}
}

type plain struct{ Name string }

func (p plain) Build(BuildContext) Widget {
	builds[p.Name]++
	return Text{Text: p.Name}
}

type peeker struct{ Name string }

func (p peeker) Build(ctx BuildContext) Widget {
	builds[p.Name]++
	t, _ := Peek[theme](ctx)
	return Text{Text: p.Name + ":" + t.Accent}
}

func themed(accent string) Widget {
	return Provider[theme]{Value: theme{accent}, Child: Column{Children: []Widget{
		reader{Name: "r"}, plain{Name: "p"}, peeker{Name: "k"},
	}}}
}

func TestProviderRebuildsOnlyDependents(t *testing.T) {
	clear(builds)
	bo := NewBuildOwner()
	po := render.NewPipelineOwner()
	root := Mount(themed("red"), geom.White, bo, po)

	// Column holds a slice, so it's re-updated; its children are equal
	// comparable structs and are skipped unless they depend on the theme.
	root.SetApp(themed("blue"), geom.White)
	bo.FlushBuild()
	if builds["r"] != 2 || builds["p"] != 1 || builds["k"] != 1 {
		t.Fatalf("builds = %v, want r:2 p:1 k:1", builds)
	}
	if got := paragraphs(po); got[0] != "r:blue" || got[2] != "k:red" {
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
	_, _, po := setup(Provider[theme]{Value: theme{"outer"}, Child: Column{Children: []Widget{
		reader{Name: "a"},
		Provider[theme]{Value: theme{"inner"}, Child: reader{Name: "b"}},
		Provider[int]{Value: 7, Child: reader{Name: "c"}}, // different T doesn't shadow
	}}})
	got := paragraphs(po)
	if got[0] != "a:outer" || got[1] != "b:inner" || got[2] != "c:outer" {
		t.Fatalf("got %v", got)
	}

	_, _, po = setup(reader{Name: "none"})
	if got := paragraphs(po); got[0] != "none:" {
		t.Fatalf("missing provider should give zero value, got %v", got)
	}
}

// A hand-written InheritedWidget with a custom notify rule.
type locale struct {
	Lang  string
	Debug int // changes here don't matter to dependents
	Child Widget
}

func (l locale) ChildWidget() Widget                { return l.Child }
func (l locale) UpdateShouldNotify(old Widget) bool { return old.(locale).Lang != l.Lang }

type greeter struct{}

func (greeter) Build(ctx BuildContext) Widget {
	builds["g"]++
	l, _ := DependOn[locale](ctx)
	return Text{Text: map[string]string{"en": "hello", "uk": "привіт"}[l.Lang]}
}

func TestCustomInheritedWidget(t *testing.T) {
	clear(builds)
	root, bo, po := setup(locale{Lang: "en", Child: greeter{}})
	root.SetApp(locale{Lang: "en", Debug: 1, Child: greeter{}}, geom.White)
	bo.FlushBuild()
	if builds["g"] != 1 {
		t.Fatalf("UpdateShouldNotify=false still rebuilt: %d", builds["g"])
	}
	root.SetApp(locale{Lang: "uk", Child: greeter{}}, geom.White)
	bo.FlushBuild()
	if got := paragraphs(po); got[0] != "привіт" || builds["g"] != 2 {
		t.Fatalf("got %v builds %d", got, builds["g"])
	}
}
