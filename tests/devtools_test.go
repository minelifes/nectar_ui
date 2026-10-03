package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/devtools"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func TestSemanticsTree(t *testing.T) {
	saved, toggled := 0, false
	page := m.App{Home: m.Scaffold{Body: w.Column{Cross: w.CrossStart, Children: []w.Widget{
		m.FilledButton{Label: "Save", OnPressed: func() { saved++ }},
		m.TextButton{Label: "Disabled"},
		m.IconButton{Icon: icons.Search, SemanticLabel: "Search", OnPressed: func() {}},
		m.Checkbox{Value: true, SemanticLabel: "Wrap lines", OnChanged: func(bool) {}},
		m.Switch{Value: false, SemanticLabel: "Autosave", OnChanged: func(v bool) { toggled = v }},
		w.SizedBox{Width: 200, Child: m.Slider{Value: 3, Max: 10, SemanticLabel: "Size", OnChanged: func(float32) {}}},
		w.SizedBox{Width: 300, Child: m.TextField{Label: "Name", Controller: w.NewTextController("Ada")}},
		w.SizedBox{Width: 300, Child: m.TabBar{Tabs: []m.Tab{{Text: "One"}, {Text: "Two"}}, Selected: 1, OnChanged: func(int) {}}},
		w.Semantics{SemanticsData: w.SemanticsData{Role: "header", Label: "Custom"}, Child: w.Text{Text: "Custom"}},
	}}}}
	tt := tester.New(page, 800, 900)
	var lines []string
	for _, n := range tt.Semantics() {
		lines = append(lines, n.String())
	}
	got := strings.Join(lines, "\n")
	for _, want := range []string{
		`button "Save"`, `button "Disabled" disabled`, `button "Search"`,
		`checkbox "Wrap lines" checked`, `switch "Autosave" unchecked`, `slider "Size" = 3`,
		`textfield "Name" = Ada`, `tab "Two" selected`, `tab "One"`, `header "Custom"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Actions work through the tree, and rects match what's drawn.
	n, ok := tt.FindSemantics("button", "Save")
	if !ok || n.Rect.W < 40 {
		t.Fatalf("Save node: %+v", n)
	}
	label, _ := tt.Find("Save")
	if !n.Rect.Contains(geom.Pt(label.X+1, label.Y+1)) {
		t.Fatalf("rect %v doesn't hold the label %v", n.Rect, label)
	}
	n.OnTap()
	sw, _ := tt.FindSemantics("switch", "Autosave")
	sw.OnTap()
	if saved != 1 || !toggled {
		t.Fatalf("actions: saved=%d toggled=%v", saved, toggled)
	}
}

func TestInspector(t *testing.T) {
	enabled := true
	exited := false
	pressed := 0
	var app w.Widget
	build := func() w.Widget {
		return devtools.Inspector{Enabled: enabled, OnExit: func() { exited = true }, Child: w.Align{Alignment: geom.TopLeft,
			Child: w.Padding{Padding: geom.Insets(20), Child: w.GestureDetector{OnTap: func() { pressed++ },
				Child: w.SizedBox{Width: 120, Height: 40, Child: w.Text{Text: "target"}}}}}}
	}
	app = build()
	tt := tester.New(w.Builder{Builder: func(w.BuildContext) w.Widget { return app }}, 600, 400)
	r, _ := tt.Find("target")
	tt.Hover(r.X+2, r.Y+2)
	if !contains(tt.Texts(), "Text  "+fmtSize(r.W, r.H)) {
		t.Fatalf("hover label: %v", tt.Texts())
	}
	// Clicking selects instead of pressing the app's button.
	tt.Tap(r.X+2, r.Y+2)
	if pressed != 0 {
		t.Fatal("the click reached the app")
	}
	texts := strings.Join(tt.Texts(), "\n")
	for _, want := range []string{"Text", "render: RenderParagraph", "ancestors:", "GestureDetector", "Padding"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("panel lacks %q:\n%s", want, texts)
		}
	}
	tt.Key(w.KeyEscape)
	if !exited {
		t.Fatal("Escape didn't exit")
	}
	// Disabled, clicks reach the app again.
	enabled = false
	app = build()
	tt.Root.Reassemble()
	tt.Pump()
	tt.Tap(r.X+2, r.Y+2)
	if pressed != 1 {
		t.Fatalf("disabled inspector still intercepts: %d", pressed)
	}
}

func fmtSize(wd, ht float32) string {
	return ftoa(wd) + "×" + ftoa(ht)
}

func ftoa(v float32) string { return fmt.Sprintf("%g", v) }

func TestPerformanceOverlayAndDumps(t *testing.T) {
	tt := tester.New(devtools.PerformanceOverlay{Enabled: true, Child: w.Center{Child: w.Text{Text: "app"}}}, 600, 400)
	tt.Advance(200 * time.Millisecond)
	texts := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, "fps") || !strings.Contains(texts, "rebuilt") {
		t.Fatalf("overlay: %s", texts)
	}
	if st := tt.Build.Stats(); st.Frames < 5 || st.Commands == 0 {
		t.Fatalf("stats: %+v", st)
	}
	dump := devtools.DumpWidgets(tt.Root.Context())
	if !strings.Contains(dump, "devtools.PerformanceOverlay") || !strings.Contains(dump, `Text "app"`) {
		t.Fatalf("widget dump:\n%s", dump)
	}
	if rd := devtools.DumpRender(tt.Pipeline.Root()); !strings.Contains(rd, "RenderParagraph") {
		t.Fatalf("render dump:\n%s", rd)
	}
}
