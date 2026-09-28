// Material 3 gallery: every component, light/dark, switchable seed color.
//
//	CGO_ENABLED=0 go run ./examples/material
package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/minelifes/nectar_ui/ui"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func main() {
	app := ui.NewApp(ui.DefaultConfig().WithTitle("Nectar UI — Material 3").WithSize(1200, 800))
	if err := app.Run(Gallery{}); err != nil {
		log.Fatal(err)
	}
}

// Gallery owns app-wide settings (theme seed, dark mode).
type Gallery struct {
	Page int  // initial page
	Dark bool // initial brightness
}

func (Gallery) CreateState() w.State { return &galleryState{} }

type galleryState struct {
	w.StateBase
	seed geom.Color
	dark bool
	page int
}

func (s *galleryState) InitState() {
	g := w.WidgetOf[Gallery](s)
	s.seed, s.dark, s.page = m.BaselineSeed, g.Dark, g.Page
}

var seeds = []struct {
	name string
	c    geom.Color
}{
	{"Baseline", m.BaselineSeed}, {"Indigo", geom.Hex(0x3F51B5)}, {"Teal", geom.Hex(0x009688)},
	{"Green", geom.Hex(0x4CAF50)}, {"Orange", geom.Hex(0xFF9800)}, {"Pink", geom.Hex(0xE91E63)},
}

func (s *galleryState) Build(w.BuildContext) w.Widget {
	return m.App{Theme: m.NewTheme(s.seed, false), Dark: s.dark, Home: w.Builder{Builder: s.home}}
}

func (s *galleryState) home(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	dests := []m.NavigationDestination{
		{Icon: icons.TouchApp, Label: "Actions"},
		{Icon: icons.Notifications, Label: "Comms", Badge: "3"},
		{Icon: icons.Widgets, Label: "Contain"},
		{Icon: icons.CheckBox, Label: "Select"},
		{Icon: icons.TextFields, Label: "Text"},
		{Icon: icons.Explore, Label: "Navigate"},
	}
	builders := []func(w.BuildContext) w.Widget{actionsPage, commsPage, containPage, selectPage, textPage, navPage}
	seedItems := make([]m.MenuItem, len(seeds))
	for i, sd := range seeds {
		sd := sd
		seedItems[i] = m.MenuItem{Label: sd.name, Leading: icons.Palette, Selected: sd.c == s.seed,
			OnTap: func() { s.SetState(func() { s.seed = sd.c }) }}
	}
	themeIcon := icons.DarkMode
	if s.dark {
		themeIcon = icons.LightMode
	}
	return m.Scaffold{
		AppBar: m.AppBar{Title: "Material 3 · " + dests[s.page].Label, Actions: []w.Widget{
			m.Tooltip{Message: "Toggle dark mode", Child: m.IconButton{Icon: themeIcon, OnPressed: func() { s.SetState(func() { s.dark = !s.dark }) }}},
			m.PopupMenuButton{Icon: icons.Palette, Items: seedItems},
		}},
		Rail: m.NavigationRail{Destinations: dests, Selected: s.page,
			OnSelected: func(i int) { s.SetState(func() { s.page = i }) },
			Leading:    m.FloatingActionButton{Icon: icons.Edit, OnPressed: func() { m.ShowSnackBar(ctx, m.SnackBar{Message: "Compose tapped"}) }}},
		Body: w.KeyedSubtree{ID: s.page, Child: w.ListView{Padding: geom.InsetsLTRB(24, 8, 24, 32), Spacing: 16,
			ThumbColor: th.Scheme.OnSurfaceVariant.WithAlpha(0.4), Children: []w.Widget{w.Builder{Builder: builders[s.page]}}}},
	}
}

// section renders a titled group.
func section(ctx w.BuildContext, title string, kids ...w.Widget) w.Widget {
	th := m.ThemeOf(ctx)
	return m.Card{Variant: m.CardOutlined, Padding: geom.Insets(16), Child: w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 12, Children: append([]w.Widget{
		w.Text{Text: title, Style: m.Styled(th.Text.TitleMedium, th.Scheme.OnSurface)},
	}, kids...)}}
}

func wrap(kids ...w.Widget) w.Widget {
	return w.Wrap{Spacing: 12, RunSpacing: 12, Children: kids}
}

func noop() {}

// ---------------------------------------------------------------------------

