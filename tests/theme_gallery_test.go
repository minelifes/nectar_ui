package tests

import (
	"reflect"

	"fmt"
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// themeGallery shows (nearly) every Material component, several states each.
func themeGallery(th m.Theme, dark bool) w.Widget {
	nop := func() {}
	tree := w.NewTreeController()
	tree.Select("c.txt")
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	dests := []m.NavigationDestination{{Icon: icons.Home, Label: "Home"}, {Icon: icons.Search, Label: "Search", Badge: "3"}, {Icon: icons.Settings, Label: "Set", Badge: "•"}}
	col1 := []w.Widget{
		m.AppBar{Title: "Small", Actions: []w.Widget{w.Icon{Icon: icons.Home}}}, m.AppBar{Title: "Center", Variant: m.AppBarCenterAligned, ScrolledUnder: true},
		m.AppBar{Title: "Large", Variant: m.AppBarLarge},
		w.Row{Spacing: 4, Children: []w.Widget{
			m.ElevatedButton{Label: "E", Icon: icons.Add, OnPressed: nop}, m.ElevatedButton{Label: "D"}, m.FilledTonalButton{Label: "D"}, m.FilledButton{Label: "F", Icon: icons.Add, OnPressed: nop},
			m.FilledTonalButton{Label: "T", Icon: icons.Add, OnPressed: nop}, m.OutlinedButton{Label: "O", Icon: icons.Add, OnPressed: nop}, m.TextButton{Label: "X", Icon: icons.Add, OnPressed: nop},
			m.FilledButton{Label: "D"}, m.OutlinedButton{Label: "D"}, m.TextButton{Label: "D"},
		}},
		w.Row{Spacing: 4, Children: []w.Widget{
			m.IconButton{Icon: icons.Add, OnPressed: nop}, m.IconButton{Icon: icons.Add, Variant: m.IconButtonFilled, OnPressed: nop},
			m.IconButton{Icon: icons.Add, Variant: m.IconButtonTonal, Toggle: true, OnPressed: nop},
			m.IconButton{Icon: icons.Add, Variant: m.IconButtonOutlined, Toggle: true, Selected: true, OnPressed: nop},
			m.IconButton{Icon: icons.Add, Variant: m.IconButtonFilled},
			m.FloatingActionButton{Icon: icons.Add, OnPressed: nop}, m.FloatingActionButton{Icon: icons.Add, Size: m.FABSmall, Surface: true, OnPressed: nop},
			m.FloatingActionButton{Icon: icons.Add, Label: "New", OnPressed: nop},
		}},
		m.SegmentedButton{Segments: []m.Segment{{Value: 1, Label: "A"}, {Value: 2, Label: "B", Icon: icons.Add}}, Selected: []any{1}, OnChanged: func([]any) {}},
		m.SegmentedButton{Segments: []m.Segment{{Value: 1, Label: "A"}}},
		w.Row{Spacing: 4, Children: []w.Widget{
			m.Card{Child: w.SizedBox{Width: 40, Height: 30}}, m.Card{Variant: m.CardFilled, OnTap: nop, Child: w.SizedBox{Width: 40, Height: 30}},
			m.Card{Variant: m.CardOutlined, Child: w.SizedBox{Width: 40, Height: 30}},
			m.Badge{Label: "9", Child: w.Icon{Icon: icons.Home}}, m.Badge{Child: w.Icon{Icon: icons.Home}},
			m.CircleAvatar{Text: "AB"}, m.CircleAvatar{Icon: icons.Home, Radius: 12},
		}},
		m.Divider{Indent: 8}, w.SizedBox{Height: 20, Child: m.VerticalDivider{}},
		m.ListTile{Title: "Tile", Subtitle: "sub", Leading: w.Icon{Icon: icons.Home}, Trailing: w.Text{Text: "t"}, OnTap: nop},
		m.ListTile{Title: "Sel", Selected: true, Overline: "OV", OnTap: nop}, m.ListTile{Title: "Dis", Disabled: true},
		m.CheckboxListTile{Title: "cb", Value: true, OnChanged: func(bool) {}},
		m.SwitchListTile{Title: "sw", Value: true, OnChanged: func(bool) {}},
		m.RadioListTile[int]{Title: "r", Value: 1, GroupValue: 1, OnChanged: func(int) {}},
		m.ExpansionTile{Title: "Exp", InitiallyExpanded: true, Children: []w.Widget{w.Text{Text: "in"}}},
		m.MaterialBanner{Icon: icons.Home, Content: "banner", Actions: []w.Widget{m.TextButton{Label: "OK", OnPressed: nop}}},
	}
	col2 := []w.Widget{
		w.Wrap{Spacing: 4, Children: []w.Widget{
			m.AssistChip{Label: "as", Icon: icons.Add, OnPressed: nop}, m.AssistChip{Label: "el", Elevated: true, OnPressed: nop},
			m.FilterChip{Label: "f", Selected: true, OnSelected: func(bool) {}}, m.FilterChip{Label: "g", Icon: icons.Add, OnSelected: func(bool) {}},
			m.ChoiceChip{Label: "c", Selected: true, OnSelected: nop}, m.InputChip{Label: "i", Icon: icons.Add, OnDeleted: nop},
			m.SuggestionChip{Label: "s", OnPressed: nop}, m.SuggestionChip{Label: "dis"},
		}},
		w.Row{Children: []w.Widget{
			m.Checkbox{Value: true, OnChanged: func(bool) {}}, m.Checkbox{OnChanged: func(bool) {}}, m.Checkbox{Indeterminate: true, Error: true, OnChanged: func(bool) {}}, m.Checkbox{Value: true},
			m.Radio[int]{Value: 1, GroupValue: 1, OnChanged: func(int) {}}, m.Radio[int]{Value: 2, GroupValue: 1, OnChanged: func(int) {}},
			m.Switch{Value: true, OnChanged: func(bool) {}}, m.Switch{OnChanged: func(bool) {}}, m.Switch{Value: true, ThumbIcon: true, OnChanged: func(bool) {}}, m.Switch{},
		}},
		m.Slider{Value: 0.3, OnChanged: func(float32) {}}, m.Slider{Value: 5, Max: 10, Divisions: 5, OnChanged: func(float32) {}}, m.Slider{Value: 0.5},
		m.RangeSlider{Start: 0.2, End: 0.7, OnChanged: func(a, b float32) {}},
		m.LinearProgressIndicator{Value: 0.4}, w.Row{Children: []w.Widget{m.CircularProgressIndicator{Value: 0.6}, m.CircularProgressIndicator{Value: 0.3, Size: 24, StrokeWidth: 2}}},
		m.TextField{Label: "Filled", Helper: "help", MaxLength: 10}, m.TextField{Label: "Out", Outlined: true, Prefix: icons.Search}, m.TextField{Label: "Err", Error: "bad"},
		m.TextField{Label: "Dis", Disabled: true, Hint: "h"},
		m.SearchBar{Hint: "search"},
		w.Row{Children: []w.Widget{m.DropdownMenu{Label: "Pick", Entries: []m.DropdownEntry{{Label: "one", Value: 1}}, Selected: 1, OnSelected: func(any) {}}}},
		m.TabBar{Tabs: []m.Tab{{Text: "One"}, {Text: "Two", Icon: icons.Home}}, Selected: 1, OnChanged: func(int) {}},
		m.TabBar{Tabs: []m.Tab{{Text: "One"}, {Text: "Two"}}, Secondary: true, OnChanged: func(int) {}},
		m.NavigationBar{Destinations: dests, Selected: 1, OnSelected: func(int) {}},
		m.BottomAppBar{Actions: []w.Widget{m.IconButton{Icon: icons.Home, OnPressed: nop}, w.Icon{Icon: icons.Home}}, FAB: m.FloatingActionButton{Icon: icons.Add, Lowered: true, OnPressed: nop}},
		m.QuickPickPanel{QuickPick: m.QuickPick{Query: "o", MaxVisible: 3, Items: []m.QuickPickItem{
			{Label: "Open File", Detail: "file.go", Hint: "Ctrl+O"}, {Label: "Close", Hint: "Ctrl+W"}}}},
		w.SizedBox{Height: 90, Child: m.Dock{Controller: w.NewDockController(&w.DockNode{Children: []*w.DockNode{
			{Tabs: []string{"a", "b"}, Active: "a"}, {Tabs: []string{"c"}}}}),
			Panels: []w.DockPanel{{ID: "a", Title: "A", Closable: true}, {ID: "b", Title: "B"}, {ID: "c", Title: "C"}}}},
	}
	col3 := []w.Widget{
		w.SizedBox{Height: 260, Child: w.Row{Children: []w.Widget{
			m.NavigationRail{Destinations: dests, Selected: 0, OnSelected: func(int) {}, Leading: m.FloatingActionButton{Icon: icons.Add, Size: m.FABSmall, OnPressed: nop}},
			w.Align{Alignment: geom.TopLeft, Child: m.NavigationDrawer{Destinations: append([]m.NavigationDestination{{Label: "Head"}}, dests...), Selected: 2, OnSelected: func(int) {}}},
		}}},
		m.Dialog{Child: w.Text{Text: "dlg"}},
		m.AlertDialog{Icon: icons.Home, Title: "Alert", Content: "content", Actions: []w.Widget{m.TextButton{Label: "OK", OnPressed: nop}}},
		m.SimpleDialog{Title: "Simple", Options: []w.Widget{m.ListTile{Title: "o"}}},
		m.BottomSheet{Child: w.Text{Text: "sheet"}},
		w.SizedBox{Height: 200, Child: w.Row{Children: []w.Widget{w.Expanded{Child: w.SizedBox{}}, m.SideSheet{Title: "Side", Open: true, OnClose: nop, Actions: []w.Widget{m.FilledButton{Label: "S", OnPressed: nop}}, Child: w.Text{Text: "body"}}}}},
		m.DataTable{Columns: []m.DataColumn{{Label: "A", OnSort: func(bool) {}}, {Label: "N", Numeric: true}}, SortColumn: 0, SortAscending: true,
			Rows: []m.DataRow{{Cells: m.TextCells("x", "1"), Selected: true, OnSelectChanged: func(bool) {}}, {Cells: m.TextCells("y", "2"), OnSelectChanged: func(bool) {}}}},
		m.Stepper{Current: 1, Steps: []m.Step{{Title: "one", State: m.StepComplete}, {Title: "two", Subtitle: "s", Content: w.Text{Text: "c"}}, {Title: "three", State: m.StepError}, {Title: "four", State: m.StepDisabled}},
			OnContinue: nop, OnCancel: nop, OnStepTapped: func(int) {}},
		m.CalendarDatePicker{Selected: day, Today: day.AddDate(0, 0, 2), OnChanged: func(time.Time) {}},
		w.SizedBox{Height: 120, Child: m.SplitView{Panes: []w.Pane{{Child: w.Text{Text: "L"}}, {Child: w.Text{Text: "R"}}}}},
		w.SizedBox{Height: 120, Child: m.FileTree{FS: fstest.MapFS{"a/b.go": {}, "c.txt": {}}, ShowRoot: true, ShowGuides: true, Controller: tree}},
		m.TitleBar{Title: "Win", Leading: []w.Widget{w.Icon{Icon: icons.Home}}, Actions: []w.Widget{m.IconButton{Icon: icons.Home, OnPressed: nop}}},
		w.SizedBox{Height: 150, Child: m.Carousel{Items: []w.Widget{w.Text{Text: "1"}, w.Text{Text: "2"}, w.Text{Text: "3"}, w.Text{Text: "4"}}}},
	}
	colOf := func(k []w.Widget) w.Widget {
		return w.SizedBox{Width: 520, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 4, Children: k}}
	}
	return m.App{Theme: th, Dark: dark, Home: m.Scaffold{Body: w.Row{Cross: w.CrossStart, Children: []w.Widget{colOf(col1), colOf(col2), colOf(col3)}}}}
}

