package themeeditor

import (
	"fmt"
	"strings"

	"golang.org/x/image/font/gofont/gomono"

	"github.com/minelifes/nectar_ui/ui"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// Run opens the editor window and blocks until it's closed.
func Run(vm *VM) error {
	title := "Nectar theme"
	if dir := vm.Options().Dir; dir != "" {
		title += " — " + dir
	}
	return ui.NewApp(ui.DefaultConfig().WithTitle(title).WithSize(1440, 900)).Run(Editor{VM: vm})
}

// chrome is the editor's own theme (the design is shown in the preview).
var chrome = func() m.Theme {
	t := m.NewTheme(geom.Hex(0x5B5F71), false)
	t.ListTile = m.ListTileTheme{Radius: m.Dp(m.CornerFull), ContentPadding: w.Ptr(geom.InsetsHV(16, 0)), MinHeight: m.Dp(40)}
	return t
}()

// Editor is the editor's root widget.
type Editor struct{ VM *VM }

func (e Editor) Build(w.BuildContext) w.Widget {
	return m.App{Theme: chrome, Home: editorPage{vm: e.VM}}
}

type editorPage struct{ vm *VM }

func (p editorPage) Build(ctx w.BuildContext) w.Widget {
	vm := p.vm
	return w.Focus{Autofocus: true, OnKey: func(e w.KeyEvent) bool {
		if e.Key == w.KeyS && e.Mods.Shortcut() {
			_ = vm.Save()
			return true
		}
		return false
	}, Child: m.Scaffold{
		AppBar: toolbar{vm: vm},
		Body: w.Row{Cross: w.CrossStretch, Children: []w.Widget{
			w.SizedBox{Width: 260, Child: sidebar{vm: vm}},
			m.VerticalDivider{},
			w.SizedBox{Width: 420, Child: sectionEditor{vm: vm}},
			m.VerticalDivider{},
			w.Expanded{Child: preview{vm: vm}},
		}},
	}}
}

// toolbar: title, save status, light/dark preview, reset, code, save.
type toolbar struct{ vm *VM }

func (t toolbar) Build(ctx w.BuildContext) w.Widget {
	vm := t.vm
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Dark)
	w.Listen(ctx, vm.Status)
	w.Listen(ctx, vm.Saved)
	th := m.ThemeOf(ctx)
	status := vm.Status.Get()
	if vm.Dirty() {
		status = fmt.Sprintf("light %d · dark %d edits · unsaved", vm.EditsIn(false), vm.EditsIn(true))
	}
	dark := vm.Dark.Get()
	other := "light"
	if !dark {
		other = "dark"
	}
	mode := 0
	if dark {
		mode = 1
	}
	actions := []w.Widget{
		w.Padding{Padding: geom.InsetsHV(12, 0), Child: w.Text{Text: status, Style: m.Styled(th.Text.BodySmall, th.Scheme.OnSurfaceVariant)}},
		// Which design is edited and previewed: the two are independent.
		w.SizedBox{Width: 200, Child: m.SegmentedButton{Segments: []m.Segment{{Value: 0, Label: "Light", Icon: icons.LightMode}, {Value: 1, Label: "Dark", Icon: icons.DarkMode}},
			Selected: []any{mode}, OnChanged: func(v []any) {
				if len(v) > 0 {
					vm.Dark.Set(v[0] == 1)
				}
			}, Style: m.ButtonStyle{MinHeight: m.Dp(36)}}},
		w.SizedBox{Width: 8},
		m.Tooltip{Message: "Replace this design with a copy of the " + other + " one", Child: m.TextButton{Label: "Copy from " + other, Icon: icons.ContentCopy,
			OnPressed: func() { vm.CopyFromOtherMode() }}},
		m.TextButton{Label: "Reset", Icon: icons.RestartAlt, OnPressed: func() { confirmReset(ctx, vm) }},
		w.SizedBox{Width: 8},
		m.OutlinedButton{Label: "Code", Icon: icons.Code, OnPressed: func() { showCode(ctx, vm) }},
		w.SizedBox{Width: 8},
	}
	if vm.Options().Dir != "" {
		var save func()
		if vm.Dirty() {
			save = func() { _ = vm.Save() }
		}
		actions = append(actions, m.FilledButton{Label: "Save", Icon: icons.Save, OnPressed: save}, w.SizedBox{Width: 8})
	}
	return m.AppBar{Title: "Theme editor", Leading: w.Padding{Padding: geom.InsetsHV(12, 0), Child: w.Icon{Icon: icons.Palette}},
		Actions: actions, ScrolledUnder: true}
}

