package material

import (
	"strconv"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// ---------------------------------------------------------------------------
// DataTable

// DataColumn describes a table column.
type DataColumn struct {
	Label   string
	Numeric bool    // right-aligned
	Width   float32 // 0 = share remaining width
	// OnSort makes the header tappable; called with the new direction.
	OnSort func(ascending bool)
}

// DataRow is one table row.
type DataRow struct {
	Cells    []w.Widget // or use Text cells via Texts
	Selected bool
	// OnSelectChanged adds a leading checkbox to every row.
	OnSelectChanged func(bool)
}

// TextCells makes a row of plain text cells.
func TextCells(values ...string) []w.Widget {
	out := make([]w.Widget, len(values))
	for i, v := range values {
		out[i] = w.Text{Text: v, MaxLines: 1, Ellipsis: true}
	}
	return out
}

// DataTable shows rows of data with a header.
type DataTable struct {
	Columns       []DataColumn
	Rows          []DataRow
	SortColumn    int // -1 = none
	SortAscending bool
	// Style overrides Theme.DataTable.
	Style DataTableTheme
}

func (t DataTable) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.DataTable, t.Style)
	margin := pickF(st.HorizontalMargin, 16)
	divider := Divider{Style: DividerTheme{Color: st.DividerColor}}
	checkboxes := false
	for _, r := range t.Rows {
		if r.OnSelectChanged != nil {
			checkboxes = true
		}
	}
	cell := func(i int, child w.Widget) w.Widget {
		col := t.Columns[i]
		al := geom.CenterLeft
		if col.Numeric {
			al = geom.CenterRight
		}
		var c w.Widget = w.Padding{Padding: geom.InsetsHV(margin, 0), Child: w.Align{Alignment: al, Child: child}}
		if col.Width > 0 {
			return w.SizedBox{Width: col.Width, Child: c}
		}
		return w.Expanded{Child: c}
	}
	header := []w.Widget{}
	if checkboxes {
		all := len(t.Rows) > 0
		for _, r := range t.Rows {
			all = all && r.Selected
		}
		header = append(header, w.SizedBox{Width: 56, Child: w.Center{Child: Checkbox{Value: all, OnChanged: func(v bool) {
			for _, r := range t.Rows {
				if r.OnSelectChanged != nil && r.Selected != v {
					r.OnSelectChanged(v)
				}
			}
		}}}})
	}
	for i, c := range t.Columns {
		hs := pickTC(st.HeadingTextStyle, th.Text.TitleSmall, sc.OnSurfaceVariant)
		fg := hs.Color
		kids := []w.Widget{w.Text{Text: c.Label, Style: hs, MaxLines: 1}}
		sorted := i == t.SortColumn
		if sorted {
			arrow := iconArrowUp
			if !t.SortAscending {
				arrow = iconArrowDown
			}
			kids = append(kids, w.Icon{Icon: arrow, Size: 18, Color: pick(st.SortIconColor, sc.OnSurface)})
		}
		var label w.Widget = w.Row{ShrinkMain: true, Cross: w.CrossCenter, Spacing: 4, Children: kids}
		if c.OnSort != nil {
			c := c
			asc := true
			if sorted {
				asc = !t.SortAscending
			}
			label = InkSurface{OnTap: func() { c.OnSort(asc) }, ContentColor: fg, Radius: CornerExtraSmall, NoFocus: true, Child: label}
		}
		header = append(header, cell(i, label))
	}
	rows := []w.Widget{w.DecoratedBox{Color: pick(st.HeadingRowColor, geom.Transparent),
		Child: w.SizedBox{Height: pickF(st.HeadingRowHeight, 56), Child: w.Row{Cross: w.CrossCenter, Children: header}}}, divider}
	for _, r := range t.Rows {
		kids := []w.Widget{}
		if checkboxes {
			kids = append(kids, w.SizedBox{Width: 56, Child: w.Center{Child: Checkbox{Value: r.Selected, OnChanged: r.OnSelectChanged}}})
		}
		for i := range t.Columns {
			var c w.Widget = w.SizedBox{}
			if i < len(r.Cells) {
				c = r.Cells[i]
			}
			kids = append(kids, cell(i, c))
		}
		var body w.Widget = w.SizedBox{Height: pickF(st.DataRowHeight, 52), Child: w.Row{Cross: w.CrossCenter, Children: kids}}
		bg := pick(st.DataRowColor, geom.Transparent)
		if r.Selected {
			bg = pick(st.SelectedRowColor, sc.Primary.WithAlpha(0.08))
		}
		if r.OnSelectChanged != nil {
			r := r
			body = InkSurface{OnTap: func() { r.OnSelectChanged(!r.Selected) }, Color: bg, ContentColor: sc.OnSurface, NoFocus: true, Child: body}
		} else {
			body = w.DecoratedBox{Color: bg, Child: body}
		}
		rows = append(rows, body, divider)
	}
	return w.DefaultTextStyle{Style: pickTC(st.DataTextStyle, th.Text.BodyMedium, sc.OnSurface),
		Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: rows}}
}

// ---------------------------------------------------------------------------
// Stepper

// StepState marks a step's status.
type StepState uint8

