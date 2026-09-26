package text

import (
	"strings"
	"testing"
)

func TestLayoutSingleLine(t *testing.T) {
	p := Layout("Hello", Style{Size: 16}, Options{})
	if len(p.Lines) != 1 || len(p.Glyphs) != 5 {
		t.Fatalf("lines=%d glyphs=%d", len(p.Lines), len(p.Glyphs))
	}
	if p.Width <= 0 || p.Height <= 0 {
		t.Fatalf("bad size %vx%v", p.Width, p.Height)
	}
	for i := 1; i < len(p.Glyphs); i++ {
		if p.Glyphs[i].X <= p.Glyphs[i-1].X {
			t.Fatalf("glyph %d not advancing", i)
		}
	}
}

func TestLayoutWrapsAtSpaces(t *testing.T) {
	style := Style{Size: 16}
	one := Layout("word", style, Options{}).Width
	p := Layout("word word word", style, Options{MaxWidth: one * 1.5})
	if len(p.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(p.Lines))
	}
	for _, l := range p.Lines {
		if l.Width > one*1.5 {
			t.Fatalf("line too wide: %v", l.Width)
		}
		if l.Last-l.First != 4 {
			t.Fatalf("each line should hold one word, got %d glyphs", l.Last-l.First)
		}
	}
}

func TestLayoutHardBreaksAndEmptyLines(t *testing.T) {
	p := Layout("a\n\nb", Style{}, Options{})
	if len(p.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(p.Lines))
	}
	if p.Lines[1].Last != p.Lines[1].First {
		t.Fatal("middle line should be empty")
	}
}

func TestLayoutLongWordBreaksMidWord(t *testing.T) {
	style := Style{Size: 16}
	w := Layout("abc", style, Options{}).Width
	p := Layout(strings.Repeat("a", 30), style, Options{MaxWidth: w})
	if len(p.Lines) < 5 {
		t.Fatalf("expected char wrapping, got %d lines", len(p.Lines))
	}
}

func TestLayoutEllipsis(t *testing.T) {
	style := Style{Size: 16}
	w := Layout("hello world", style, Options{}).Width
	p := Layout("hello world and more text here", style, Options{MaxWidth: w, MaxLines: 1, Ellipsis: "…"})
	if !p.Truncated || len(p.Lines) != 1 {
		t.Fatalf("truncated=%v lines=%d", p.Truncated, len(p.Lines))
	}
	if p.Lines[0].Width > w+0.01 {
		t.Fatalf("ellipsized line overflows: %v > %v", p.Lines[0].Width, w)
	}
	last := p.Glyphs[len(p.Glyphs)-1]
	if last.ID != style.Resolved().Font.Glyph('…') {
		t.Fatal("last glyph should be the ellipsis")
	}
}

func TestCyrillic(t *testing.T) {
	f := DefaultFont()
	for _, r := range "Привіт, світе! Їжак ґанок" {
		if r != ' ' && r != ',' && r != '!' && !f.HasGlyph(r) {
			t.Fatalf("default font missing %q", r)
		}
	}
}

func TestAtlasRasterizes(t *testing.T) {
	a := NewAtlas(256)
	f := DefaultFont()
	a.TakeDirty()
	e, ok := a.Glyph(f, f.Glyph('A'), 32, 0)
	if !ok || e.Empty || e.W == 0 || e.H == 0 {
		t.Fatalf("bad entry %+v ok=%v", e, ok)
	}
	// Top of 'A' is above the baseline.
	if e.OffY >= 0 {
		t.Fatalf("expected negative OffY, got %v", e.OffY)
	}
	var cov int
	for y := e.Y; y < e.Y+e.H; y++ {
		for x := e.X; x < e.X+e.W; x++ {
			cov += int(a.Pixels()[(y*a.Size()+x)*4+3])
		}
	}
	if cov == 0 {
		t.Fatal("glyph bitmap is empty")
	}
	if r, ok := a.TakeDirty(); !ok || r.Dx() != e.W {
		t.Fatalf("dirty rect %v", r)
	}
	// Cached.
	e2, _ := a.Glyph(f, f.Glyph('A'), 32, 0)
	if e2 != e {
		t.Fatal("expected cache hit")
	}
	if sp, _ := a.Glyph(f, f.Glyph(' '), 32, 0); !sp.Empty {
		t.Fatal("space should be empty")
	}
}

func TestAtlasFull(t *testing.T) {
	a := NewAtlas(64)
	f := DefaultFont()
	full := false
	for r := 'A'; r <= 'z'; r++ {
		if _, ok := a.Glyph(f, f.Glyph(r), 40, 0); !ok {
			full = true
			break
		}
	}
	if !full {
		t.Fatal("expected tiny atlas to fill up")
	}
	gen := a.Generation()
	a.Reset()
	if a.Generation() == gen {
		t.Fatal("generation should change on reset")
	}
}
