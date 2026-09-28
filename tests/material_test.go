package tests

import (
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// host puts a widget in a Material app with a mutable model.
type host struct {
	build func(ctx w.BuildContext, set func(func())) w.Widget
}

func (host) CreateState() w.State { return &hostState{} }

type hostState struct{ w.StateBase }

func (s *hostState) Build(w.BuildContext) w.Widget {
	h := w.WidgetOf[host](s)
	return m.App{Home: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		return w.Padding{Padding: geom.Insets(20), Child: w.Align{Alignment: geom.TopLeft,
			Child: h.build(ctx, func(f func()) { s.SetState(f) })}}
	}}}
}

func run(b func(ctx w.BuildContext, set func(func())) w.Widget) *tester.Tester {
	t := tester.New(host{build: b}, 800, 600)
	t.Settle()
	return t
}

func TestSelectionControls(t *testing.T) {
	var check, sw bool
	radio := 0
	tt := run(func(_ w.BuildContext, set func(func())) w.Widget {
		return w.Column{ShrinkMain: true, Cross: w.CrossStart, Children: []w.Widget{
			m.Checkbox{Value: check, OnChanged: func(v bool) { set(func() { check = v }) }},                 // y 20..60
			m.Switch{Value: sw, OnChanged: func(v bool) { set(func() { sw = v }) }},                         // y 60..100
			m.Radio[int]{Value: 2, GroupValue: radio, OnChanged: func(v int) { set(func() { radio = v }) }}, // y 100..140
			m.Checkbox{Value: false}, // disabled, y 140..180
		}}
	})
	tt.Tap(40, 40)
	tt.Tap(46, 80)
	tt.Tap(40, 120)
	tt.Tap(40, 160)
	tt.Settle()
	if !check || !sw || radio != 2 {
		t.Fatalf("check=%v switch=%v radio=%v", check, sw, radio)
	}
	// Dragging the switch thumb back off.
	tt.Drag(geom.Pt(60, 80), geom.Pt(20, 80))
	tt.Settle()
	if sw {
		t.Fatal("drag did not turn the switch off")
	}
}

func TestSliderTapAndKeys(t *testing.T) {
	var v float32 = 0
	tt := run(func(_ w.BuildContext, set func(func())) w.Widget {
		return w.SizedBox{Width: 440, Child: m.Slider{Value: v, Max: 100, Divisions: 10, OnChanged: func(x float32) { set(func() { v = x }) }}}
	})
	// Track spans x = 20+20 .. 20+440-20 → 40..440; 75% ≈ x 340.
	tt.Tap(340, 42)
	if v != 80 && v != 70 {
		t.Fatalf("tap set %v, want ~75 snapped", v)
	}
	tt.Key(w.KeyTab) // focus the slider (the only focusable widget)
	before := v
	tt.Key(w.KeyRight)
	if v != before+10 {
		t.Fatalf("arrow key: %v -> %v", before, v)
	}
}

func TestTextFieldTypingAndCounter(t *testing.T) {
	ctrl := w.NewTextController("")
	var changed string
	tt := run(func(_ w.BuildContext, _ func(func())) w.Widget {
		return w.SizedBox{Width: 300, Child: m.TextField{Controller: ctrl, Label: "Name", MaxLength: 5, OnChanged: func(s string) { changed = s }}}
	})
	tt.Tap(100, 48)
	tt.Type("Vadym!!!")
	tt.Settle()
	if ctrl.Text() != "Vadym" || changed != "Vadym" {
		t.Fatalf("text %q changed %q", ctrl.Text(), changed)
	}
	if _, ok := tt.Find("5/5"); !ok {
		t.Fatalf("counter missing: %v", tt.Texts())
	}
}

