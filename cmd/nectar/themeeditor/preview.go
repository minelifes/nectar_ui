package themeeditor

import (
	"reflect"
	"strings"
	"testing/fstest"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// preview shows the designed theme: the selected section's demo first,
// then every component. It is its own material.App, so dialogs, menus,
// snack bars and sheets opened from it use the designed theme too.
type preview struct{ vm *VM }

func (p preview) Build(ctx w.BuildContext) w.Widget {
	vm := p.vm
	w.Listen(ctx, vm.Rev)
	w.Listen(ctx, vm.Dark)
	w.Listen(ctx, vm.Section)
	th := vm.Theme(vm.Dark.Get())
	return m.App{Theme: th, Home: previewPage{section: vm.Current()}}
}

type previewPage struct{ section Section }

func (p previewPage) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	heading := func(s string) w.Widget {
		return w.Padding{Padding: geom.InsetsLTRB(0, 16, 0, 8), Child: w.Text{Text: s, Style: m.Styled(th.Text.TitleMedium, th.Scheme.OnSurfaceVariant)}}
	}
	left := func(child w.Widget) w.Widget { return w.Align{Alignment: geom.TopLeft, Child: child} }
	kids := []w.Widget{heading("Preview · " + p.section.Title), left(demoFor(p.section.Name)), w.SizedBox{Height: 16}, m.Divider{}, heading("All components")}
	for _, d := range demos {
		kids = append(kids, w.Text{Text: d.title, Style: m.Styled(th.Text.LabelLarge, th.Scheme.Outline)}, left(d.build()), w.SizedBox{Height: 12})
	}
	return w.ListView{Padding: geom.InsetsHV(24, 8), Spacing: 4, Children: kids}
}

// demoFor returns the demo showing a section.
func demoFor(section string) w.Widget {
	switch {
	case section == "Scheme":
		return schemeDemo{}
	case strings.HasPrefix(section, "Text."):
		return typeScaleDemo{highlight: strings.TrimPrefix(section, "Text.")}
	}
	for _, d := range demos {
		for _, s := range d.sections {
			if s == section {
				return d.build()
			}
		}
	}
	return demos[0].build()
}

type demo struct {
	title    string
	sections []string // theme sections this demo shows
	build    func() w.Widget
}

var nop = func() {}