func confirmReset(ctx w.BuildContext, vm *VM) {
	m.ShowDialog(ctx, func(ctx w.BuildContext) w.Widget {
		mode := "light"
		if vm.Dark.Get() {
			mode = "dark"
		}
		return m.AlertDialog{Title: "Start the " + mode + " theme over?", Content: "Every edit of the " + mode + " theme goes back to the Material 3 baseline. The other theme is kept.", Actions: []w.Widget{
			m.TextButton{Label: "Cancel", OnPressed: func() { m.Pop(ctx, nil) }},
			m.TextButton{Label: "Reset", OnPressed: func() { vm.ResetMode(); m.Pop(ctx, nil) }},
		}}
	}, nil)
}

// sidebar lists the sections, with a search field and edit counts.
type sidebar struct{ vm *VM }

func (s sidebar) Build(ctx w.BuildContext) w.Widget {
	return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(12, 12, 12, 8), Child: m.TextField{Hint: "Search components", Prefix: icons.Search, Outlined: true,
			OnChanged: s.vm.Filter.Set}},
		w.Expanded{Child: sectionList{vm: s.vm}},
	}}
}

type sectionList struct{ vm *VM }

func (l sectionList) Build(ctx w.BuildContext) w.Widget {
	vm := l.vm
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Dark)
	w.Listen(ctx, vm.Filter)
	w.Listen(ctx, vm.Section)
	th := m.ThemeOf(ctx)
	cur := vm.Section.Get()
	var rows []w.Widget
	group := ""
	for _, sec := range vm.VisibleSections() {
		name := sec.Name
		if g := sec.Group(); g != group {
			group = g
			rows = append(rows, w.KeyedSubtree{ID: "group:" + g, Child: w.Padding{Padding: geom.InsetsLTRB(16, 16, 16, 6),
				Child: w.Text{Text: g, Style: m.Styled(th.Text.TitleSmall, th.Scheme.OnSurfaceVariant)}}})
		}
		var trailing w.Widget
		if n := vm.EditCount(sec); n > 0 {
			trailing = w.Text{Text: fmt.Sprint(n), Style: m.Styled(th.Text.LabelMedium, th.Scheme.Primary)}
		}
		rows = append(rows, w.KeyedSubtree{ID: name, Child: m.ListTile{Title: sec.Title, Dense: true, Selected: name == cur, Trailing: trailing,
			OnTap: func() { vm.Section.Set(name) }}})
	}
	if len(rows) == 0 {
		rows = append(rows, w.Padding{Padding: geom.Insets(16), Child: w.Text{Text: "Nothing matches."}})
	}
	return w.ListView{Padding: geom.InsetsHV(8, 0), Children: rows}
}

// sectionEditor edits the fields of the selected section.
type sectionEditor struct{ vm *VM }

func (e sectionEditor) Build(ctx w.BuildContext) w.Widget {
	vm := e.vm
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Dark)
	w.Listen(ctx, vm.Section)
	th := m.ThemeOf(ctx)
	sec := vm.Current()
	var reset func()
	if vm.EditCount(sec) > 0 {
		reset = func() { vm.ResetSection(sec) }
	}
	modeName := "Light theme"
	if vm.Dark.Get() {
		modeName = "Dark theme"
	}
	kids := []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(16, 12, 16, 0), Child: w.Text{Text: modeName, Style: m.Styled(th.Text.LabelMedium, th.Scheme.Primary)}},
		w.Padding{Padding: geom.InsetsLTRB(16, 0, 8, 4), Child: w.Row{Cross: w.CrossCenter, Children: []w.Widget{
			w.Expanded{Child: w.Text{Text: sec.Title, Style: m.Styled(th.Text.TitleLarge, th.Scheme.OnSurface)}},
			m.TextButton{Label: "Reset section", OnPressed: reset},
		}}},
	}
	note := func(s string) {
		kids = append(kids, w.Padding{Padding: geom.InsetsLTRB(16, 0, 16, 8), Child: w.Text{Text: s, Style: m.Styled(th.Text.BodySmall, th.Scheme.OnSurfaceVariant)}})
	}
	switch {
	case sec.Name == "General":
		note("The seed color generates this theme's Material 3 palette. The light and dark themes each have their own seed and edits.")
		kids = append(kids, seedEditor{vm: vm})
	case sec.Name == "Scheme":
		note("Unedited roles follow the seed.")
	case strings.HasPrefix(sec.Name, "Text."):
		note("Text style " + sec.Title + ". Unset fields keep the Material 3 type scale.")
	default:
		note("Unset fields use the Material 3 defaults (shown greyed).")
	}
	for _, f := range sec.Fields {
		kids = append(kids, fieldEditor{vm: vm, field: f})
	}
	kids = append(kids, w.SizedBox{Height: 24})
	return w.KeyedSubtree{ID: sec.Name, Child: w.ListView{Children: kids}}
}