func TestDialogResultAndMenus(t *testing.T) {
	var result any
	var picked string
	var fruit any = "a"
	tt := run(func(ctx w.BuildContext, set func(func())) w.Widget {
		return w.Row{ShrinkMain: true, Spacing: 8, Children: []w.Widget{
			m.FilledButton{Label: "Open", OnPressed: func() {
				m.ShowDialog(ctx, func(dctx w.BuildContext) w.Widget {
					return m.AlertDialog{Title: "Sure?", Actions: []w.Widget{
						m.TextButton{Label: "No", OnPressed: func() { m.Pop(dctx, false) }},
						m.TextButton{Label: "Yes", OnPressed: func() { m.Pop(dctx, true) }},
					}}
				}, func(r any) { result = r })
			}},
			m.PopupMenuButton{Items: []m.MenuItem{{Label: "Copy", OnTap: func() { picked = "copy" }}, {Label: "Gone", Disabled: true}}},
			m.DropdownMenu{Label: "Fruit", Selected: fruit, Entries: []m.DropdownEntry{{Label: "Apple", Value: "a"}, {Label: "Kiwi", Value: "k"}},
				OnSelected: func(v any) { set(func() { fruit = v }) }},
		}}
	})
	tt.TapText("Open")
	tt.Settle()
	tt.TapText("Yes")
	tt.Settle()
	if result != true {
		t.Fatalf("dialog result %v", result)
	}
	if _, ok := tt.Find("Sure?"); ok {
		t.Fatal("dialog still open")
	}
	// Popup menu: disabled items do nothing, enabled ones fire and close.
	r, _ := tt.Find("Open")
	tt.Tap(r.Right()+40, r.Y+r.H/2)
	tt.Settle()
	tt.TapText("Gone")
	if picked != "" {
		t.Fatal("disabled item fired")
	}
	tt.TapText("Copy")
	tt.Settle()
	if picked != "copy" {
		t.Fatalf("menu pick %q", picked)
	}
	if _, ok := tt.Find("Copy"); ok {
		t.Fatal("menu did not close")
	}
	tt.TapText("Apple")
	tt.Settle()
	tt.TapText("Kiwi")
	tt.Settle()
	if fruit != "k" {
		t.Fatalf("dropdown %v", fruit)
	}
}

func TestSnackbarActionAndAutoDismiss(t *testing.T) {
	undone := false
	tt := run(func(ctx w.BuildContext, _ func(func())) w.Widget {
		return m.TextButton{Label: "Delete", OnPressed: func() {
			m.ShowSnackBar(ctx, m.SnackBar{Message: "Deleted", ActionLabel: "Undo", Duration: time.Second, OnAction: func() { undone = true }})
		}}
	})
	tt.TapText("Delete")
	tt.Settle()
	tt.TapText("Undo")
	tt.Settle()
	if !undone {
		t.Fatal("action not called")
	}
	if _, ok := tt.Find("Deleted"); ok {
		t.Fatal("snackbar should hide after its action")
	}
}

func TestTabsSegmentsNavAndChips(t *testing.T) {
	tab, nav := 0, 0
	seg := []any{"a"}
	var filter bool
	tt := run(func(_ w.BuildContext, set func(func())) w.Widget {
		return w.SizedBox{Width: 600, Child: w.Column{ShrinkMain: true, Cross: w.CrossStretch, Spacing: 8, Children: []w.Widget{
			m.TabBar{Tabs: []m.Tab{{Text: "One"}, {Text: "Two"}}, Selected: tab, OnChanged: func(i int) { set(func() { tab = i }) }},
			m.SegmentedButton{Segments: []m.Segment{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}}, Selected: seg,
				OnChanged: func(v []any) { set(func() { seg = v }) }},
			m.NavigationBar{Selected: nav, OnSelected: func(i int) { set(func() { nav = i }) }, Destinations: []m.NavigationDestination{
				{Icon: icons.Home, Label: "Home"}, {Icon: icons.Search, Label: "Find"}}},
			w.Row{ShrinkMain: true, Children: []w.Widget{m.FilterChip{Label: "Dogs", Selected: filter, OnSelected: func(v bool) { set(func() { filter = v }) }}}},
		}}}
	})
	tt.TapText("Two")
	tt.TapText("Beta")
	tt.TapText("Find")
	tt.TapText("Dogs")
	tt.Settle()
	if tab != 1 || len(seg) != 1 || seg[0] != "b" || nav != 1 || !filter {
		t.Fatalf("tab=%d seg=%v nav=%d filter=%v", tab, seg, nav, filter)
	}
}

func TestScaffoldDrawerAndPush(t *testing.T) {
	tt := tester.New(m.App{Home: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		return m.Scaffold{
			AppBar: m.AppBar{Title: "Home", Leading: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
				return m.IconButton{Icon: icons.Menu, OnPressed: func() { m.ScaffoldOf(ctx).OpenDrawer() }}
			}}},
			Drawer: m.NavigationDrawer{Destinations: []m.NavigationDestination{{Icon: icons.Inbox, Label: "Inbox"}}},
			Body: m.FilledButton{Label: "Details", OnPressed: func() {
				m.Push(ctx, func(ctx w.BuildContext) w.Widget {
					return w.Column{Children: []w.Widget{m.AppBar{Title: "Details page"}}}
				})
			}},
		}
	}}}, 800, 600)
	tt.Settle()
	tt.Tap(28, 32) // menu button
	tt.Settle()
	if _, ok := tt.Find("Inbox"); !ok {
		t.Fatal("drawer not open")
	}
	tt.Key(w.KeyEscape)
	tt.Settle()
	if _, ok := tt.Find("Inbox"); ok {
		t.Fatal("escape did not close the drawer")
	}
	tt.TapText("Details")
	tt.Settle()
	if _, ok := tt.Find("Details page"); !ok {
		t.Fatal("page not pushed")
	}
	tt.Tap(28, 32) // auto back button
	tt.Settle()
	if _, ok := tt.Find("Details page"); ok {
		t.Fatal("back button did not pop")
	}
}