func galleryCmds(th m.Theme) string {
	tt := tester.New(themeGallery(th, false), 1600, 2200)
	var b strings.Builder
	for _, c := range tt.Pump().Commands {
		if c.Text != nil {
			fmt.Fprintf(&b, "lines=%+v ", c.Text.Lines)
			if c.Text.Rich {
				for _, g := range c.Text.Glyphs {
					fmt.Fprintf(&b, "%v ", g.Color)
				}
			}
		}
		c.Text, c.Icon, c.Image = nil, nil, nil
		fmt.Fprintf(&b, "%+v\n", c)
	}
	return b.String()
}

// untestable lists theme fields the static gallery can't show: they apply
// while hovering, pressing or dragging, in routes and overlays, in states
// the gallery leaves out, or (qualified names) not to that component.
var untestable = map[string]bool{
	"FocusRingColor": true,
	// state layers (hover / focus / press / drag)
	"OverlayColor": true, "HoverBorderColor": true, "PressedThumbColor": true, "HoverHandleColor": true, "DraggedHandleColor": true,
	"HoverColor": true, "FocusColor": true, "ValueIndicatorColor": true, "ValueIndicatorTextStyle": true,
	// routes, overlays and timing
	"BarrierColor": true, "WaitDuration": true, "Tooltip": true, "SnackBar": true, "Menu": true, "TimePicker": true,
	"SideSheet.Radius": true, "SideSheet.ModalBackgroundColor": true, "DatePicker.BackgroundColor": true, "DatePicker.HeadlineTextStyle": true,
	// states the gallery doesn't show
	"ShadowColor": true, "ErrorColor": true, "ScrolledUnderColor": true, "InactiveThumbColor": true, "HintStyle": true,
	"CollapsedBackgroundColor": true, "CollapsedIconColor": true, "CollapsedTextColor": true, "DisabledColor": true,
	"SelectedTextColor": true, "ScrollbarColor": true, "ExtendedPadding": true, "HeadingRowColor": true, "DataRowColor": true,
	"QuickPick.Width": true, // only ShowQuickPick sizes the panel
	"DropHintColor":   true, // while dragging
	"MaxWidth":        true, "Margin": true, "ChildrenPadding": true, "InactiveIconColor": true, "IndicatorHeight": true,
	"DisabledBackgroundColor": true,
	// not used by that component (see the ButtonStyle docs)
	"TextButton.Radius": true, "IconButton.Padding": true, "IconButton.TextStyle": true,
	"SegmentedButton.Elevation": true, "SegmentedButton.MinWidth": true,
}