// demos are the component groups of the preview.
var demos = []demo{
	{"Buttons", []string{"General", "ElevatedButton", "FilledButton", "FilledTonalButton", "OutlinedButton", "TextButton"}, func() w.Widget {
		return w.Wrap{Spacing: 8, RunSpacing: 8, Children: []w.Widget{
			m.ElevatedButton{Label: "Elevated", OnPressed: nop}, m.FilledButton{Label: "Filled", Icon: icons.Add, OnPressed: nop},
			m.FilledTonalButton{Label: "Tonal", OnPressed: nop}, m.OutlinedButton{Label: "Outlined", OnPressed: nop},
			m.TextButton{Label: "Text", OnPressed: nop}, m.FilledButton{Label: "Disabled"}, m.OutlinedButton{Label: "Disabled"},
		}}
	}},
	{"Icon buttons, FAB, segmented", []string{"IconButton", "FAB", "SegmentedButton"}, func() w.Widget {
		return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 8, Children: []w.Widget{
			w.Row{ShrinkMain: true, Spacing: 8, Cross: w.CrossCenter, Children: []w.Widget{
				m.IconButton{Icon: icons.Favorite, OnPressed: nop}, m.IconButton{Icon: icons.Favorite, Variant: m.IconButtonFilled, OnPressed: nop},
				m.IconButton{Icon: icons.Favorite, Variant: m.IconButtonTonal, OnPressed: nop}, m.IconButton{Icon: icons.Favorite, Variant: m.IconButtonOutlined, OnPressed: nop},
				m.IconButton{Icon: icons.Favorite, Variant: m.IconButtonOutlined, Toggle: true, Selected: true, OnPressed: nop},
				m.FloatingActionButton{Icon: icons.Edit, Size: m.FABSmall, OnPressed: nop}, m.FloatingActionButton{Icon: icons.Edit, OnPressed: nop},
				m.FloatingActionButton{Icon: icons.Edit, Label: "Compose", OnPressed: nop},
			}},
			w.SizedBox{Width: 360, Child: m.SegmentedButton{Segments: []m.Segment{{Value: 1, Label: "Day"}, {Value: 2, Label: "Week"}, {Value: 3, Label: "Month"}},
				Selected: []any{2}, OnChanged: func([]any) {}}},
		}}
	}},
	{"Selection", []string{"Checkbox", "Radio", "Switch", "Slider"}, func() w.Widget {
		return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
			w.Row{ShrinkMain: true, Cross: w.CrossCenter, Children: []w.Widget{
				m.Checkbox{Value: true, OnChanged: func(bool) {}}, m.Checkbox{OnChanged: func(bool) {}}, m.Checkbox{Value: true, Error: true, OnChanged: func(bool) {}},
				m.Radio[int]{Value: 1, GroupValue: 1, OnChanged: func(int) {}}, m.Radio[int]{Value: 2, GroupValue: 1, OnChanged: func(int) {}},
				m.Switch{Value: true, OnChanged: func(bool) {}}, m.Switch{OnChanged: func(bool) {}}, m.Switch{Value: true, ThumbIcon: true, OnChanged: func(bool) {}},
			}},
			w.SizedBox{Width: 420, Child: m.Slider{Value: 40, Max: 100, OnChanged: func(float32) {}}},
			w.SizedBox{Width: 420, Child: m.Slider{Value: 3, Max: 5, Divisions: 5, OnChanged: func(float32) {}}},
		}}
	}},
	{"Chips", []string{"Chip"}, func() w.Widget {
		return w.Wrap{Spacing: 8, RunSpacing: 8, Children: []w.Widget{
			m.AssistChip{Label: "Assist", Icon: icons.Event, OnPressed: nop}, m.FilterChip{Label: "Filter", Selected: true, OnSelected: func(bool) {}},
			m.FilterChip{Label: "Filter", OnSelected: func(bool) {}}, m.InputChip{Label: "Input", OnDeleted: nop},
			m.SuggestionChip{Label: "Suggestion", OnPressed: nop}, m.AssistChip{Label: "Elevated", Elevated: true, OnPressed: nop},
		}}
	}},
	{"Text input", []string{"Input", "SearchBar", "DropdownMenu", "Menu"}, func() w.Widget {
		return w.SizedBox{Width: 420, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			m.TextField{Label: "Name", Helper: "Your full name"},
			m.TextField{Label: "Email", Outlined: true, Prefix: icons.Mail},
			m.TextField{Label: "Password", Error: "Too short", Obscure: true},
			m.SearchBar{Hint: "Search"},
			w.Row{Children: []w.Widget{m.DropdownMenu{Label: "Size", Entries: []m.DropdownEntry{{Label: "Small", Value: 1}, {Label: "Large", Value: 2}}, Selected: 1, OnSelected: func(any) {}}}},
		}}}
	}},
	{"Lists", []string{"ListTile", "ExpansionTile", "Divider"}, func() w.Widget {
		return w.SizedBox{Width: 480, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
			m.ListTile{Title: "Inbox", Subtitle: "3 new messages", Leading: w.Icon{Icon: icons.Inbox}, Trailing: w.Text{Text: "12:30"}, OnTap: nop},
			m.ListTile{Title: "Selected", Leading: w.Icon{Icon: icons.Star}, Selected: true, OnTap: nop},
			m.Divider{},
			m.SwitchListTile{Title: "Notifications", Value: true, OnChanged: func(bool) {}},
			m.ExpansionTile{Title: "More", Leading: w.Icon{Icon: icons.Folder}, Children: []w.Widget{m.ListTile{Title: "Nested"}}},
		}}}
	}},
	{"Containers", []string{"Card", "Badge", "Avatar", "Banner", "Tooltip"}, func() w.Widget {
		card := func(v m.CardVariant, label string) w.Widget {
			return m.Card{Variant: v, OnTap: nop, Padding: geom.Insets(16), Child: w.SizedBox{Width: 120, Child: w.Text{Text: label}}}
		}
		return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			w.Row{ShrinkMain: true, Spacing: 12, Children: []w.Widget{card(m.CardElevated, "Elevated card"), card(m.CardFilled, "Filled card"), card(m.CardOutlined, "Outlined card")}},
			w.Row{ShrinkMain: true, Spacing: 20, Cross: w.CrossCenter, Children: []w.Widget{
				m.Badge{Label: "3", Child: w.Icon{Icon: icons.Mail}}, m.Badge{Child: w.Icon{Icon: icons.Notifications}},
				m.CircleAvatar{Text: "AB"}, m.Tooltip{Message: "A tooltip", Child: w.Text{Text: "Hover for a tooltip"}},
			}},
			w.SizedBox{Width: 520, Child: m.MaterialBanner{Icon: icons.Wifi, Content: "You're offline. Some features aren't available.",
				Actions: []w.Widget{m.TextButton{Label: "Retry", OnPressed: nop}}}},
		}}
	}},
	{"Progress", []string{"Progress"}, func() w.Widget {
		return w.Row{ShrinkMain: true, Spacing: 24, Cross: w.CrossCenter, Children: []w.Widget{
			w.SizedBox{Width: 240, Child: m.LinearProgressIndicator{Value: 0.6}}, m.CircularProgressIndicator{Value: 0.7},
		}}
	}},
	{"Top and bottom bars", []string{"AppBar", "BottomAppBar", "TitleBar", "TabBar"}, func() w.Widget {
		return w.SizedBox{Width: 520, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 8, Children: []w.Widget{
			m.TitleBar{Title: "My app", Leading: []w.Widget{w.Icon{Icon: icons.Palette}}},
			m.AppBar{Title: "Title", NoAutoBack: true, Leading: m.IconButton{Icon: icons.Menu, OnPressed: nop}, Actions: []w.Widget{m.IconButton{Icon: icons.Search, OnPressed: nop}}},
			m.AppBar{Title: "Large title", Variant: m.AppBarLarge, NoAutoBack: true},
			m.TabBar{Tabs: []m.Tab{{Text: "Flights"}, {Text: "Trips"}, {Text: "Explore"}}, Selected: 0, OnChanged: func(int) {}},
			m.BottomAppBar{Actions: []w.Widget{m.IconButton{Icon: icons.Search, OnPressed: nop}, m.IconButton{Icon: icons.Delete, OnPressed: nop}},
				FAB: m.FloatingActionButton{Icon: icons.Add, Lowered: true, OnPressed: nop}},
		}}}
	}},
	{"Navigation", []string{"NavigationBar", "NavigationRail", "NavigationDrawer"}, func() w.Widget {
		dests := []m.NavigationDestination{{Icon: icons.Home, Label: "Home"}, {Icon: icons.Search, Label: "Search", Badge: "3"}, {Icon: icons.Settings, Label: "Settings"}}
		return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			w.SizedBox{Width: 520, Child: m.NavigationBar{Destinations: dests, Selected: 0, OnSelected: func(int) {}}},
			w.SizedBox{Height: 280, Child: w.Row{Cross: w.CrossStretch, Children: []w.Widget{
				m.NavigationRail{Destinations: dests, Selected: 1, OnSelected: func(int) {}},
				w.Align{Alignment: geom.TopLeft, Child: w.SizedBox{Height: 280, Child: m.NavigationDrawer{Destinations: dests, Selected: 2, OnSelected: func(int) {}}}},
			}}},
		}}
	}},
	{"Dialogs and sheets", []string{"Dialog", "BottomSheet", "SideSheet", "SnackBar", "DatePicker", "TimePicker"}, func() w.Widget {
		return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			overlayButtons{},
			w.SizedBox{Width: 400, Child: m.AlertDialog{Icon: icons.Delete, Title: "Delete draft?", Content: "This can't be undone.",
				Actions: []w.Widget{m.TextButton{Label: "Cancel", OnPressed: nop}, m.TextButton{Label: "Delete", OnPressed: nop}}}},
			w.SizedBox{Width: 400, Child: m.BottomSheet{Child: w.Padding{Padding: geom.InsetsHV(24, 0), Child: w.Text{Text: "Bottom sheet"}}}},
			w.SizedBox{Width: 360, Child: m.CalendarDatePicker{Selected: time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local), OnChanged: func(time.Time) {}}},
		}}
	}},
	{"Data", []string{"DataTable", "Stepper"}, func() w.Widget {
		return w.SizedBox{Width: 520, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			m.DataTable{Columns: []m.DataColumn{{Label: "Dessert", OnSort: func(bool) {}}, {Label: "Calories", Numeric: true}}, SortColumn: 0, SortAscending: true,
				Rows: []m.DataRow{{Cells: m.TextCells("Frozen yogurt", "159"), Selected: true, OnSelectChanged: func(bool) {}}, {Cells: m.TextCells("Eclair", "262"), OnSelectChanged: func(bool) {}}}},
			m.Stepper{Current: 1, Steps: []m.Step{{Title: "Account", State: m.StepComplete}, {Title: "Details", Content: w.Text{Text: "Fill in your details"}}, {Title: "Done"}},
				OnContinue: nop, OnCancel: nop, OnStepTapped: func(int) {}},
		}}}
	}},
	{"Tree, split view, carousel", []string{"TreeView", "SplitView", "Carousel"}, func() w.Widget {
		tree := w.NewTreeController()
		tree.Select("src/main.go")
		tree.Expand("src")
		files := fstest.MapFS{"src/main.go": {}, "src/app.go": {}, "README.md": {}}
		return w.SizedBox{Width: 520, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
			w.SizedBox{Height: 160, Child: m.SplitView{Panes: []w.Pane{
				{Child: m.FileTree{FS: files, Controller: tree, ShowGuides: true}},
				{Child: m.Card{Variant: m.CardFilled, Child: w.Center{Child: w.Text{Text: "Pane"}}}},
			}}},
			w.SizedBox{Height: 140, Child: m.Carousel{Items: []w.Widget{w.Text{Text: "1"}, w.Text{Text: "2"}, w.Text{Text: "3"}, w.Text{Text: "4"}}}},
		}}}
	}},
}

