package tests

import (
	"math"
	"testing"

	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// An outlined field with a leading icon floats its label to the border
// start (into the notch), not above the text after the icon.
func TestOutlinedLabelWithPrefix(t *testing.T) {
	tt := run(func(w.BuildContext, func(func())) w.Widget {
		return w.SizedBox{Width: 400, Child: m.TextField{Outlined: true, Label: "Search", Prefix: icons.Search, Hint: "Type to search"}}
	})
	// Resting: the label sits where the text starts (20 host padding + 12 +
	// 24 icon + 16).
	r, ok := tt.Find("Search")
	if !ok || math.Abs(float64(r.X-72)) > 1 {
		t.Fatalf("resting label at %v (ok=%v), want x 72", r, ok)
	}
	tt.Tap(250, 48)
	tt.Settle()
	// Floated: 16px from the field edge, inside the notch that starts at 12.
	r, ok = tt.Find("Search")
	if !ok || math.Abs(float64(r.X-36)) > 1 || r.Y > 20+2 {
		t.Fatalf("floated label at %v (ok=%v), want x 36 on the border", r, ok)
	}
	if _, ok := tt.Find("Type to search"); !ok {
		t.Fatal("hint not shown when focused")
	}
}