func TestDatePickerAndStepper(t *testing.T) {
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	step := 0
	tt := run(func(ctx w.BuildContext, set func(func())) w.Widget {
		return w.Column{ShrinkMain: true, Cross: w.CrossStart, Children: []w.Widget{
			m.TextButton{Label: "Pick", OnPressed: func() {
				m.ShowDatePicker(ctx, m.DatePickerOptions{Initial: day, OnPicked: func(d time.Time) { set(func() { day = d }) }})
			}},
			w.SizedBox{Width: 400, Child: m.Stepper{Current: step, OnContinue: func() { set(func() { step++ }) },
				Steps: []m.Step{{Title: "A", Content: w.Text{Text: "a body"}}, {Title: "B"}}}},
		}}
	})
	tt.TapText("Pick")
	tt.Settle()
	if _, ok := tt.Find("March 2026"); !ok {
		t.Fatalf("picker not showing the initial month: %v", tt.Texts())
	}
	tt.TapText("21")
	tt.TapText("OK")
	tt.Settle()
	if day.Day() != 21 || day.Month() != time.March {
		t.Fatalf("picked %v", day)
	}
	tt.TapText("Continue")
	tt.Settle()
	if step != 1 {
		t.Fatalf("stepper step %d", step)
	}
}

func TestCarouselTapDragSnapKeys(t *testing.T) {
	tapped := -1
	items := make([]w.Widget, 6)
	for i := range items {
		items[i] = w.Text{Text: string(rune('A' + i))}
	}
	tt := run(func(_ w.BuildContext, _ func(func())) w.Widget {
		// 760px wide hero carousel: large item ≈ 696px, unit ≈ 704px.
		return w.SizedBox{Width: 760, Child: m.Carousel{Layout: m.CarouselHero, Height: 200, Items: items, OnTap: func(i int) { tapped = i }}}
	})
	tt.Tap(300, 120)
	if tapped != 0 {
		t.Fatalf("tap on first item gave %d", tapped)
	}
	tt.Tap(750, 120) // the peeking small item
	if tapped != 1 {
		t.Fatalf("tap on peek gave %d", tapped)
	}
	// Drag 60% of an item: snaps forward to item 1.
	tt.Drag(geom.Pt(600, 120), geom.Pt(180, 120))
	tt.Settle()
	tt.Tap(300, 120)
	if tapped != 1 {
		t.Fatalf("after drag+snap the large item is %d, want 1", tapped)
	}
	// Arrow keys move one item (the carousel got focus from the tap).
	tt.Key(w.KeyRight)
	tt.Settle()
	tt.Tap(300, 120)
	if tapped != 2 {
		t.Fatalf("after →, large item is %d, want 2", tapped)
	}
	// A vertical wheel doesn't move it (the page gets it instead).
	tt.Scroll(300, 120, 500)
	tt.Settle()
	tt.Tap(300, 120)
	if tapped != 2 {
		t.Fatalf("vertical wheel moved the carousel to %d", tapped)
	}
}

func TestSideSheets(t *testing.T) {
	open := true
	closed := false
	tt := run(func(ctx w.BuildContext, set func(func())) w.Widget {
		return w.SizedBox{Width: 760, Height: 400, Child: w.Row{Cross: w.CrossStretch, Children: []w.Widget{
			w.Expanded{Child: m.TextButton{Label: "Modal", OnPressed: func() {
				m.ShowModalSideSheet(ctx, m.SideSheet{Title: "Filters", Child: w.Text{Text: "modal body"}}, func(any) { closed = true })
			}}},
			m.SideSheet{Title: "Details", Open: open, OnClose: func() { set(func() { open = false }) }, Child: w.Text{Text: "std body"}},
		}}}
	})
	if _, ok := tt.Find("std body"); !ok {
		t.Fatal("standard sheet not shown")
	}
	r, _ := tt.Find("Details")
	tt.Tap(780-12-20, r.Y+r.H/2) // close button: 12px in from the sheet's right edge (x=780)
	tt.Settle()
	if _, ok := tt.Find("std body"); ok || open {
		t.Fatal("standard sheet did not close")
	}
	tt.TapText("Modal")
	tt.Settle()
	if _, ok := tt.Find("modal body"); !ok {
		t.Fatal("modal sheet not shown")
	}
	f, _ := tt.Find("Filters")
	tt.Tap(800-12-20, f.Y+f.H/2) // its close button, at the window's right edge
	tt.Settle()
	if _, ok := tt.Find("modal body"); ok || !closed {
		t.Fatal("modal sheet did not close via its close button")
	}
	tt.TapText("Modal")
	tt.Settle()
	tt.Key(w.KeyEscape)
	tt.Settle()
	if _, ok := tt.Find("modal body"); ok {
		t.Fatal("escape did not close the modal sheet")
	}
}

