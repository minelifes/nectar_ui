package material

import (
	"fmt"
	"strconv"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// CalendarDatePicker is an inline month grid.
type CalendarDatePicker struct {
	Selected    time.Time
	First, Last time.Time // zero = unbounded
	Today       time.Time // zero = time.Now()
	OnChanged   func(time.Time)
}

func (CalendarDatePicker) CreateState() w.State { return &calendarState{} }

type calendarState struct {
	w.StateBase
	month time.Time // first day of the displayed month
}

func (s *calendarState) InitState() {
	sel := w.WidgetOf[CalendarDatePicker](s).Selected
	if sel.IsZero() {
		sel = time.Now()
	}
	s.month = time.Date(sel.Year(), sel.Month(), 1, 0, 0, 0, 0, time.Local)
}

func (s *calendarState) Build(ctx w.BuildContext) w.Widget {
	cp := w.WidgetOf[CalendarDatePicker](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	today := cp.Today
	if today.IsZero() {
		today = time.Now()
	}
	inRange := func(d time.Time) bool {
		d = dateOnly(d)
		return (cp.First.IsZero() || !d.Before(dateOnly(cp.First))) && (cp.Last.IsZero() || !d.After(dateOnly(cp.Last)))
	}
	prev := s.month.AddDate(0, -1, 0)
	next := s.month.AddDate(0, 1, 0)
	canPrev := cp.First.IsZero() || !prev.AddDate(0, 1, -1).Before(dateOnly(cp.First))
	canNext := cp.Last.IsZero() || !next.After(dateOnly(cp.Last))
	var onPrev, onNext func()
	if canPrev {
		onPrev = func() { s.SetState(func() { s.month = prev }) }
	}
	if canNext {
		onNext = func() { s.SetState(func() { s.month = next }) }
	}
	header := w.Row{Cross: w.CrossCenter, Children: []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(12, 0, 0, 0), Child: w.Text{Text: s.month.Format("January 2006"), Style: Styled(th.Text.TitleSmall, sc.OnSurfaceVariant)}},
		w.Spacer{},
		IconButton{Icon: iconChevronLeft, OnPressed: onPrev},
		IconButton{Icon: iconChevronRight, OnPressed: onNext},
	}}
	cell := func(child w.Widget) w.Widget {
		return w.Expanded{Child: w.SizedBox{Height: 48, Child: w.Center{Child: child}}}
	}
	week := []w.Widget{}
	for _, d := range []string{"S", "M", "T", "W", "T", "F", "S"} {
		week = append(week, cell(w.Text{Text: d, Style: Styled(th.Text.BodySmall, sc.OnSurface)}))
	}
	rows := []w.Widget{header, w.Row{Children: week}}
	lead := int(s.month.Weekday())
	days := s.month.AddDate(0, 1, -1).Day()
	var row []w.Widget
	for i := 0; i < lead; i++ {
		row = append(row, cell(w.SizedBox{}))
	}
	for d := 1; d <= days; d++ {
		day := time.Date(s.month.Year(), s.month.Month(), d, 0, 0, 0, 0, time.Local)
		sel := !cp.Selected.IsZero() && sameDay(day, cp.Selected)
		isToday := sameDay(day, today)
		ok := inRange(day)
		fg := sc.OnSurface
		var bg, border geom.Color
		var bw float32
		switch {
		case sel:
			bg, fg = sc.Primary, sc.OnPrimary
		case isToday:
			fg, border, bw = sc.Primary, sc.Primary, 1
		}
		if !ok {
			fg, _ = disabledColors(sc)
		}
		var tap func()
		if ok && cp.OnChanged != nil {
			tap = func() { cp.OnChanged(day) }
		}
		row = append(row, cell(InkSurface{OnTap: tap, Color: bg, ContentColor: fg, BorderColor: border, BorderWidth: bw, Radius: CornerFull, NoFocus: true,
			Child: w.SizedBox{Width: 40, Height: 40, Child: w.Center{Child: w.Text{Text: strconv.Itoa(d), Style: Styled(th.Text.BodyLarge, fg)}}}}))
		if len(row) == 7 {
			rows = append(rows, w.Row{Children: row})
			row = nil
		}
	}
	if len(row) > 0 {
		for len(row) < 7 {
			row = append(row, cell(w.SizedBox{}))
		}
		rows = append(rows, w.Row{Children: row})
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: rows}
}

// DatePickerOptions configure ShowDatePicker.
type DatePickerOptions struct {
	Initial     time.Time
	First, Last time.Time
	Title       string // default "Select date"
	OnPicked    func(time.Time)
}

// ShowDatePicker opens the M3 date picker dialog.
func ShowDatePicker(ctx w.BuildContext, opts DatePickerOptions) {
	ShowDialog(ctx, func(ctx w.BuildContext) w.Widget { return datePickerDialog{opts: opts} }, nil)
}

type datePickerDialog struct{ opts DatePickerOptions }

func (datePickerDialog) CreateState() w.State { return &datePickerState{} }

type datePickerState struct {
	w.StateBase
	sel time.Time
}