// overlayButtons open the overlay components with the designed theme.
type overlayButtons struct{}

func (overlayButtons) Build(ctx w.BuildContext) w.Widget {
	return w.Wrap{Spacing: 8, RunSpacing: 8, Children: []w.Widget{
		m.OutlinedButton{Label: "Dialog", OnPressed: func() {
			m.ShowDialog(ctx, func(ctx w.BuildContext) w.Widget {
				return m.AlertDialog{Title: "Dialog", Content: "Styled by Theme.Dialog.", Actions: []w.Widget{m.TextButton{Label: "Close", OnPressed: func() { m.Pop(ctx, nil) }}}}
			}, nil)
		}},
		m.OutlinedButton{Label: "Bottom sheet", OnPressed: func() {
			m.ShowModalBottomSheet(ctx, func(w.BuildContext) w.Widget {
				return w.Padding{Padding: geom.InsetsHV(24, 0), Child: w.Text{Text: "Styled by Theme.BottomSheet."}}
			}, nil)
		}},
		m.OutlinedButton{Label: "Side sheet", OnPressed: func() {
			m.ShowModalSideSheet(ctx, m.SideSheet{Title: "Side sheet", Child: w.Text{Text: "Styled by Theme.SideSheet."}}, nil)
		}},
		m.OutlinedButton{Label: "Snack bar", OnPressed: func() {
			m.ShowSnackBar(ctx, m.SnackBar{Message: "Styled by Theme.SnackBar", ActionLabel: "Undo", ShowClose: true})
		}},
		m.OutlinedButton{Label: "Menu", OnPressed: func() {
			var at geom.Rect
			if ro := ctx.RenderObject(); ro != nil {
				at = geom.RectFrom(render.GlobalOrigin(ro), ro.Base().Size())
			}
			m.ShowMenu(ctx, at, []m.MenuItem{{Label: "Cut", Leading: icons.ContentCut, Trailing: "⌘X"}, {Label: "Copy", Leading: icons.ContentCopy, Selected: true}, {Divider: true}, {Label: "Disabled", Disabled: true}}, m.MenuOptions{})
		}},
		m.OutlinedButton{Label: "Date picker", OnPressed: func() { m.ShowDatePicker(ctx, m.DatePickerOptions{Initial: time.Now()}) }},
		m.OutlinedButton{Label: "Time picker", OnPressed: func() { m.ShowTimePicker(ctx, 9, 30, nil) }},
	}}
}