// seedEditor edits the seed color.
type seedEditor struct{ vm *VM }

func (s seedEditor) Build(ctx w.BuildContext) w.Widget {
	vm := s.vm
	w.Listen(ctx, vm.Rev)
	th := m.ThemeOf(ctx)
	seed := vm.Seed()
	presets := []geom.Color{m.BaselineSeed, geom.Hex(0x0B57D0), geom.Hex(0x006A6A), geom.Hex(0x386A20), geom.Hex(0x8B5000), geom.Hex(0xB3261E), geom.Hex(0x5B5F71)}
	var chips []w.Widget
	for _, c := range presets {
		c := c
		chips = append(chips, swatch{color: c, size: 28, onTap: func() { vm.SetSeed(c) }})
	}
	return w.Padding{Padding: geom.InsetsLTRB(16, 4, 16, 12), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 8, Children: []w.Widget{
		w.Text{Text: "Seed", Style: m.Styled(th.Text.LabelLarge, th.Scheme.Primary)},
		w.Row{Spacing: 12, Cross: w.CrossCenter, Children: []w.Widget{
			swatch{color: seed, onTap: func() { openColorPicker(ctx, vm, seed, vm.SetSeed) }},
			w.Expanded{Child: hexField{color: seed, set: true, onColor: vm.SetSeed}},
		}},
		w.Wrap{Spacing: 6, RunSpacing: 6, Children: chips},
	}}}
}

// ---------------------------------------------------------------------------
// Code view

var monoFont = func() *text.Font {
	f, err := text.ParseFont("Go Mono", gomono.TTF)
	if err != nil {
		return text.DefaultFont()
	}
	return f
}()

func showCode(ctx w.BuildContext, vm *VM) {
	m.ShowModalSideSheet(ctx, m.SideSheet{Title: "Generated code", Width: 760, Child: codeView{vm: vm}}, nil)
}

// codeView shows one of the two generated files (the previewed mode's
// first) with Copy and Save.
type codeView struct{ vm *VM }

func (codeView) CreateState() w.State { return &codeViewState{} }

type codeViewState struct {
	w.StateBase
	dark bool
}

func (s *codeViewState) InitState() { s.dark = w.WidgetOf[codeView](s).vm.Dark.Get() }

func (s *codeViewState) Build(ctx w.BuildContext) w.Widget {
	vm := w.WidgetOf[codeView](s).vm
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Status)
	th := m.ThemeOf(ctx)
	file := 0
	if s.dark {
		file = 1
	}
	code, err := vm.Code(s.dark)
	if err != nil {
		code = "// " + err.Error()
	}
	// The design line is long and only for the editor: show it shortened.
	shown := code
	if i := strings.LastIndex(shown, "\n"+Marker); i >= 0 {
		shown = shown[:i+1] + Marker + "{…}\n"
	}
	buttons := []w.Widget{
		w.SizedBox{Width: 300, Child: m.SegmentedButton{Segments: []m.Segment{{Value: 0, Label: LightFile}, {Value: 1, Label: DarkFile}},
			Selected: []any{file}, OnChanged: func(v []any) {
				if len(v) > 0 {
					s.SetState(func() { s.dark = v[0] == 1 })
				}
			}, Style: m.ButtonStyle{MinHeight: m.Dp(36)}}},
		m.FilledTonalButton{Label: "Copy", Icon: icons.ContentCopy, OnPressed: func() {
			if cb := ctx.Owner().Clipboard; cb != nil {
				if cb.WriteText(code) == nil {
					vm.Status.Set("Code copied")
				}
			}
		}},
	}
	if vm.Options().Dir != "" {
		buttons = append(buttons, m.FilledButton{Label: "Save both", Icon: icons.Save, OnPressed: func() { _ = vm.Save() }})
	}
	kids := []w.Widget{w.Row{ShrinkMain: true, Spacing: 8, Children: buttons}}
	if s := vm.Status.Get(); s != "" {
		kids = append(kids, w.Text{Text: s, Style: m.Styled(th.Text.BodySmall, th.Scheme.OnSurfaceVariant)})
	}
	kids = append(kids, m.Card{Variant: m.CardFilled, Padding: geom.Insets(16), Child: w.Text{Text: shown,
		Style: text.Style{Font: monoFont, Size: 13, LineHeight: 1.4, Color: th.Scheme.OnSurface}}})
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: kids}
}
