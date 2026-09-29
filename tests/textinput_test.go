package tests

import (
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// field puts an EditableText of the given width at the top-left.
func editField(ctrl *w.TextEditingController, width float32, mod func(*w.EditableText)) w.Widget {
	e := w.EditableText{Controller: ctrl, Style: text.Style{Size: 16}}
	if mod != nil {
		mod(&e)
	}
	return w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Width: width, Child: e}}
}

func editableRO(tt *tester.Tester) *render.RenderEditable {
	var found *render.RenderEditable
	var walk func(ro render.RenderObject)
	walk = func(ro render.RenderObject) {
		if r, ok := ro.(*render.RenderEditable); ok && found == nil {
			found = r
		}
		ro.VisitChildren(walk)
	}
	walk(tt.Pipeline.Root())
	return found
}

const long = "the quick brown fox jumps over the lazy dog and keeps running far away"

func TestMultilineSoftWrap(t *testing.T) {
	ctrl := w.NewTextController(long)
	tt := tester.New(editField(ctrl, 160, func(e *w.EditableText) { e.Multiline = true }), 400, 400)
	r := editableRO(tt)
	lineH, _ := text.LineMetrics(text.Style{Size: 16})
	n := r.Lines()
	if n < 3 {
		t.Fatalf("expected the long line to wrap, got %d line(s)", n)
	}
	if h := r.Size().H; h < float32(n)*lineH-0.5 {
		t.Fatalf("field height %v for %d lines of %v", h, n, lineH)
	}
	if r.Size().W > 160 {
		t.Fatalf("wrapped field wider than its box: %v", r.Size())
	}

	// Home/End work per visual line; ↓ moves to the next visual line.
	tt.Tap(5, 5) // caret at the start
	tt.Key(w.KeyHome)
	tt.Key(w.KeyEnd)
	_, end1 := ctrl.Selection()
	if end1 == 0 || end1 >= len(long) || long[end1] != ' ' {
		t.Fatalf("End on a wrapped line: offset %d (%q)", end1, long[:end1])
	}
	tt.Key(w.KeyHome)
	tt.Key(w.KeyDown)
	_, down := ctrl.Selection()
	if down <= end1 {
		t.Fatalf("↓ stayed on the first line: %d", down)
	}
	// Clicking on the second visual line lands there.
	tt.Tap(5, lineH*1.5)
	_, clicked := ctrl.Selection()
	if clicked <= end1 || clicked > end1+2 {
		t.Fatalf("click on line 2 → offset %d, line 1 ends at %d", clicked, end1)
	}
	// Typing keeps wrapping.
	tt.Key(w.KeyEnd)
	tt.Type(" " + strings.Repeat("x", 5))
	if editableRO(tt).Lines() < n {
		t.Fatal("lines lost after typing")
	}
}

func TestNoWrapAndSingleLine(t *testing.T) {
	tt := tester.New(editField(w.NewTextController(long), 160, func(e *w.EditableText) { e.Multiline, e.NoWrap = true, true }), 400, 200)
	if n := editableRO(tt).Lines(); n != 1 {
		t.Fatalf("NoWrap: %d lines", n)
	}
	tt = tester.New(editField(w.NewTextController(long), 160, nil), 400, 200)
	if n := editableRO(tt).Lines(); n != 1 {
		t.Fatalf("single-line field wrapped: %d lines", n)
	}
	// Hard newlines still split lines.
	tt = tester.New(editField(w.NewTextController("a\nb\nc"), 160, func(e *w.EditableText) { e.Multiline = true }), 400, 200)
	if n := editableRO(tt).Lines(); n != 3 {
		t.Fatalf("hard lines: %d", n)
	}
}

func TestIMEComposition(t *testing.T) {
	ctrl := w.NewTextController("ab")
	changed := 0
	tt := tester.New(editField(ctrl, 300, func(e *w.EditableText) { e.OnChanged = func(string) { changed++ } }), 400, 200)
	tt.Tap(280, 8) // focus, caret at the end

	tt.Compose("ni", -1)
	if got := editableRO(tt).Text; got != "abni" {
		t.Fatalf("preedit not shown inline: %q", got)
	}
	if ctrl.Text() != "ab" || changed != 0 {
		t.Fatalf("preedit leaked into the value: %q (%d changes)", ctrl.Text(), changed)
	}
	// The preedit is underlined.
	r := editableRO(tt)
	if r.ComposeStart != 2 || r.ComposeEnd != 4 {
		t.Fatalf("compose range %d..%d", r.ComposeStart, r.ComposeEnd)
	}
	canvas := tt.Pump()
	underlined := false
	for _, c := range canvas.Commands {
		if c.Kind == render.CmdRect && c.Rect.H <= 2 && c.Rect.W > 4 {
			underlined = true
		}
	}
	if !underlined {
		t.Fatal("no underline under the preedit")
	}
	// Editing keys belong to the IME while composing.
	tt.Key(w.KeyBackspace)
	if ctrl.Text() != "ab" {
		t.Fatalf("backspace edited the field during composition: %q", ctrl.Text())
	}
	// Candidate conversion, then commit.
	tt.Compose("你", -1)
	rectBefore, ok := tt.IMERect()
	if !ok {
		t.Fatal("no caret rect for the candidate window")
	}
	tt.Commit("你好")
	if ctrl.Text() != "ab你好" || changed != 1 {
		t.Fatalf("commit: %q (%d changes)", ctrl.Text(), changed)
	}
	if b, e := ctrl.Selection(); b != len("ab你好") || e != b {
		t.Fatalf("caret after commit %d..%d", b, e)
	}
	rectAfter, _ := tt.IMERect()
	if rectAfter.X <= rectBefore.X || rectAfter.Y < 0 || rectAfter.X > 300 {
		t.Fatalf("caret rect didn't follow the text: %v → %v", rectBefore, rectAfter)
	}

	// Cancel leaves the value alone.
	tt.Compose("xyz", 1)
	tt.CancelIME()
	if ctrl.Text() != "ab你好" || editableRO(tt).Text != "ab你好" {
		t.Fatalf("cancel: %q / %q", ctrl.Text(), editableRO(tt).Text)
	}

	// The preedit replaces the selection, like typing does.
	tt.Key(w.KeyA, w.ModControl|w.ModSuper)
	tt.Compose("k", -1)
	if got := editableRO(tt).Text; got != "k" {
		t.Fatalf("preedit over selection: %q", got)
	}
	tt.Commit("か")
	if ctrl.Text() != "か" {
		t.Fatalf("commit over selection: %q", ctrl.Text())
	}
}

func TestIMEOffForPasswords(t *testing.T) {
	ctrl := w.NewTextController("")
	tt := tester.New(editField(ctrl, 300, func(e *w.EditableText) { e.Obscure = true }), 400, 200)
	tt.Tap(10, 8)
	if tt.Build.Focus().WantsIME() {
		t.Fatal("password field asks for IME input")
	}
	tt.Compose("abc", -1)
	tt.Commit("abc")
	if ctrl.Text() != "" {
		t.Fatalf("IME text reached a password field: %q", ctrl.Text())
	}
	tt.Type("pw") // normal typing still works
	if ctrl.Text() != "pw" {
		t.Fatalf("typing: %q", ctrl.Text())
	}
}
