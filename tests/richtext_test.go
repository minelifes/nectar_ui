package tests

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// A single span lays out exactly like plain text.
func TestSpansMatchPlainLayout(t *testing.T) {
	st := text.Style{Size: 15, LineHeight: 1.4, LetterSpacing: 0.3}
	for _, opt := range []text.Options{{}, {MaxWidth: 90}, {MaxWidth: 90, MaxLines: 2, Ellipsis: "…"}} {
		s := "Hello brave new world, wrapped\nand a second line"
		a := text.Layout(s, st, opt)
		b := text.LayoutSpans([]text.Span{{Text: "Hello brave "}, {Text: "new world, wrapped\nand"}, {Text: " a second line"}}, st, opt)
		if !reflect.DeepEqual(a.Glyphs, b.Glyphs) || !reflect.DeepEqual(a.Lines, b.Lines) || a.Width != b.Width || a.Height != b.Height || b.Rich {
			t.Fatalf("opt %+v: spans with the base style differ from plain text", opt)
		}
	}
}

func TestSpanStyles(t *testing.T) {
	red := geom.Hex(0xff0000)
	base := text.Style{Size: 14}
	p := text.LayoutSpans([]text.Span{
		{Text: "plain "},
		{Text: "red", Style: text.Style{Color: red}},
		{Text: " big", Style: text.Style{Size: 28}},
	}, base, text.Options{})
	if !p.Rich {
		t.Fatal("a colored span makes the paragraph rich")
	}
	plain := text.Layout("plain red big", base, text.Options{})
	if p.Height <= plain.Height {
		t.Fatalf("a bigger span should grow the line: %v vs %v", p.Height, plain.Height)
	}
	for _, g := range p.Glyphs {
		switch {
		case g.Cluster >= 6 && g.Cluster < 9:
			if g.Color != red || g.Size != 14 {
				t.Fatalf("red span glyph: %+v", g)
			}
		case g.Cluster >= 10:
			if g.Size != 28 || g.Color != geom.Black {
				t.Fatalf("big span glyph: %+v", g)
			}
		default:
			if g.Color != geom.Black {
				t.Fatalf("plain glyph: %+v", g)
			}
		}
	}
	// All glyphs share one baseline.
	for _, g := range p.Glyphs {
		if g.Y != p.Glyphs[0].Y {
			t.Fatal("glyphs of one line must share the baseline")
		}
	}
}

func TestDecorations(t *testing.T) {
	hl := geom.Hex(0xffff00)
	p := text.LayoutSpans([]text.Span{
		{Text: "a "},
		{Text: "marked", Style: text.Style{Background: hl, Decoration: text.Underline | text.Strikethrough}},
	}, text.Style{}, text.Options{})
	var bg, lines int
	for _, d := range p.Decorations {
		if d.Behind {
			bg++
			if d.Color != hl || d.Rect.H != p.Lines[0].Height {
				t.Fatalf("background: %+v", d)
			}
		} else {
			lines++
			if d.Color != geom.Black { // the text color
				t.Fatalf("decoration color: %+v", d)
			}
		}
	}
	if bg != 1 || lines != 2 {
		t.Fatalf("got %d backgrounds and %d lines: %+v", bg, lines, p.Decorations)
	}
	// The canvas draws the background before the text and the lines after.
	var c render.Canvas
	c.Reset(geom.Rect{W: 400, H: 100})
	c.DrawParagraph(p, geom.Offset{X: 10, Y: 5})
	var kinds []render.CommandKind
	for _, cmd := range c.Commands {
		kinds = append(kinds, cmd.Kind)
	}
	want := []render.CommandKind{render.CmdRect, render.CmdText, render.CmdRect, render.CmdRect}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("commands %v, want %v", kinds, want)
	}
	if c.Commands[0].Rect.X <= 10 {
		t.Fatalf("background should start after %q: %+v", "a ", c.Commands[0].Rect)
	}
}

func TestTabStops(t *testing.T) {
	st := text.Style{TabSize: 4}
	p := text.Layout("a\tb\n\tc", st, text.Options{})
	space := text.Layout(" ", text.Style{}, text.Options{}).Lines[0].Stops[1]
	b := p.Glyphs[1]
	if d := b.X - 4*space; d > 0.01 || d < -0.01 {
		t.Fatalf("b after a tab at %v, want the stop at %v", b.X, 4*space)
	}
	c := p.Glyphs[2]
	if d := c.X - 4*space; d > 0.01 || d < -0.01 {
		t.Fatalf("tab stops restart on each line: c at %v", c.X)
	}
	// Without TabSize a tab is a space, as before.
	q := text.Layout("a\tb", text.Style{}, text.Options{})
	if r := text.Layout("a b", text.Style{}, text.Options{}); q.Glyphs[1].X != r.Glyphs[1].X {
		t.Fatal("TabSize 0 should keep tabs one space wide")
	}
}

