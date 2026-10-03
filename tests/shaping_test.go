package tests

import (
	"os"
	"sort"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

func systemFont(t *testing.T, path string) *text.Font {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("font %s not installed", path)
	}
	f, err := text.ParseFont(path, data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const dejavu = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"

func TestLigatures(t *testing.T) {
	f := systemFont(t, dejavu)
	on := text.Layout("office fluffy", text.Style{Font: f, Size: 20}, text.Options{})
	off := text.Layout("office fluffy", text.Style{Font: f, Size: 20, NoLigatures: true}, text.Options{})
	if len(off.Glyphs) != 12 { // the space has no glyph
		t.Fatalf("without ligatures: %d glyphs", len(off.Glyphs))
	}
	if len(on.Glyphs) >= len(off.Glyphs) {
		t.Fatalf("ligatures not applied: %d glyphs vs %d", len(on.Glyphs), len(off.Glyphs))
	}
	// A ligature cluster covers several characters; the caret still has a
	// stop in front of each one.
	l := on.Lines[0]
	multi := false
	for _, c := range l.Clusters {
		if c.End-c.Start > 1 {
			multi = true
		}
	}
	if !multi || len(l.Offsets) != len("office fluffy")+1 {
		t.Fatalf("clusters %+v, %d offsets", l.Clusters, len(l.Offsets))
	}
	for i := 1; i < len(l.Stops); i++ {
		if l.Stops[i] < l.Stops[i-1] {
			t.Fatalf("left-to-right stops must ascend: %v", l.Stops)
		}
	}
	// Selecting half of "ffi" covers part of the ligature.
	if rs := on.SelectionRects(1, 2); len(rs) != 1 || rs[0].W <= 0 {
		t.Fatalf("partial ligature selection: %v", rs)
	}
}

func TestArabicAndHebrew(t *testing.T) {
	f := systemFont(t, dejavu)
	st := text.Style{Font: f, Size: 20}
	// Arabic letters join: the shaped glyphs aren't the isolated forms.
	word := "سلام"
	p := text.Layout(word, st, text.Options{})
	var nominal []text.GlyphID
	for _, r := range word {
		nominal = append(nominal, f.Glyph(r))
	}
	same := 0
	for i, g := range p.Glyphs {
		if i < len(nominal) && g.ID == nominal[i] {
			same++
		}
		if g.ID == 0 {
			t.Fatal(".notdef glyph")
		}
	}
	if same == len(nominal) {
		t.Fatal("no contextual forms")
	}
	if !p.Lines[0].RTL || !p.HasRTL() {
		t.Fatal("Arabic paragraph not right to left")
	}
	// Right to left: the first character is drawn rightmost.
	boxes := p.Lines[0].Clusters
	if boxes[0].X <= boxes[len(boxes)-1].X {
		t.Fatalf("not reversed: %+v", boxes)
	}
	// AlignStart puts an RTL paragraph on the right.
	p.SetAlign(text.AlignStart, 300)
	if p.Lines[0].X < 300-p.Lines[0].Width-0.5 {
		t.Fatalf("RTL start alignment: x=%v", p.Lines[0].X)
	}

	// Mixed: "abc שלום def" shows abc, then the Hebrew reversed, then def.
	mixed := "abc שלום def"
	m := text.Layout(mixed, st, text.Options{})
	l := m.Lines[0]
	if l.RTL {
		t.Fatal("paragraph starting with Latin should be left to right")
	}
	x := func(byteOff int) float32 {
		for _, c := range l.Clusters {
			if c.Start == byteOff {
				return c.X
			}
		}
		t.Fatalf("no cluster at %d", byteOff)
		return 0
	}
	shin, mem := 4, 4+3*2 // ש and ם (2 bytes each)
	if !(x(0) < x(mem) && x(mem) < x(shin) && x(shin) < x(len("abc שלום "))) {
		t.Fatalf("visual order: a=%v ם=%v ש=%v d=%v", x(0), x(mem), x(shin), x(len("abc שלום ")))
	}
	// Selecting "c ש" spans the direction change: two boxes.
	if rs := m.SelectionRects(2, 6); len(rs) != 2 {
		t.Fatalf("mixed selection: %v", rs)
	}
	// Every caret position maps back to its offset.
	for _, off := range l.Offsets[:len(l.Offsets)-1] {
		li, cx := m.CaretAt(off)
		if got := m.OffsetAt(geom.Offset{X: cx, Y: m.Lines[li].Top + 2}); got != off {
			// At a direction change two offsets share a position (the
			// edge between the runs); that's the only allowed mismatch.
			if _, gx := m.CaretAt(got); gx-cx > 0.01 || cx-gx > 0.01 {
				t.Errorf("offset %d → x %v → %d (at %v)", off, cx, got, gx)
			}
		}
	}
}

func TestIndicShaping(t *testing.T) {
	f := systemFont(t, "/usr/share/fonts/truetype/freefont/FreeSans.ttf")
	if !f.HasGlyph('क') {
		t.Skip("no Devanagari in this font")
	}
	// क्षि: ka + virama + ssa + vowel sign i. The vowel sign is drawn
	// before the conjunct although it comes last.
	s := "क्षि"
	p := text.Layout(s, text.Style{Font: f, Size: 24}, text.Options{})
	if len(p.Glyphs) == 0 || len(p.Glyphs) == 4 && p.Glyphs[3].ID == f.Glyph('ि') && p.Glyphs[3].X > p.Glyphs[0].X {
		t.Fatalf("not reordered: %+v", p.Glyphs)
	}
	xs := make([]float32, len(p.Glyphs))
	for i, g := range p.Glyphs {
		xs[i] = g.X
	}
	if !sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] }) {
		t.Logf("glyph x positions %v (marks may overlap)", xs)
	}
	if c := p.Lines[0].Clusters; len(c) != 1 || c[0].End != len(s) {
		t.Fatalf("a conjunct is one cluster: %+v", c)
	}
}

func TestPlainTextKeepsFastPath(t *testing.T) {
	// The Go fonts have no ligatures: Latin text is laid out exactly as
	// before (one glyph per character, kerning from the kern table).
	p := text.Layout("AVATAR office", text.Style{Size: 20}, text.Options{})
	if len(p.Glyphs) != 12 {
		t.Fatalf("%d glyphs", len(p.Glyphs))
	}
	for _, c := range p.Lines[0].Clusters {
		if c.End-c.Start != 1 || c.RTL {
			t.Fatalf("cluster %+v", c)
		}
	}
}
