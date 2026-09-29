package tests

import (
	"fmt"
	"slices"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// diffScreen is a busy Material screen inside a scroll view (so its
// content sits in a repaint boundary): theme switch, text field, switch,
// checkbox, expansion tile, tabs, hover states, and a label that recolors
// only itself.
type diffScreen struct{ recolor *func() }

func (diffScreen) CreateState() w.State { return &diffScreenState{} }

type diffScreenState struct {
	w.StateBase
	dark, sw, check bool
	tab             int
}

func (s *diffScreenState) Build(w.BuildContext) w.Widget {
	recolor := w.WidgetOf[diffScreen](s).recolor
	return m.App{Dark: s.dark, Home: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		sc := m.ThemeOf(ctx).Scheme
		return w.ListView{Padding: geom.Insets(16), Spacing: 12, ThumbColor: sc.OnSurface.WithAlpha(0.4), Children: []w.Widget{
			w.Row{Spacing: 8, Children: []w.Widget{
				m.FilledButton{Label: "Theme", OnPressed: func() { s.SetState(func() { s.dark = !s.dark }) }},
				m.ElevatedButton{Label: "Other", OnPressed: func() {}},
			}},
			m.TextField{Label: "Name", Hint: "type here"},
			m.SwitchListTile{Title: "Switch", Value: s.sw, OnChanged: func(v bool) { s.SetState(func() { s.sw = v }) }},
			m.CheckboxListTile{Title: "Check", Value: s.check, OnChanged: func(v bool) { s.SetState(func() { s.check = v }) }},
			m.ExpansionTile{Title: "More", Leading: w.Icon{Icon: icons.Folder}, Children: []w.Widget{m.ListTile{Title: "Inside"}}},
			m.TabBar{Tabs: []m.Tab{{Text: "One"}, {Text: "Two"}}, Selected: s.tab,
				OnChanged: func(i int) { s.SetState(func() { s.tab = i }) }},
			w.Text{Text: fmt.Sprintf("tab %d", s.tab)},
			colorLabel{recolor: recolor},
			w.SizedBox{Height: 600, Child: w.Text{Text: "tall filler"}},
		}}
	}}}
}

// colorLabel recolors itself and nothing else: the case where a render
// object that forgets MarkNeedsPaint would leave its boundary stale.
type colorLabel struct{ recolor *func() }

func (colorLabel) CreateState() w.State { return &colorLabelState{} }

type colorLabelState struct {
	w.StateBase
	red bool
}

func (s *colorLabelState) Build(ctx w.BuildContext) w.Widget {
	*w.WidgetOf[colorLabel](s).recolor = func() { s.SetState(func() { s.red = !s.red }) }
	c := m.ThemeOf(ctx).Scheme.OnSurface
	if s.red {
		c = m.ThemeOf(ctx).Scheme.Error
	}
	return w.Text{Text: "colored label", Style: text.Style{Color: c}}
}

// visibleCmds drops commands that can't show (replays skip them early).
func visibleCmds(cmds []render.Command) []render.Command {
	var out []render.Command
	for _, c := range cmds {
		r := c.Rect
		if c.Kind == render.CmdShadow {
			g := c.Blur * 3
			r = geom.Rect{X: r.X - g, Y: r.Y - g, W: r.W + 2*g, H: r.H + 2*g}
		}
		if !r.Empty() && c.Clip.Intersect(r).Empty() {
			continue
		}
		out = append(out, c)
	}
	return out
}

func sameCmd(a, b render.Command) bool {
	near := func(x, y float32) bool { d := x - y; return d < 0.01 && d > -0.01 }
	nearRect := func(p, q geom.Rect) bool { return near(p.X, q.X) && near(p.Y, q.Y) && near(p.W, q.W) && near(p.H, q.H) }
	return a.Kind == b.Kind && nearRect(a.Rect, b.Rect) && nearRect(a.Clip, b.Clip) && a.Color == b.Color &&
		a.Radius == b.Radius && a.Icon == b.Icon && a.Image == b.Image && sameText(a.Text, b.Text) && near(a.Width, b.Width)
}

// sameText compares laid-out text (each tester has its own paragraphs).
func sameText(a, b *text.Paragraph) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Style == b.Style && slices.Equal(a.Glyphs, b.Glyphs) && slices.Equal(a.Lines, b.Lines)
}

// TestRepaintBoundaryMatchesUncached drives the same screen twice, with
// and without display-list caching, and requires identical output after
// every step. A render object that changes its looks without invalidating
// its boundary shows up here as a stale command.
func TestRepaintBoundaryMatchesUncached(t *testing.T) {
	defer func() { render.CacheRepaintBoundaries = true }()
	var recolorCached, recolorPlain func()
	cached := tester.New(diffScreen{recolor: &recolorCached}, 480, 400)
	render.CacheRepaintBoundaries = false
	plain := tester.New(diffScreen{recolor: &recolorPlain}, 480, 400)
	recolor := map[*tester.Tester]*func(){cached: &recolorCached, plain: &recolorPlain}

	steps := []struct {
		name string
		do   func(tt *tester.Tester)
	}{
		{"idle", func(*tester.Tester) {}},
		{"hover button", func(tt *tester.Tester) { tt.Hover(50, 36) }},
		{"toggle theme", func(tt *tester.Tester) { tt.TapText("Theme") }},
		{"recolor one label", func(tt *tester.Tester) { (*recolor[tt])() }},
		{"type", func(tt *tester.Tester) { tt.TapText("Name"); tt.Type("héllo 世界") }},
		{"switch", func(tt *tester.Tester) { tt.TapText("Switch") }},
		{"check", func(tt *tester.Tester) { tt.TapText("Check") }},
		{"expand", func(tt *tester.Tester) { tt.TapText("More") }},
		{"tab", func(tt *tester.Tester) { tt.TapText("Two") }},
		{"scroll", func(tt *tester.Tester) { tt.Scroll(200, 300, 120) }},
		{"theme back", func(tt *tester.Tester) { tt.Scroll(200, 300, -500); tt.TapText("Theme") }},
		{"resize", func(tt *tester.Tester) { tt.Window.SetSize(360, 500) }},
	}
	for _, st := range steps {
		render.CacheRepaintBoundaries = true
		st.do(cached)
		cached.Settle()
		a := visibleCmds(cached.Pump().Commands)
		render.CacheRepaintBoundaries = false
		st.do(plain)
		plain.Settle()
		b := visibleCmds(plain.Pump().Commands)
		if len(a) != len(b) {
			t.Fatalf("after %q: %d commands cached vs %d uncached", st.name, len(a), len(b))
		}
		for i := range a {
			if !sameCmd(a[i], b[i]) {
				t.Fatalf("after %q: command %d differs:\n cached   %+v\n uncached %+v", st.name, i, a[i], b[i])
			}
		}
	}
}