func TestFontFallback(t *testing.T) {
	data, err := os.ReadFile("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	if err != nil {
		t.Skip("no DejaVu Sans to fall back to")
	}
	dejavu, err := text.ParseFont("DejaVu Sans", data)
	if err != nil {
		t.Fatal(err)
	}
	const star = '★'
	if text.DefaultFont().HasGlyph(star) || !dejavu.HasGlyph(star) {
		t.Skip("fonts don't differ in the test rune")
	}
	f := text.DefaultFont().WithFallback(dejavu)
	if f.Face() != text.DefaultFont() || f.ID() != text.DefaultFont().ID() {
		t.Fatal("a fallback font shares its face")
	}
	p := text.Layout("a★b", text.Style{Font: f}, text.Options{})
	if len(p.Glyphs) != 3 || p.Glyphs[1].Font != dejavu || p.Glyphs[0].Font != text.DefaultFont() || p.Glyphs[1].ID == 0 {
		t.Fatalf("fallback glyphs: %+v", p.Glyphs)
	}
	// Without the chain the rune is .notdef in the primary font...
	if q := text.Layout("★", text.Style{}, text.Options{}); q.Glyphs[0].ID != 0 {
		t.Fatal("expected .notdef without fallback")
	}
	// ...unless the global fallback has it.
	text.SetGlobalFallback(dejavu)
	defer text.SetGlobalFallback()
	if q := text.Layout("★", text.Style{}, text.Options{}); q.Glyphs[0].Font != dejavu {
		t.Fatal("global fallback not used")
	}
}

func TestParagraphGeometry(t *testing.T) {
	s := "hello world again"
	p := text.Layout(s, text.Style{Size: 16}, text.Options{MaxWidth: 60})
	if len(p.Lines) < 2 {
		t.Fatalf("expected wrapping, got %d lines", len(p.Lines))
	}
	for i := 0; i <= len(s); i++ {
		li, x := p.CaretAt(i)
		l := p.Lines[li]
		if got := p.OffsetAt(geom.Offset{X: x, Y: l.Top + 1}); got != i {
			// The end of a soft-wrapped line is the next line's start.
			if !(i == l.End && got == l.End) {
				t.Errorf("offset %d → caret (%d, %v) → offset %d", i, li, x, got)
			}
		}
	}
	rs := p.SelectionRects(2, 14)
	if len(rs) != len(p.Lines) && len(rs) < 2 {
		t.Fatalf("selection across lines: %v", rs)
	}
	if a, b := text.WordAt(s, 7); s[a:b] != "world" {
		t.Fatalf("WordAt: %q", s[a:b])
	}
	if a, b := text.WordAt(s, 5); s[a:b] != "hello" {
		t.Fatalf("WordAt at a word's end: %q", s[a:b])
	}
}

func TestRichTextWidget(t *testing.T) {
	spans := []text.Span{{Text: "Hello "}, {Text: "bold", Style: text.Style{Font: text.DefaultBoldFont()}}}
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.RichText{Spans: spans, Style: text.Style{Size: 18}}}, 300, 100)
	cmds := tt.Pump().Commands
	if len(cmds) != 1 || cmds[0].Text == nil || cmds[0].Text.Text != "Hello bold" {
		t.Fatalf("commands: %+v", cmds)
	}
	var bold int
	for _, g := range cmds[0].Text.Glyphs {
		if g.Font == text.DefaultBoldFont() {
			bold++
		}
	}
	if bold != 4 {
		t.Fatalf("%d bold glyphs", bold)
	}
}

func TestSelectableText(t *testing.T) {
	var got string
	s := "copy these words"
	tt := tester.New(w.Align{Alignment: geom.TopLeft, Child: w.SelectableText{Text: s, Style: text.Style{Size: 16},
		OnSelectionChanged: func(sel string) { got = sel }}}, 400, 100)
	r, ok := tt.Find(s)
	if !ok {
		t.Fatal("text not drawn")
	}
	// Double-click selects a word.
	p := text.Layout(s, text.Style{Size: 16}, text.Options{})
	_, x := p.CaretAt(7)
	tt.Tap(r.X+x+3, r.Y+5)
	tt.Tap(r.X+x+3, r.Y+5)
	if got != "these" {
		t.Fatalf("double-click selected %q", got)
	}
	if !hasSelectionRect(tt.Pump()) {
		t.Fatal("selection not painted")
	}
	tt.Key(w.KeyC, w.ModControl)
	if tt.Clipboard() != "these" {
		t.Fatalf("copied %q", tt.Clipboard())
	}
	// Drag selects a range; Ctrl+A everything.
	tt.Advance(time.Second)
	tt.Drag(geom.Offset{X: r.X + 1, Y: r.Y + 5}, geom.Offset{X: r.X + x, Y: r.Y + 5})
	if got != "copy th" {
		t.Fatalf("drag selected %q", got)
	}
	tt.Key(w.KeyA, w.ModControl)
	if got != s {
		t.Fatalf("select all: %q", got)
	}
	// Clicking elsewhere clears it.
	tt.Tap(390, 90)
	if got != "" {
		t.Fatalf("still selected after clicking away: %q", got)
	}
}

func hasSelectionRect(c *render.Canvas) bool {
	for _, cmd := range c.Commands {
		if cmd.Kind == render.CmdRect && cmd.Color.A > 0.2 && cmd.Color.A < 0.4 {
			return true
		}
	}
	return false
}