func actionsPage(ctx w.BuildContext) w.Widget {
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Common buttons",
			wrap(m.ElevatedButton{Label: "Elevated", OnPressed: noop}, m.FilledButton{Label: "Filled", OnPressed: noop},
				m.FilledTonalButton{Label: "Tonal", OnPressed: noop}, m.OutlinedButton{Label: "Outlined", OnPressed: noop},
				m.TextButton{Label: "Text", OnPressed: noop}),
			wrap(m.ElevatedButton{Label: "Icon", Icon: icons.Add, OnPressed: noop}, m.FilledButton{Label: "Icon", Icon: icons.Send, OnPressed: noop},
				m.FilledTonalButton{Label: "Icon", Icon: icons.Share, OnPressed: noop}, m.OutlinedButton{Label: "Icon", Icon: icons.Upload, OnPressed: noop},
				m.TextButton{Label: "Icon", Icon: icons.Download, OnPressed: noop}),
			wrap(m.ElevatedButton{Label: "Disabled"}, m.FilledButton{Label: "Disabled"}, m.FilledTonalButton{Label: "Disabled"},
				m.OutlinedButton{Label: "Disabled"}, m.TextButton{Label: "Disabled"}),
		),
		section(ctx, "Icon buttons", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return iconButtons{} }}),
		section(ctx, "Floating action buttons", w.Row{Spacing: 16, Cross: w.CrossCenter, ShrinkMain: true, Children: []w.Widget{
			m.FloatingActionButton{Icon: icons.Add, Size: m.FABSmall, OnPressed: noop},
			m.FloatingActionButton{Icon: icons.Add, OnPressed: noop},
			m.FloatingActionButton{Icon: icons.Add, Size: m.FABLarge, OnPressed: noop},
			m.FloatingActionButton{Icon: icons.Navigation, Label: "Navigate", OnPressed: noop},
			m.FloatingActionButton{Icon: icons.Edit, Surface: true, OnPressed: noop},
		}}),
		section(ctx, "Segmented buttons", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return segments{} }}),
	}}
}

type iconButtons struct{}

func (iconButtons) CreateState() w.State { return &iconButtonsState{} }

type iconButtonsState struct {
	w.StateBase
	on [4]bool
}

func (s *iconButtonsState) Build(w.BuildContext) w.Widget {
	vs := []m.IconButtonVariant{m.IconButtonStandard, m.IconButtonFilled, m.IconButtonTonal, m.IconButtonOutlined}
	kids := []w.Widget{}
	for _, v := range vs {
		kids = append(kids, m.IconButton{Icon: icons.Settings, Variant: v, OnPressed: noop})
	}
	for i, v := range vs {
		i := i
		kids = append(kids, m.IconButton{Icon: icons.FavoriteBorder, SelectedIcon: icons.Favorite, Variant: v, Toggle: true, Selected: s.on[i],
			OnPressed: func() { s.SetState(func() { s.on[i] = !s.on[i] }) }})
	}
	for _, v := range vs {
		kids = append(kids, m.IconButton{Icon: icons.Block, Variant: v})
	}
	return wrap(kids...)
}

type segments struct{}

func (segments) CreateState() w.State {
	return &segmentsState{single: []any{"week"}, multi: []any{"s"}}
}

type segmentsState struct {
	w.StateBase
	single, multi []any
}

func (s *segmentsState) Build(w.BuildContext) w.Widget {
	return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
		w.SizedBox{Width: 420, Child: m.SegmentedButton{Segments: []m.Segment{
			{Value: "day", Label: "Day", Icon: icons.CalendarViewDay}, {Value: "week", Label: "Week", Icon: icons.CalendarViewWeek},
			{Value: "month", Label: "Month", Icon: icons.CalendarViewMonth}, {Value: "year", Label: "Year", Icon: icons.CalendarToday},
		}, Selected: s.single, OnChanged: func(v []any) { s.SetState(func() { s.single = v }) }}},
		w.SizedBox{Width: 320, Child: m.SegmentedButton{Multi: true, Segments: []m.Segment{
			{Value: "xs", Label: "XS"}, {Value: "s", Label: "S"}, {Value: "m", Label: "M"}, {Value: "l", Label: "L"}, {Value: "xl", Label: "XL"},
		}, Selected: s.multi, OnChanged: func(v []any) { s.SetState(func() { s.multi = v }) }}},
	}}
}

// ---------------------------------------------------------------------------

func commsPage(ctx w.BuildContext) w.Widget {
	on := true
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Badges", w.Row{Spacing: 32, ShrinkMain: true, Children: []w.Widget{
			m.Badge{Child: w.Icon{Icon: icons.Mail}},
			m.Badge{Label: "3", Child: w.Icon{Icon: icons.Notifications}},
			m.Badge{Label: "999+", Child: w.Icon{Icon: icons.Chat}},
			m.Badge{Label: "1", Show: &on, Child: m.CircleAvatar{Text: "VZ"}},
		}}),
		section(ctx, "Progress indicators",
			m.LinearProgressIndicator{Value: 0.35},
			m.LinearProgressIndicator{Indeterminate: true},
			w.Row{Spacing: 24, ShrinkMain: true, Children: []w.Widget{
				m.CircularProgressIndicator{Value: 0.7}, m.CircularProgressIndicator{Indeterminate: true},
				m.CircularProgressIndicator{Value: 0.25, Size: 32, StrokeWidth: 3},
			}}),
		section(ctx, "Snackbar", wrap(
			m.FilledTonalButton{Label: "Show snackbar", OnPressed: func() { m.ShowSnackBar(ctx, m.SnackBar{Message: "Photo archived"}) }},
			m.FilledTonalButton{Label: "With action", OnPressed: func() {
				m.ShowSnackBar(ctx, m.SnackBar{Message: "Message deleted", ActionLabel: "Undo", ShowClose: true, OnAction: func() {}})
			}},
		)),
		section(ctx, "Tooltips", wrap(
			m.Tooltip{Message: "Add to favorites", Child: m.IconButton{Icon: icons.FavoriteBorder, OnPressed: noop}},
			m.Tooltip{Message: "Share this item", Child: m.OutlinedButton{Label: "Hover me", OnPressed: noop}},
		)),
	}}
}