func skipField(theme, field string) bool {
	if untestable[field] || untestable[theme+"."+field] {
		return true
	}
	common := map[string]bool{"ElevatedButton": true, "FilledButton": true, "FilledTonalButton": true, "OutlinedButton": true, "TextButton": true}
	return common[theme] && strings.HasPrefix(field, "Selected")
}

// probe returns a value for a theme field that differs from its default.
func probe(t reflect.Type, n int, alt int) (reflect.Value, bool) {
	c := geom.Color{R: 0.9, G: float32(n%97) / 97, B: 0.1, A: 1}
	switch t {
	case reflect.TypeFor[geom.Color]():
		return reflect.ValueOf(c), true
	case reflect.TypeFor[[3]geom.Color]():
		return reflect.ValueOf([3]geom.Color{c, c, c}), true
	case reflect.TypeFor[text.Style]():
		return reflect.ValueOf(text.Style{Color: c, Size: 23}), true
	case reflect.TypeFor[*float32]():
		v := []float32{7.5, 0, 97}[alt]
		return reflect.ValueOf(&v), true
	case reflect.TypeFor[*int]():
		v := []int{5, 0, 2}[alt]
		return reflect.ValueOf(&v), true
	case reflect.TypeFor[*bool]():
		v := alt == 1
		return reflect.ValueOf(&v), true
	case reflect.TypeFor[*geom.EdgeInsets]():
		v := geom.Insets(13)
		return reflect.ValueOf(&v), true
	}
	return reflect.Value{}, false
}