// schemeDemo shows every color role of the designed scheme.
type schemeDemo struct{}

func (schemeDemo) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	var kids []w.Widget
	for _, f := range Sections()[1].Fields {
		c, _ := Value(th, f.Path)
		col, _ := c.(geom.Color)
		name := strings.TrimPrefix(f.Path, "Scheme.")
		kids = append(kids, w.SizedBox{Width: 150, Child: w.Row{Spacing: 8, Cross: w.CrossCenter, Children: []w.Widget{
			w.Container{Width: 28, Height: 28, Color: col, Border: &geom.Border{Radius: 6, Color: th.Scheme.OutlineVariant, Width: 1}},
			w.Expanded{Child: w.Text{Text: name, Style: m.Styled(th.Text.LabelSmall, th.Scheme.OnSurfaceVariant), MaxLines: 2}},
		}}})
	}
	return w.Wrap{Spacing: 8, RunSpacing: 8, Children: kids}
}

// typeScaleDemo shows each text style, the edited one highlighted.
type typeScaleDemo struct{ highlight string }

func (d typeScaleDemo) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	var kids []w.Widget
	for _, s := range Sections() {
		name, ok := strings.CutPrefix(s.Name, "Text.")
		if !ok {
			continue
		}
		v := reflect.ValueOf(th.Text).FieldByName(name).Interface().(text.Style)
		c := th.Scheme.OnSurfaceVariant
		if name == d.highlight {
			c = th.Scheme.Primary
		}
		kids = append(kids, w.Row{Spacing: 16, Cross: w.CrossCenter, Children: []w.Widget{
			w.SizedBox{Width: 120, Child: w.Text{Text: name, Style: m.Styled(th.Text.LabelSmall, c)}},
			w.Expanded{Child: w.Text{Text: "The quick brown fox", Style: v, MaxLines: 1, Ellipsis: true}},
		}})
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 4, Children: kids}
}