// ---------------------------------------------------------------------------

func containPage(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	card := func(v m.CardVariant, name string) w.Widget {
		return w.SizedBox{Width: 200, Height: 120, Child: m.Card{Variant: v, OnTap: noop, Padding: geom.Insets(16),
			Child: w.Column{Cross: w.CrossStart, Spacing: 4, Children: []w.Widget{
				w.Text{Text: name, Style: m.Styled(th.Text.TitleMedium, th.Scheme.OnSurface)},
				w.Text{Text: "Tap me", Style: m.Styled(th.Text.BodyMedium, th.Scheme.OnSurfaceVariant)},
			}}}}
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Cards", wrap(card(m.CardElevated, "Elevated"), card(m.CardFilled, "Filled"), card(m.CardOutlined, "Outlined"))),
		section(ctx, "Dialogs & sheets", wrap(
			m.FilledTonalButton{Label: "Alert dialog", OnPressed: func() {
				m.ShowDialog(ctx, func(dctx w.BuildContext) w.Widget {
					return m.AlertDialog{Icon: icons.Delete, Title: "Permanently delete?",
						Content: "Deleting the selected messages will also remove them from all synced devices.",
						Actions: []w.Widget{
							m.TextButton{Label: "Cancel", OnPressed: func() { m.Pop(dctx, false) }},
							m.TextButton{Label: "Delete", OnPressed: func() { m.Pop(dctx, true) }},
						}}
				}, func(r any) {
					if r == true {
						m.ShowSnackBar(ctx, m.SnackBar{Message: "Deleted"})
					}
				})
			}},
			m.FilledTonalButton{Label: "Simple dialog", OnPressed: func() {
				m.ShowDialog(ctx, func(dctx w.BuildContext) w.Widget {
					opt := func(name string) w.Widget {
						return m.ListTile{Leading: m.CircleAvatar{Text: name[:1]}, Title: name, OnTap: func() { m.Pop(dctx, name) }}
					}
					return m.SimpleDialog{Title: "Set backup account", Options: []w.Widget{opt("alice@example.com"), opt("bob@example.com")}}
				}, nil)
			}},
			m.FilledTonalButton{Label: "Bottom sheet", OnPressed: func() {
				m.ShowModalBottomSheet(ctx, func(sctx w.BuildContext) w.Widget {
					return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
						m.ListTile{Leading: w.Icon{Icon: icons.Share}, Title: "Share", OnTap: func() { m.Pop(sctx, nil) }},
						m.ListTile{Leading: w.Icon{Icon: icons.Link}, Title: "Get link", OnTap: func() { m.Pop(sctx, nil) }},
						m.ListTile{Leading: w.Icon{Icon: icons.Edit}, Title: "Edit name", OnTap: func() { m.Pop(sctx, nil) }},
						m.ListTile{Leading: w.Icon{Icon: icons.Delete}, Title: "Delete collection", OnTap: func() { m.Pop(sctx, nil) }},
					}}
				}, nil)
			}},
			m.FilledTonalButton{Label: "Full-screen dialog", OnPressed: func() {
				m.ShowFullScreenDialog(ctx, func(dctx w.BuildContext) w.Widget {
					return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
						m.AppBar{Title: "New event", Leading: m.IconButton{Icon: icons.Close, OnPressed: func() { m.Pop(dctx, nil) }},
							Actions: []w.Widget{m.TextButton{Label: "Save", OnPressed: func() { m.Pop(dctx, nil) }}}},
						w.Padding{Padding: geom.Insets(24), Child: m.TextField{Label: "Event name"}},
					}}
				})
			}},
		)),
		section(ctx, "Lists",
			m.ListTile{Leading: m.CircleAvatar{Text: "A"}, Title: "One-line item", Trailing: w.Text{Text: "100+"}, OnTap: noop},
			m.ListTile{Leading: w.Icon{Icon: icons.Person}, Title: "Two-line item", Subtitle: "Supporting text", Trailing: w.Icon{Icon: icons.ChevronRight}, OnTap: noop},
			m.ListTile{Leading: w.Icon{Icon: icons.Image}, Overline: "OVERLINE", Title: "Three-line item", ThreeLine: true,
				Subtitle: "Supporting text that is long enough to fill up multiple lines in the item.", OnTap: noop},
			m.ListTile{Leading: w.Icon{Icon: icons.Star}, Title: "Selected item", Selected: true, OnTap: noop},
			m.Divider{},
			m.ExpansionTile{Title: "Expansion tile", Subtitle: "Tap to expand", Leading: w.Icon{Icon: icons.Folder}, Children: []w.Widget{
				m.ListTile{Title: "Child one"}, m.ListTile{Title: "Child two"},
			}},
		),
		section(ctx, "Split views", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return splits{} }}),
		section(ctx, "Tree view", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return fileBrowser{} }}),
		section(ctx, "Carousels", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return carousels{} }}),
		section(ctx, "Side sheets", w.Builder{Builder: func(ctx w.BuildContext) w.Widget { return sideSheets{} }}),
		section(ctx, "Banner", m.MaterialBanner{Icon: icons.WifiOff, Content: "You're offline. Some features may be unavailable.",
			Actions: []w.Widget{m.TextButton{Label: "Dismiss", OnPressed: noop}, m.TextButton{Label: "Retry", OnPressed: noop}}}),
	}}
}