func (s *datePickerState) InitState() {
	s.sel = w.WidgetOf[datePickerDialog](s).opts.Initial
	if s.sel.IsZero() {
		s.sel = time.Now()
	}
}

func (s *datePickerState) Build(ctx w.BuildContext) w.Widget {
	o := w.WidgetOf[datePickerDialog](s).opts
	th := ThemeOf(ctx)
	sc := th.Scheme
	title := o.Title
	if title == "" {
		title = "Select date"
	}
	pad := geom.InsetsLTRB(12, 16, 12, 12)
	return w.SizedBox{Width: 360, Child: Dialog{Padding: &pad, Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: []w.Widget{
		w.Padding{Padding: geom.InsetsLTRB(12, 0, 12, 0), Child: w.Text{Text: title, Style: Styled(th.Text.LabelLarge, sc.OnSurfaceVariant)}},
		w.Padding{Padding: geom.InsetsLTRB(12, 20, 12, 12), Child: w.Text{Text: s.sel.Format("Mon, Jan 2"), Style: Styled(th.Text.HeadlineLarge, sc.OnSurface)}},
		Divider{},
		w.SizedBox{Height: 8},
		CalendarDatePicker{Selected: s.sel, First: o.First, Last: o.Last, OnChanged: func(t time.Time) { s.SetState(func() { s.sel = t }) }},
		w.Row{Main: w.MainEnd, Spacing: 8, Children: []w.Widget{
			TextButton{Label: "Cancel", OnPressed: func() { Pop(ctx, nil) }},
			TextButton{Label: "OK", OnPressed: func() {
				Pop(ctx, s.sel)
				if o.OnPicked != nil {
					o.OnPicked(s.sel)
				}
			}},
		}},
	}}}}
}

// ShowTimePicker opens a time picker (input mode, 24-hour).
func ShowTimePicker(ctx w.BuildContext, hour, minute int, onPicked func(hour, minute int)) {
	ShowDialog(ctx, func(ctx w.BuildContext) w.Widget { return timePickerDialog{h: hour, m: minute, onPicked: onPicked} }, nil)
}

type timePickerDialog struct {
	h, m     int
	onPicked func(int, int)
}

func (timePickerDialog) CreateState() w.State { return &timePickerState{} }

type timePickerState struct {
	w.StateBase
	hc, mc *w.TextEditingController
	err    bool
}

func (s *timePickerState) InitState() {
	d := w.WidgetOf[timePickerDialog](s)
	s.hc = w.NewTextController(fmt.Sprintf("%02d", d.h))
	s.mc = w.NewTextController(fmt.Sprintf("%02d", d.m))
}

func (s *timePickerState) Build(ctx w.BuildContext) w.Widget {
	d := w.WidgetOf[timePickerDialog](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	box := func(c *w.TextEditingController, label string) w.Widget {
		return w.Column{Cross: w.CrossStart, ShrinkMain: true, Spacing: 6, Children: []w.Widget{
			w.Container{Width: 96, Height: 72, Color: sc.SurfaceContainerHighest, Border: &geom.Border{Radius: CornerSmall},
				Alignment: w.Ptr(geom.Center), Padding: geom.InsetsHV(16, 0),
				Child: w.EditableText{Controller: c, Style: Styled(th.Text.DisplayMedium, sc.OnSurface), CursorColor: sc.Primary}},
			w.Text{Text: label, Style: Styled(th.Text.BodySmall, sc.OnSurfaceVariant)},
		}}
	}
	parse := func() (int, int, bool) {
		h, e1 := strconv.Atoi(s.hc.Text())
		m, e2 := strconv.Atoi(s.mc.Text())
		return h, m, e1 == nil && e2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60
	}
	kids := []w.Widget{
		w.Text{Text: "Enter time", Style: Styled(th.Text.LabelLarge, sc.OnSurfaceVariant)},
		w.SizedBox{Height: 20},
		w.Row{ShrinkMain: true, Cross: w.CrossStart, Spacing: 8, Children: []w.Widget{
			box(s.hc, "Hour"),
			w.SizedBox{Height: 72, Child: w.Center{Child: w.Text{Text: ":", Style: Styled(th.Text.DisplayLarge, sc.OnSurface)}}},
			box(s.mc, "Minute"),
		}},
	}
	if s.err {
		kids = append(kids, w.SizedBox{Height: 8}, w.Text{Text: "Enter a valid time", Style: Styled(th.Text.BodySmall, sc.Error)})
	}
	kids = append(kids, w.SizedBox{Height: 24}, w.Row{Main: w.MainEnd, Spacing: 8, Children: []w.Widget{
		TextButton{Label: "Cancel", OnPressed: func() { Pop(ctx, nil) }},
		TextButton{Label: "OK", OnPressed: func() {
			h, m, ok := parse()
			if !ok {
				s.SetState(func() { s.err = true })
				return
			}
			Pop(ctx, [2]int{h, m})
			if d.onPicked != nil {
				d.onPicked(h, m)
			}
		}},
	}})
	return Dialog{Child: w.Column{Cross: w.CrossStart, ShrinkMain: true, Children: kids}}
}
