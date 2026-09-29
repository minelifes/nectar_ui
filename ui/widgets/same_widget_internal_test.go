package widgets

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
)

func TestSameWidget(t *testing.T) {
	kids := []Widget{Text{Text: "a"}, Padding{Padding: geom.Insets(4), Child: Text{Text: "b"}}}
	clone := []Widget{Text{Text: "a"}, Padding{Padding: geom.Insets(4), Child: Text{Text: "b"}}}
	border := &geom.Border{Radius: 4}
	fn := func() {}
	cases := []struct {
		name     string
		old, new Widget
		want     bool
	}{
		{"equal structs", Text{Text: "a"}, Text{Text: "a"}, true},
		{"different field", Text{Text: "a"}, Text{Text: "b"}, false},
		{"different type", Text{Text: "a"}, Padding{}, false},
		{"equal children in new slices", Column{Children: kids}, Column{Children: clone}, true},
		{"changed nested child", Column{Children: kids}, Column{Children: []Widget{Text{Text: "a"}, Text{Text: "b"}}}, false},
		{"shared backing array", Column{Children: kids}, Column{Children: kids}, false},
		{"nil and empty slice", Column{}, Column{Children: []Widget{}}, true},
		{"same pointer", DecoratedBox{Border: border}, DecoratedBox{Border: border}, true},
		{"equal pointee, other pointer", DecoratedBox{Border: border}, DecoratedBox{Border: &geom.Border{Radius: 4}}, false},
		{"funcs never equal", GestureDetector{OnTap: fn}, GestureDetector{OnTap: fn}, false},
		{"nil funcs equal", GestureDetector{}, GestureDetector{}, true},
		{"maps never equal", mapWidget{m: map[string]int{}}, mapWidget{m: map[string]int{}}, false},
	}
	for _, c := range cases {
		if got := sameWidget(c.old, c.new); got != c.want {
			t.Errorf("%s: sameWidget = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSameWidgetGivesUpOnHugeTrees(t *testing.T) {
	mk := func() Widget {
		var kids []Widget
		for range sameWidgetBudget {
			kids = append(kids, Text{Text: "x"})
		}
		return Column{Children: kids}
	}
	if sameWidget(mk(), mk()) {
		t.Fatal("comparison should stop at the budget and report a change")
	}
}

type mapWidget struct{ m map[string]int }

func (mapWidget) Build(BuildContext) Widget { return nil }