// ---------------------------------------------------------------------------

func selectPage(ctx w.BuildContext) w.Widget { return selection{} }

type selection struct{}

func (selection) CreateState() w.State {
	return &selectionState{checks: [3]bool{true, false, false}, radio: 1, sw: true, slider: 40, lo: 20, hi: 70,
		filters: map[string]bool{"Dogs": true}, date: time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local), hour: 9, minute: 30, fruit: "apple"}
}

type selectionState struct {
	w.StateBase
	checks       [3]bool
	radio        int
	sw, sw2      bool
	slider       float32
	lo, hi       float32
	filters      map[string]bool
	choice       string
	tags         []string
	date         time.Time
	hour, minute int
	fruit        any
}

func (s *selectionState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	if s.tags == nil {
		s.tags = []string{"Go", "wgpu", "Material"}
	}
	filterChips := []w.Widget{}
	for _, f := range []string{"Dogs", "Cats", "Birds"} {
		f := f
		filterChips = append(filterChips, m.FilterChip{Label: f, Selected: s.filters[f], OnSelected: func(v bool) { s.SetState(func() { s.filters[f] = v }) }})
	}
	choice := []w.Widget{}
	for _, c := range []string{"Small", "Medium", "Large"} {
		c := c
		choice = append(choice, m.ChoiceChip{Label: c, Selected: s.choice == c, OnSelected: func() { s.SetState(func() { s.choice = c }) }})
	}
	tagChips := []w.Widget{}
	for i, t := range s.tags {
		i := i
		tagChips = append(tagChips, m.InputChip{Label: t, OnDeleted: func() {
			s.SetState(func() { s.tags = append(s.tags[:i:i], s.tags[i+1:]...) })
		}})
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Checkboxes, radios, switches", w.Row{Spacing: 32, Cross: w.CrossStart, Children: []w.Widget{
			w.Column{ShrinkMain: true, Children: []w.Widget{
				m.Checkbox{Value: s.checks[0], OnChanged: func(v bool) { s.SetState(func() { s.checks[0] = v }) }},
				m.Checkbox{Value: s.checks[1], Indeterminate: !s.checks[1], OnChanged: func(v bool) { s.SetState(func() { s.checks[1] = v }) }},
				m.Checkbox{Value: s.checks[2], Error: true, OnChanged: func(v bool) { s.SetState(func() { s.checks[2] = v }) }},
				m.Checkbox{Value: true},
			}},
			w.Column{ShrinkMain: true, Children: []w.Widget{
				m.Radio[int]{Value: 0, GroupValue: s.radio, OnChanged: func(v int) { s.SetState(func() { s.radio = v }) }},
				m.Radio[int]{Value: 1, GroupValue: s.radio, OnChanged: func(v int) { s.SetState(func() { s.radio = v }) }},
				m.Radio[int]{Value: 2, GroupValue: s.radio},
			}},
			w.Column{ShrinkMain: true, Spacing: 8, Children: []w.Widget{
				m.Switch{Value: s.sw, OnChanged: func(v bool) { s.SetState(func() { s.sw = v }) }},
				m.Switch{Value: s.sw2, ThumbIcon: true, OnChanged: func(v bool) { s.SetState(func() { s.sw2 = v }) }},
				m.Switch{Value: true},
			}},
			w.Expanded{Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
				m.CheckboxListTile{Title: "Checkbox tile", Subtitle: "Supporting text", Value: s.checks[0], OnChanged: func(v bool) { s.SetState(func() { s.checks[0] = v }) }},
				m.RadioListTile[int]{Title: "Radio tile", Value: 0, GroupValue: s.radio, OnChanged: func(v int) { s.SetState(func() { s.radio = v }) }},
				m.SwitchListTile{Title: "Switch tile", Value: s.sw, OnChanged: func(v bool) { s.SetState(func() { s.sw = v }) }},
			}}},
		}}),
		section(ctx, "Sliders",
			m.Slider{Value: s.slider, Max: 100, OnChanged: func(v float32) { s.SetState(func() { s.slider = v }) }},
			m.Slider{Value: s.slider, Max: 100, Divisions: 10, OnChanged: func(v float32) { s.SetState(func() { s.slider = v }) }},
			m.RangeSlider{Start: s.lo, End: s.hi, Max: 100, Divisions: 20, OnChanged: func(a, b float32) { s.SetState(func() { s.lo, s.hi = a, b }) }},
			m.Slider{Value: 60, Max: 100},
			w.Text{Text: fmt.Sprintf("Value %.0f · range %.0f–%.0f", s.slider, s.lo, s.hi), Style: m.Styled(th.Text.BodySmall, th.Scheme.OnSurfaceVariant)},
		),
		section(ctx, "Chips",
			wrap(m.AssistChip{Label: "Add to calendar", Icon: icons.Event, OnPressed: noop}, m.AssistChip{Label: "Elevated", Icon: icons.Map, Elevated: true, OnPressed: noop},
				m.SuggestionChip{Label: "Sounds good", OnPressed: noop}, m.SuggestionChip{Label: "Disabled"}),
			wrap(filterChips...), wrap(choice...), wrap(tagChips...)),
		section(ctx, "Pickers & menus", w.Row{Spacing: 16, Cross: w.CrossCenter, Children: []w.Widget{
			m.OutlinedButton{Label: s.date.Format("Jan 2, 2006"), Icon: icons.CalendarToday, OnPressed: func() {
				m.ShowDatePicker(ctx, m.DatePickerOptions{Initial: s.date, OnPicked: func(t time.Time) { s.SetState(func() { s.date = t }) }})
			}},
			m.OutlinedButton{Label: fmt.Sprintf("%02d:%02d", s.hour, s.minute), Icon: icons.Schedule, OnPressed: func() {
				m.ShowTimePicker(ctx, s.hour, s.minute, func(h, mi int) { s.SetState(func() { s.hour, s.minute = h, mi }) })
			}},
			m.DropdownMenu{Label: "Fruit", Width: 200, Selected: s.fruit, Entries: []m.DropdownEntry{
				{Label: "Apple", Value: "apple"}, {Label: "Banana", Value: "banana"}, {Label: "Cherry", Value: "cherry"},
			}, OnSelected: func(v any) { s.SetState(func() { s.fruit = v }) }},
			m.PopupMenuButton{Items: []m.MenuItem{
				{Label: "Cut", Leading: icons.ContentCut, Trailing: "⌘X"}, {Label: "Copy", Leading: icons.ContentCopy, Trailing: "⌘C"},
				{Label: "Paste", Leading: icons.ContentPaste, Trailing: "⌘V", Disabled: true}, {Divider: true},
				{Label: "Settings", Leading: icons.Settings},
			}},
		}}),
		section(ctx, "Inline calendar", w.SizedBox{Width: 336, Child: m.CalendarDatePicker{Selected: s.date,
			OnChanged: func(t time.Time) { s.SetState(func() { s.date = t }) }}}),
	}}
}