func TestCarouselWheelScrollCapturesThenReleases(t *testing.T) {
	page := w.NewScrollController()
	tapped := -1
	items := []w.Widget{w.Text{Text: "A"}, w.Text{Text: "B"}, w.Text{Text: "C"}}
	tt := run(func(_ w.BuildContext, _ func(func())) w.Widget {
		return w.SizedBox{Width: 760, Height: 560, Child: w.ListView{Controller: page, Children: []w.Widget{
			w.SizedBox{Height: 300},
			// Hero: one wheel "item" = its unit (≈704px); 3 items → positions 0..2.
			m.Carousel{Layout: m.CarouselHero, Height: 200, Items: items, WheelScroll: true, OnTap: func(i int) { tapped = i }},
			w.SizedBox{Height: 2000},
		}}}
	})
	large := func() int { // index of the large item: tap its middle
		tt.Tap(300, float32(20+300+100)-page.Offset())
		return tapped
	}
	over := func() float32 { return float32(20+300+100) - page.Offset() } // carousel's middle row
	unit := float32(760 - 56 - 8 + 8)

	// Wheel down over the carousel: it scrolls, the page doesn't.
	tt.Scroll(300, over(), unit)
	tt.Settle()
	if page.Offset() != 0 || large() != 1 {
		t.Fatalf("step 1: page %v, large item %d", page.Offset(), tapped)
	}
	tt.Scroll(300, over(), unit)
	tt.Settle()
	if page.Offset() != 0 || large() != 2 {
		t.Fatalf("step 2: page %v, large item %d", page.Offset(), tapped)
	}
	// At the last item the wheel is released: the page scrolls.
	tt.Scroll(300, over(), 100)
	tt.Settle()
	if page.Offset() != 100 || large() != 2 {
		t.Fatalf("at the end: page %v, large item %d", page.Offset(), tapped)
	}
	// Scrolling up over it moves the carousel back first, page stays.
	tt.Scroll(300, over(), -unit)
	tt.Scroll(300, over(), -unit)
	tt.Settle()
	if page.Offset() != 100 || large() != 0 {
		t.Fatalf("scrolling up: page %v, large item %d", page.Offset(), tapped)
	}
	// At the first item it releases again: the page scrolls up.
	tt.Scroll(300, over(), -100)
	tt.Settle()
	if page.Offset() != 0 {
		t.Fatalf("at the start: page %v", page.Offset())
	}
	// Horizontal scrolling moves it the same way.
	tt.ScrollXY(300, over(), unit, 0)
	tt.Settle()
	if large() != 1 || page.Offset() != 0 {
		t.Fatalf("horizontal: page %v, large item %d", page.Offset(), tapped)
	}
}

func TestCarouselWheelNotchPagesByOneItem(t *testing.T) {
	tapped := -1
	items := []w.Widget{w.Text{Text: "A"}, w.Text{Text: "B"}, w.Text{Text: "C"}}
	tt := run(func(_ w.BuildContext, _ func(func())) w.Widget {
		return w.SizedBox{Width: 760, Child: m.Carousel{Layout: m.CarouselHero, Height: 200, Items: items, WheelScroll: true,
			OnTap: func(i int) { tapped = i }}}
	})
	tt.Scroll(300, 120, 40) // one mouse-wheel notch
	tt.Settle()
	tt.Tap(300, 120)
	if tapped != 1 {
		t.Fatalf("one notch down: large item %d, want 1", tapped)
	}
	tt.Scroll(300, 120, -40)
	tt.Settle()
	tt.Tap(300, 120)
	if tapped != 0 {
		t.Fatalf("one notch up: large item %d, want 0", tapped)
	}
}