const (
	StepIndexed StepState = iota
	StepEditing
	StepComplete
	StepDisabled
	StepError
)

// Step is one step of a Stepper.
type Step struct {
	Title    string
	Subtitle string
	Content  w.Widget
	State    StepState
}

// Stepper walks through numbered steps vertically.
type Stepper struct {
	Steps        []Step
	Current      int
	OnStepTapped func(int)
	OnContinue   func()
	OnCancel     func()
	// Style overrides Theme.Stepper.
	Style StepperTheme
}

func (Stepper) CreateState() w.State { return &stepperState{} }

type stepperState struct {
	w.StateBase
	open []*w.Animated
}

func (s *stepperState) Build(ctx w.BuildContext) w.Widget {
	sp := w.WidgetOf[Stepper](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Stepper, sp.Style)
	activeC, activeFG := pick(st.ActiveColor, sc.Primary), pick(st.ActiveIconColor, sc.OnPrimary)
	errC := pick(st.ErrorColor, sc.Error)
	for len(s.open) < len(sp.Steps) {
		v := float32(0)
		if len(s.open) == sp.Current {
			v = 1
		}
		s.open = append(s.open, w.NewAnimated(s, 250*time.Millisecond, w.Emphasized, v))
	}
	kids := []w.Widget{}
	for i, step := range sp.Steps {
		i := i
		active := i == sp.Current
		if active {
			s.open[i].Set(1)
		} else {
			s.open[i].Set(0)
		}
		circleBG, circleFG := pick(st.InactiveColor, sc.OnSurface.WithAlpha(DisabledContentOpacity)), pick(st.InactiveIconColor, sc.Surface)
		var mark w.Widget = w.Text{Text: strconv.Itoa(i + 1), Style: Styled(th.Text.LabelMedium, circleFG)}
		switch {
		case step.State == StepError:
			circleBG, circleFG = errC, sc.OnError
			mark = w.Text{Text: "!", Style: Styled(th.Text.LabelMedium, circleFG)}
		case step.State == StepComplete:
			circleBG, circleFG = activeC, activeFG
			mark = w.Icon{Icon: iconCheck, Size: 16, Color: circleFG}
		case active || step.State == StepEditing:
			circleBG, circleFG = activeC, activeFG
			if step.State == StepEditing {
				mark = w.Icon{Icon: iconEdit, Size: 14, Color: circleFG}
			} else {
				mark = w.Text{Text: strconv.Itoa(i + 1), Style: Styled(th.Text.LabelMedium, circleFG)}
			}
		}
		titleC := pick(st.TitleTextStyle.Color, sc.OnSurface)
		if step.State == StepDisabled {
			titleC, _ = disabledColors(sc)
		}
		if step.State == StepError {
			titleC = errC
		}
		ts := pickT(st.TitleTextStyle, th.Text.BodyLarge)
		ts.Color = titleC
		texts := []w.Widget{w.Text{Text: step.Title, Style: ts}}
		if step.Subtitle != "" {
			texts = append(texts, w.Text{Text: step.Subtitle, Style: pickTC(st.SubtitleTextStyle, th.Text.BodySmall, sc.OnSurfaceVariant)})
		}
		var tap func()
		if sp.OnStepTapped != nil && step.State != StepDisabled {
			tap = func() { sp.OnStepTapped(i) }
		}
		kids = append(kids, InkSurface{OnTap: tap, ContentColor: sc.OnSurface, NoFocus: true, Child: w.Padding{Padding: geom.InsetsHV(24, 12),
			Child: w.Row{Cross: w.CrossCenter, Spacing: 12, Children: []w.Widget{
				w.Container{Width: 24, Height: 24, Color: circleBG, Border: &geom.Border{Radius: 12}, Alignment: w.Ptr(geom.Center), Child: mark},
				w.Column{Cross: w.CrossStart, ShrinkMain: true, Children: texts},
			}}}})
		body := []w.Widget{}
		if step.Content != nil {
			body = append(body, step.Content)
		}
		ctrls := []w.Widget{}
		if sp.OnContinue != nil {
			ctrls = append(ctrls, FilledButton{Label: "Continue", OnPressed: sp.OnContinue})
		}
		if sp.OnCancel != nil {
			ctrls = append(ctrls, TextButton{Label: "Cancel", OnPressed: sp.OnCancel})
		}
		if len(ctrls) > 0 {
			body = append(body, w.SizedBox{Height: 16}, w.Row{ShrinkMain: true, Spacing: 8, Children: ctrls})
		}
		line := pick(st.ConnectorColor, sc.OutlineVariant)
		// Content with the connector line on the left (painted, so it spans
		// the content's height even inside the size transition).
		kids = append(kids, w.SizeTransition{Factor: s.open[i].Value(), Child: w.CustomPaint{
			Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
				c.FillRect(geom.Rect{X: o.X + 35.5, Y: o.Y, W: 1, H: size.H}, line)
			},
			Child: w.Padding{Padding: geom.InsetsLTRB(60, 0, 24, 16), Child: w.Column{Cross: w.CrossStart, ShrinkMain: true, Children: body}},
		}})
	}
	return w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: kids}
}