// ---------------------------------------------------------------------------

func textPage(ctx w.BuildContext) w.Widget { return textInputs{} }

type textInputs struct{}

func (textInputs) CreateState() w.State { return &textInputsState{} }

type textInputsState struct {
	w.StateBase
	name, email *w.TextEditingController
	showPw      bool
}

func (s *textInputsState) InitState() {
	s.name = w.NewTextController("")
	s.email = w.NewTextController("vadym@")
}

func (s *textInputsState) Build(ctx w.BuildContext) w.Widget {
	pwIcon := icons.Visibility
	if s.showPw {
		pwIcon = icons.VisibilityOff
	}
	emailErr := ""
	if t := s.email.Text(); t != "" && !contains(t, ".") {
		emailErr = "Enter a valid email address"
	}
	field := func(tf m.TextField) w.Widget { return w.Expanded{Child: tf} }
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Filled text fields", w.Row{Spacing: 16, Cross: w.CrossStart, Children: []w.Widget{
			field(m.TextField{Controller: s.name, Label: "Name", Hint: "Your full name", Helper: "Supporting text", Prefix: icons.Person}),
			field(m.TextField{Controller: s.email, Label: "Email", Error: emailErr, Helper: "We'll never share it"}),
			field(m.TextField{Label: "Disabled", Disabled: true}),
		}}),
		section(ctx, "Outlined text fields", w.Row{Spacing: 16, Cross: w.CrossStart, Children: []w.Widget{
			field(m.TextField{Outlined: true, Label: "Password", Obscure: !s.showPw, Suffix: pwIcon,
				OnSuffix: func() { s.SetState(func() { s.showPw = !s.showPw }) }}),
			field(m.TextField{Outlined: true, Label: "Bio", MaxLength: 40, Helper: "Short and sweet"}),
			field(m.TextField{Outlined: true, Label: "Search", Prefix: icons.Search, Hint: "Type to search"}),
		}}),
		section(ctx, "Multiline", m.TextField{Label: "Notes", Multiline: true, MinLines: 3}),
		section(ctx, "Search bar", m.SearchBar{Hint: "Search messages", Trailing: []w.Widget{
			m.IconButton{Icon: icons.Mic, OnPressed: noop}, m.CircleAvatar{Text: "V", Radius: 16},
		}}),
	}}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------

func navPage(ctx w.BuildContext) w.Widget { return navDemo{} }

type navDemo struct{}

func (navDemo) CreateState() w.State { return &navDemoState{sortCol: 1} }

type navDemoState struct {
	w.StateBase
	tab, tab2, nav, step int
	sortCol              int
	sortAsc              bool
	selected             map[int]bool
}

