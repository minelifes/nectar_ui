package tests

import (
	"testing"

	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

func width(s string, st text.Style) float32 {
	return text.Layout(s, st, text.Options{MaxWidth: 10000}).Size().W
}

func TestDefaultFamilyWeights(t *testing.T) {
	reg := text.DefaultFont()
	cases := map[text.FontWeight]*text.Font{
		0:             reg,
		text.Thin:     reg, // nothing lighter: the closest
		text.Light:    reg,
		text.Normal:   reg,
		text.Medium:   text.DefaultMediumFont(),
		text.SemiBold: text.DefaultBoldFont(), // bolder first
		text.Bold:     text.DefaultBoldFont(),
	}
	for wt, want := range cases {
		if got := reg.WithWeight(wt); got != want {
			t.Errorf("WithWeight(%v) = %s, want %s", wt, got.Name(), want.Name())
		}
	}
	// 900 is beyond Go Bold: a synthetic bold of it.
	black := reg.WithWeight(text.Black)
	if black == text.DefaultBoldFont() || black.Weight() != text.Black || black.Family() != text.DefaultFamily() {
		t.Errorf("Black: %s weight %v", black.Name(), black.Weight())
	}
	// Stable pointers (styles compare fonts by pointer) and back again.
	if reg.WithWeight(text.Black) != black || black.WithWeight(text.Normal) != reg ||
		text.DefaultBoldFont().WithWeight(text.Normal) != reg {
		t.Error("WithWeight not stable / reversible")
	}
	// Bolder is wider.
	s := "The quick brown fox"
	wr := width(s, text.Style{Size: 20})
	wm := width(s, text.Style{Size: 20, Weight: text.Medium})
	wb := width(s, text.Style{Size: 20, Weight: text.Bold})
	wk := width(s, text.Style{Size: 20, Weight: text.Black})
	if !(wr < wm && wm < wb && wb < wk) {
		t.Errorf("widths regular %v medium %v bold %v black %v", wr, wm, wb, wk)
	}
}

func TestSingleFontGetsSyntheticBold(t *testing.T) {
	f, err := text.ParseFont("Custom", goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	b := f.WithWeight(text.Bold)
	if b == f || b.Weight() != text.Bold {
		t.Fatalf("no synthetic bold: %v", b.Weight())
	}
	if f.WithWeight(text.Medium) != f || f.WithWeight(text.Light) != f {
		t.Error("weights a synthetic bold can't help should keep the font")
	}
	s := "Hello, weights"
	if wr, wb := width(s, text.Style{Font: f, Size: 20}), width(s, text.Style{Font: f, Size: 20, Weight: text.Bold}); wb <= wr {
		t.Errorf("synthetic bold not wider: %v vs %v", wb, wr)
	}
	// Complex-shaped text (combining marks go through HarfBuzz) widens too.
	cs := "ééééé"
	if wr, wb := width(cs, text.Style{Font: f, Size: 20}), width(cs, text.Style{Font: f, Size: 20, Weight: text.Bold}); wb <= wr {
		t.Errorf("shaped synthetic bold not wider: %v vs %v", wb, wr)
	}
}

func TestCustomFamily(t *testing.T) {
	reg, _ := text.ParseFont("Fam Regular", goregular.TTF)
	other, _ := text.ParseFont("Fam Light (italic data)", goitalic.TTF)
	fam := text.NewFamily("Fam").Add(text.Normal, reg).Add(text.Light, other)
	if fam.Font() != reg || reg.Family() != fam || other.Weight() != text.Light {
		t.Fatal("family setup")
	}
	if reg.WithWeight(text.Thin) != other || reg.WithWeight(text.ExtraLight) != other {
		t.Error("lighter weights should pick the light face")
	}
	if reg.WithWeight(text.Medium) != reg {
		t.Error("500 should fall back to 400")
	}
	if b := reg.WithWeight(text.Bold); b == reg || b.Weight() != text.Bold {
		t.Error("bold should be synthetic")
	}
	// Fallback chains survive a weight change.
	fb := reg.WithFallback(text.DefaultFont())
	if got := fb.WithWeight(text.Light); len(got.Fallback()) != 1 || got.Face() != other {
		t.Errorf("fallback chain lost: %d %s", len(got.Fallback()), got.Face().Name())
	}
}

func TestWeightInheritsAndCopyWith(t *testing.T) {
	base := text.Style{Size: 14, Weight: text.Bold}
	if (text.Style{Color: geom.Hex(0xFF0000)}).Inherit(base).Weight != text.Bold {
		t.Error("Weight not inherited")
	}
	th := m.NewTheme(geom.Hex(0x6750A4), false)
	if got := th.Text.LabelLarge.CopyWith(text.Style{Weight: text.Bold}).Resolved().Font; got != text.DefaultBoldFont() {
		t.Errorf("label large bold = %s", got.Name())
	}
	if got := th.Text.LabelLarge.CopyWith(text.Style{Weight: text.Normal}).Resolved().Font; got != text.DefaultFont() {
		t.Errorf("label large normal = %s", got.Name())
	}
}

func TestWeightInWidgets(t *testing.T) {
	tt := tester.New(w.Column{Children: []w.Widget{
		w.Text{Text: "Same words here", Style: text.Style{Size: 20}},
		w.Text{Text: "Same words here!", Style: text.Style{Size: 20, Weight: text.Bold}},
		w.RichText{Style: text.Style{Size: 20}, Spans: []text.Span{
			{Text: "plain "}, {Text: "bold", Style: text.Style{Weight: text.Bold}}}},
	}}, 600, 300)
	tt.Settle()
	r1, _ := tt.Find("Same words here")
	r2, _ := tt.Find("Same words here!")
	if r2.W <= r1.W {
		t.Errorf("bold text not wider: %v vs %v", r2.W, r1.W)
	}
	rich := text.LayoutSpans([]text.Span{{Text: "plain "}, {Text: "bold", Style: text.Style{Weight: text.Bold}}}, text.Style{Size: 20}, text.Options{MaxWidth: 1000})
	bold := false
	for _, g := range rich.Glyphs {
		if g.Font == text.DefaultBoldFont() {
			bold = true
		}
	}
	if !bold {
		t.Error("bold span not drawn with Go Bold")
	}
}

// Synthetic bold glyphs cover more pixels than the regular ones.
func TestSyntheticBoldIsThicker(t *testing.T) {
	f, _ := text.ParseFont("Thick", goregular.TTF)
	ink := func(st text.Style) int {
		tt := tester.New(w.DecoratedBox{Color: geom.Hex(0xFFFFFF), Child: w.Align{Alignment: geom.TopLeft,
			Child: w.Text{Text: "iiiill", Style: st}}}, 200, 60)
		img, err := tt.Snapshot()
		if err != nil {
			t.Skip("no GPU adapter:", err)
		}
		n := 0
		for i := 0; i < len(img.Pix); i += 4 {
			n += 255 - int(img.Pix[i])
		}
		return n
	}
	reg := ink(text.Style{Font: f, Size: 24})
	bold := ink(text.Style{Font: f, Size: 24, Weight: text.Bold})
	if float32(bold) < float32(reg)*1.25 {
		t.Errorf("synthetic bold ink %d vs regular %d", bold, reg)
	}
}