// TestEveryThemeFieldIsUsed sets each field of each component theme (and
// the top-level Theme colors) on its own and checks the rendered gallery
// changes: no theme field is silently ignored.
func TestEveryThemeFieldIsUsed(t *testing.T) {
	base := galleryCmds(m.Theme{})
	themeT := reflect.TypeFor[m.Theme]()
	n := 0
	check := func(name string, set func(th *m.Theme, v reflect.Value), ft reflect.Type) {
		n++
		for alt := range 3 {
			v, ok := probe(ft, n, alt)
			if !ok {
				t.Fatalf("%s: no probe for %v", name, ft)
			}
			var th m.Theme
			set(&th, v)
			if galleryCmds(th) != base {
				return
			}
		}
		t.Errorf("%s has no visible effect", name)
	}
	for i := range themeT.NumField() {
		f := themeT.Field(i)
		switch f.Name {
		case "Seed", "Scheme", "Text":
			continue
		}
		if skipField("", f.Name) {
			continue
		}
		if f.Type.Kind() != reflect.Struct || f.Type == reflect.TypeFor[geom.Color]() {
			check(f.Name, func(th *m.Theme, v reflect.Value) { reflect.ValueOf(th).Elem().Field(i).Set(v) }, f.Type)
			continue
		}
		for j := range f.Type.NumField() {
			sf := f.Type.Field(j)
			if skipField(f.Name, sf.Name) {
				continue
			}
			check(f.Name+"."+sf.Name, func(th *m.Theme, v reflect.Value) {
				reflect.ValueOf(th).Elem().Field(i).Field(j).Set(v)
			}, sf.Type)
		}
	}
	t.Logf("checked %d theme fields", n)
}