func (s *navDemoState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	if s.selected == nil {
		s.selected = map[int]bool{1: true}
	}
	type fruit struct {
		name  string
		kcal  int
		price float64
	}
	data := []fruit{{"Apple", 52, 0.5}, {"Banana", 89, 0.25}, {"Cherry", 50, 3.2}, {"Durian", 147, 12}}
	// sort
	for i := range data {
		for j := i + 1; j < len(data); j++ {
			less := data[j].kcal < data[i].kcal
			if s.sortCol == 0 {
				less = data[j].name < data[i].name
			} else if s.sortCol == 2 {
				less = data[j].price < data[i].price
			}
			if less == s.sortAsc {
				data[i], data[j] = data[j], data[i]
			}
		}
	}
	rows := make([]m.DataRow, len(data))
	for i, f := range data {
		i := i
		rows[i] = m.DataRow{Cells: m.TextCells(f.name, fmt.Sprint(f.kcal), fmt.Sprintf("$%.2f", f.price)), Selected: s.selected[i],
			OnSelectChanged: func(v bool) { s.SetState(func() { s.selected[i] = v }) }}
	}
	sortBy := func(col int) func(bool) {
		return func(asc bool) { s.SetState(func() { s.sortCol, s.sortAsc = col, asc }) }
	}
	tabs := []m.Tab{{Text: "Flights", Icon: icons.Flight}, {Text: "Trips", Icon: icons.Luggage}, {Text: "Explore", Icon: icons.Explore}}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		section(ctx, "Tabs",
			m.TabBar{Tabs: tabs, Selected: s.tab, OnChanged: func(i int) { s.SetState(func() { s.tab = i }) }},
			m.TabBar{Secondary: true, Tabs: []m.Tab{{Text: "Overview"}, {Text: "Specifications"}, {Text: "Reviews"}}, Selected: s.tab2,
				OnChanged: func(i int) { s.SetState(func() { s.tab2 = i }) }},
			m.TabBarView{Selected: s.tab2, Children: []w.Widget{
				w.Text{Text: "Overview content"}, w.Text{Text: "Specifications content"}, w.Text{Text: "Reviews content"},
			}},
		),
		section(ctx, "Navigation bar", m.NavigationBar{Selected: s.nav, OnSelected: func(i int) { s.SetState(func() { s.nav = i }) },
			Destinations: []m.NavigationDestination{
				{Icon: icons.Home, Label: "Home"}, {Icon: icons.Search, Label: "Search"},
				{Icon: icons.Notifications, Label: "Alerts", Badge: "•"}, {Icon: icons.Person, Label: "Profile"},
			}}),
		section(ctx, "Bottom app bar", m.BottomAppBar{Actions: []w.Widget{
			m.IconButton{Icon: icons.CheckBox, OnPressed: noop}, m.IconButton{Icon: icons.Edit, OnPressed: noop},
			m.IconButton{Icon: icons.Mic, OnPressed: noop}, m.IconButton{Icon: icons.Image, OnPressed: noop},
		}, FAB: m.FloatingActionButton{Icon: icons.Add, Lowered: true, OnPressed: noop}}),
		section(ctx, "App bars",
			m.AppBar{Title: "Small", Leading: m.IconButton{Icon: icons.Menu, OnPressed: noop}, Actions: []w.Widget{m.IconButton{Icon: icons.AttachFile, OnPressed: noop}}},
			m.AppBar{Title: "Center-aligned", Variant: m.AppBarCenterAligned, Leading: m.IconButton{Icon: icons.Menu, OnPressed: noop}, Actions: []w.Widget{m.CircleAvatar{Text: "V", Radius: 16}}},
			m.AppBar{Title: "Medium", Variant: m.AppBarMedium, ScrolledUnder: true, Leading: m.IconButton{Icon: icons.ArrowBack, OnPressed: noop}},
		),
		section(ctx, "Data table", m.DataTable{SortColumn: s.sortCol, SortAscending: s.sortAsc, Rows: rows, Columns: []m.DataColumn{
			{Label: "Fruit", OnSort: sortBy(0)}, {Label: "Calories", Numeric: true, OnSort: sortBy(1)}, {Label: "Price", Numeric: true, OnSort: sortBy(2)},
		}}),
		section(ctx, "Stepper", m.Stepper{Current: s.step, OnStepTapped: func(i int) { s.SetState(func() { s.step = i }) },
			OnContinue: func() { s.SetState(func() { s.step = min(s.step+1, 2) }) },
			OnCancel:   func() { s.SetState(func() { s.step = max(s.step-1, 0) }) },
			Steps: []m.Step{
				{Title: "Account", Subtitle: "Create your login", State: stepState(s.step, 0), Content: m.TextField{Label: "Username"}},
				{Title: "Details", State: stepState(s.step, 1), Content: w.Text{Text: "Tell us about yourself.", Style: m.Styled(th.Text.BodyMedium, th.Scheme.OnSurfaceVariant)}},
				{Title: "Confirm", State: stepState(s.step, 2), Content: w.Text{Text: "All done!"}},
			}}),
	}}
}

func stepState(cur, i int) m.StepState {
	switch {
	case i < cur:
		return m.StepComplete
	case i == cur:
		return m.StepEditing
	}
	return m.StepIndexed
}

// ---------------------------------------------------------------------------

type carousels struct{}

func (carousels) CreateState() w.State { return &carouselsState{tapped: -1} }

type carouselsState struct {
	w.StateBase
	tapped int
}

func (s *carouselsState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	sc := th.Scheme
	names := []string{"Mountains", "Forest", "Desert", "Ocean", "City", "Tundra", "Canyon", "Lake"}
	ics := []*vector.Icon{icons.Landscape, icons.Forest, icons.WbSunny, icons.Waves, icons.LocationCity, icons.AcUnit, icons.Terrain, icons.Water}
	palette := []geom.Color{sc.PrimaryContainer, sc.SecondaryContainer, sc.TertiaryContainer, sc.SurfaceContainerHighest}
	items := func(big bool) []w.Widget {
		out := make([]w.Widget, len(names))
		for i, n := range names {
			st := m.Styled(th.Text.TitleMedium, sc.OnSurface)
			if big {
				st = m.Styled(th.Text.HeadlineSmall, sc.OnSurface)
			}
			out[i] = w.Center{Child: w.Column{ShrinkMain: true, Cross: w.CrossCenter, Spacing: 8, Children: []w.Widget{
				w.Icon{Icon: ics[i], Size: 40, Color: sc.OnSurfaceVariant},
				w.Text{Text: n, Style: st},
			}}}
		}
		return out
	}
	tap := func(i int) { s.SetState(func() { s.tapped = i }) }
	label := func(t string) w.Widget {
		return w.Text{Text: t, Style: m.Styled(th.Text.LabelLarge, sc.OnSurfaceVariant)}
	}
	status := "Tap an item"
	if s.tapped >= 0 {
		status = "Tapped: " + names[s.tapped]
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
		label("Multi-browse"),
		m.Carousel{Layout: m.CarouselMultiBrowse, Items: items(false), Colors: palette, OnTap: tap},
		label("Uncontained"),
		m.Carousel{Layout: m.CarouselUncontained, ItemExtent: 220, Height: 160, Items: items(false), Colors: palette, OnTap: tap},
		label("Hero · mouse wheel scrolls it, then the page"),
		m.Carousel{Layout: m.CarouselHero, Height: 220, Items: items(true), Colors: palette, OnTap: tap, WheelScroll: true},
		label("Full-screen"),
		m.Carousel{Layout: m.CarouselFullScreen, Height: 240, Items: items(true), Colors: palette, OnTap: tap},
		w.Text{Text: status, Style: m.Styled(th.Text.BodySmall, sc.OnSurfaceVariant)},
	}}
}

type sideSheets struct{}

func (sideSheets) CreateState() w.State { return &sideSheetsState{open: true} }

type sideSheetsState struct {
	w.StateBase
	open bool
}

func (s *sideSheetsState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	sc := th.Scheme
	filters := w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 8, Children: []w.Widget{
		w.Text{Text: "Show items that match these filters.", Style: m.Styled(th.Text.BodyMedium, sc.OnSurfaceVariant)},
		m.CheckboxListTile{Title: "In stock", Value: true, OnChanged: func(bool) {}},
		m.CheckboxListTile{Title: "Free shipping", Value: false, OnChanged: func(bool) {}},
	}}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 12, Children: []w.Widget{
		wrap(
			m.FilledTonalButton{Label: "Modal side sheet", Icon: icons.FilterList, OnPressed: func() {
				m.ShowModalSideSheet(ctx, m.SideSheet{Title: "Filters", OnBack: func() {}, Child: filters,
					Actions: []w.Widget{
						m.FilledButton{Label: "Apply", OnPressed: func() { m.ShowSnackBar(ctx, m.SnackBar{Message: "Filters applied"}) }},
						m.OutlinedButton{Label: "Reset", OnPressed: func() {}},
					}}, nil)
			}},
			m.OutlinedButton{Label: "Toggle standard sheet", OnPressed: func() { s.SetState(func() { s.open = !s.open }) }},
		),
		// Standard sheet: pushes the main content aside inside its row.
		w.Container{Height: 280, Border: &geom.Border{Radius: 12, Color: sc.OutlineVariant, Width: 1}, Child: w.ClipRect{Child: w.Row{Cross: w.CrossStretch, Children: []w.Widget{
			w.Expanded{Child: w.Container{Color: sc.SurfaceContainerLowest, Alignment: w.Ptr(geom.Center),
				Child: w.Text{Text: "Main content", Style: m.Styled(th.Text.TitleMedium, sc.OnSurfaceVariant)}}},
			m.SideSheet{Title: "Details", Open: s.open, Width: 300, OnClose: func() { s.SetState(func() { s.open = false }) },
				Child: w.Text{Text: "A standard side sheet shows secondary content next to the main view. Toggle it to see the content resize.",
					Style: m.Styled(th.Text.BodyMedium, sc.OnSurfaceVariant)}},
		}}}},
	}}
}

// ---------------------------------------------------------------------------

type splits struct{}

func (splits) CreateState() w.State { return &splitsState{} }

type splitsState struct {
	w.StateBase
	sizes []float32
}

func (s *splitsState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	sc := th.Scheme
	pane := func(i int, title string, lo, hi float32) w.Widget {
		size := ""
		if i < len(s.sizes) {
			size = fmt.Sprintf("%.0f px", s.sizes[i])
		}
		limits := fmt.Sprintf("min %.0f", lo)
		if hi > 0 {
			limits += fmt.Sprintf(" · max %.0f", hi)
		}
		return m.Card{Variant: m.CardFilled, Padding: geom.Insets(16), Child: w.Column{Cross: w.CrossStart, Spacing: 4, Children: []w.Widget{
			w.Text{Text: title, Style: m.Styled(th.Text.TitleMedium, sc.OnSurface), MaxLines: 1, Ellipsis: true},
			w.Text{Text: limits, Style: m.Styled(th.Text.BodySmall, sc.OnSurfaceVariant), MaxLines: 1},
			w.Text{Text: size, Style: m.Styled(th.Text.LabelLarge, sc.Primary)},
		}}}
	}
	plainPane := func(label string, c geom.Color) w.Widget {
		return w.Container{Color: c, Alignment: w.Ptr(geom.Center),
			Child: w.Text{Text: label, Style: m.Styled(th.Text.BodyMedium, sc.OnSurfaceVariant)}}
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		w.Text{Text: "Drag the handles. Each pane stops at its min/max; the change then carries on to the next pane.",
			Style: m.Styled(th.Text.BodyMedium, sc.OnSurfaceVariant)},
		w.SizedBox{Height: 180, Child: m.SplitView{
			OnResize: func(v []float32) { s.SetState(func() { s.sizes = v }) },
			Panes: []w.Pane{
				{Min: 120, Max: 360, Size: 220, Child: pane(0, "Navigation", 120, 360)},
				{Min: 200, Child: pane(1, "Content", 200, 0)},
				{Min: 140, Max: 320, Size: 240, Child: pane(2, "Details", 140, 320)},
			}}},
		w.Container{Height: 220, Border: &geom.Border{Radius: 12, Color: sc.OutlineVariant, Width: 1}, Child: w.ClipRect{Child: w.SplitView{
			Vertical: true, DividerColor: sc.OutlineVariant, ActiveColor: sc.Primary,
			Panes: []w.Pane{
				{Min: 60, Child: plainPane("Editor (widgets.SplitView, vertical)", sc.SurfaceContainerLowest)},
				{Min: 40, Max: 140, Size: 80, Child: plainPane("Terminal · min 40 · max 140", sc.SurfaceContainer)},
			}}}},
	}}
}

// ---------------------------------------------------------------------------

// fileBrowser: a FileTree of the current folder next to a preview of the
// selected file.
type fileBrowser struct{}

func (fileBrowser) CreateState() w.State {
	return &fileBrowserState{fsys: os.DirFS("."), ctrl: w.NewTreeController()}
}

type fileBrowserState struct {
	w.StateBase
	fsys    fs.FS
	ctrl    *w.TreeController
	path    string
	preview string
}

func (s *fileBrowserState) show(p string, d fs.DirEntry) {
	s.SetState(func() {
		s.path = p
		switch {
		case d == nil || d.IsDir():
			s.preview = ""
		default:
			s.preview = previewFile(s.fsys, p)
		}
	})
}

// previewFile returns the first lines of a small text file.
func previewFile(fsys fs.FS, p string) string {
	if st, err := fs.Stat(fsys, p); err != nil || st.Size() > 256<<10 {
		return "(too large to preview)"
	}
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return err.Error()
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return "(binary file)"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 40 {
		lines = append(lines[:40], "…")
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\t", "    ")
}

func (s *fileBrowserState) Build(ctx w.BuildContext) w.Widget {
	th := m.ThemeOf(ctx)
	sc := th.Scheme
	title := "Select a file"
	if s.path != "" {
		title = s.path
	}
	preview := w.Column{Cross: w.CrossStretch, Spacing: 8, Children: []w.Widget{
		w.Row{Spacing: 8, Children: []w.Widget{
			w.Expanded{Child: w.Text{Text: title, Style: m.Styled(th.Text.TitleSmall, sc.OnSurface), MaxLines: 1, Ellipsis: true}},
			m.TextButton{Label: "Collapse all", OnPressed: s.ctrl.CollapseAll},
			m.TextButton{Label: "Reload", OnPressed: func() { s.ctrl.Reload("") }},
		}},
		w.Expanded{Child: w.ScrollView{Child: w.Text{Text: s.preview, Style: m.Styled(th.Text.BodySmall, sc.OnSurfaceVariant)}}},
	}}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 16, Children: []w.Widget{
		w.Text{Text: "The folder the gallery runs in. Click folders to expand; arrows, Home/End and Enter work after clicking. Double-click a file to open it.",
			Style: m.Styled(th.Text.BodyMedium, sc.OnSurfaceVariant)},
		w.SizedBox{Height: 360, Child: m.SplitView{Panes: []w.Pane{
			{Min: 180, Max: 420, Size: 280, Child: m.Card{Variant: m.CardOutlined, Padding: geom.InsetsHV(0, 8),
				Child: m.FileTree{FS: s.fsys, Controller: s.ctrl, ShowRoot: true, RootLabel: "nectar_ui", ShowGuides: true,
					OnSelect: s.show,
					OnOpen:   func(p string) { m.ShowSnackBar(ctx, m.SnackBar{Message: "Open " + p}) },
				}}},
			{Min: 240, Child: m.Card{Variant: m.CardFilled, Padding: geom.Insets(16), Child: preview}},
		}}},
	}}
}
