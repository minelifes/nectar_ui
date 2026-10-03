package widgets

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// GridColumn describes one column of a DataGrid.
type GridColumn struct {
	Title     string
	Width     float32 // initial width; 0 = 120
	MinWidth  float32 // when resizing; 0 = 40
	Resizable bool    // drag the header's right edge to resize
	Align     text.Align
}

// GridStyle colors a DataGrid. Zero colors are not painted.
type GridStyle struct {
	HeaderColor     geom.Color
	RowColor        geom.Color
	AltRowColor     geom.Color // every other row; zero = RowColor
	SelectedColor   geom.Color
	LineColor       geom.Color // grid lines
	TextStyle       text.Style
	HeaderTextStyle text.Style
	CellPadding     geom.EdgeInsets // zero = 8 horizontally
	ResizeHandle    geom.Color      // the resize grip while hovered
}

// DataGrid is a virtualized table for large data: only the visible rows
// are built, the header stays at the top while rows scroll vertically, and
// header and rows scroll horizontally together. Columns can be resized.
// Cells are text by default (CellText) or any widget (Cell).
type DataGrid struct {
	Columns  []GridColumn
	RowCount int
	// CellText returns the text of a cell (used when Cell is nil).
	CellText func(row, col int) string
	// Cell builds a cell's widget.
	Cell func(ctx BuildContext, row, col int) Widget
	// Header builds a header cell; nil = the column title.
	Header       func(ctx BuildContext, col int) Widget
	RowHeight    float32 // 0 = 32
	HeaderHeight float32 // 0 = 36
	Selected     int     // highlighted row; -1 (or out of range) = none
	OnRowTap     func(row int)
	OnHeaderTap  func(col int)
	// OnColumnResized reports a column's new width as the user drags.
	OnColumnResized func(col int, width float32)
	Vertical        *ScrollController // nil = internal
	Horizontal      *ScrollController // nil = internal
	ThumbColor      geom.Color
	Style           GridStyle
}

func (DataGrid) CreateState() State { return &gridState{} }

type gridState struct {
	StateBase
	widths      []float32
	v, h, hh    *ScrollController
	syncing     bool
	hoverHandle int
}

func (s *gridState) g() DataGrid { return WidgetOf[DataGrid](s) }

func (s *gridState) InitState() {
	g := s.g()
	s.hoverHandle = -1
	s.v, s.h = g.Vertical, g.Horizontal
	if s.v == nil {
		s.v = NewScrollController()
	}
	if s.h == nil {
		s.h = NewScrollController()
	}
	s.hh = NewScrollController()
	// The header follows the body sideways, and the other way round.
	s.h.AddListener(func() { s.follow(s.hh, s.h) })
	s.hh.AddListener(func() { s.follow(s.h, s.hh) })
	s.syncWidths()
}

func (s *gridState) follow(dst, src *ScrollController) {
	if s.syncing {
		return
	}
	s.syncing = true
	dst.JumpTo(src.Offset())
	s.syncing = false
}

func (s *gridState) DidUpdateWidget(Widget) { s.syncWidths() }

// syncWidths keeps user-resized widths and takes new columns' widths.
func (s *gridState) syncWidths() {
	cols := s.g().Columns
	old := s.widths
	s.widths = make([]float32, len(cols))
	for i, c := range cols {
		switch {
		case i < len(old):
			s.widths[i] = old[i]
		case c.Width > 0:
			s.widths[i] = c.Width
		default:
			s.widths[i] = 120
		}
	}
}

// totalWidth is the sum of the column widths.
func (s *gridState) totalWidth() float32 {
	var t float32
	for _, w := range s.widths {
		t += w
	}
	return t
}

func (s *gridState) resize(col int, delta float32) {
	c := s.g().Columns[col]
	minW := c.MinWidth
	if minW <= 0 {
		minW = 40
	}
	s.SetState(func() { s.widths[col] = max(s.widths[col]+delta, minW) })
	if cb := s.g().OnColumnResized; cb != nil {
		cb(col, s.widths[col])
	}
}

func (s *gridState) Build(ctx BuildContext) Widget {
	g := s.g()
	st := g.Style
	rowH, headH := g.RowHeight, g.HeaderHeight
	if rowH <= 0 {
		rowH = 32
	}
	if headH <= 0 {
		headH = 36
	}
	pad := st.CellPadding
	if pad.IsZero() {
		pad = geom.InsetsHV(8, 0)
	}
	total := s.totalWidth()
	line := func() Widget {
		if st.LineColor.A <= 0 {
			return nil
		}
		return SizedBox{Width: 1, Child: DecoratedBox{Color: st.LineColor}}
	}
	cell := func(col int, child Widget) Widget {
		align := geom.CenterLeft
		switch g.Columns[col].Align {
		case text.AlignCenter:
			align = geom.Center
		case text.AlignEnd:
			align = geom.CenterRight
		}
		return SizedBox{Width: s.widths[col], Child: ClipRect{Child: Padding{Padding: pad, Child: Align{Alignment: align, Child: child}}}}
	}

	// Header.
	heads := make([]Widget, 0, len(g.Columns))
	for i, c := range g.Columns {
		var content Widget = Text{Text: c.Title, Style: st.HeaderTextStyle, MaxLines: 1, Ellipsis: true, Align: c.Align}
		if g.Header != nil {
			content = g.Header(ctx, i)
		}
		col := i
		hc := cell(i, content)
		if g.OnHeaderTap != nil {
			hc = GestureDetector{OnTap: func() { g.OnHeaderTap(col) }, Child: hc}
		}
		layers := []Widget{hc}
		if c.Resizable {
			grip := geom.Color{}
			if s.hoverHandle == col {
				grip = st.ResizeHandle
			}
			layers = append(layers, Positioned{Right: At(0), Top: At(0), Bottom: At(0), Width: At(6),
				Child: MouseRegion{Cursor: CursorResizeEW,
					OnEnter: func(PointerEvent) { s.SetState(func() { s.hoverHandle = col }) },
					OnExit:  func(PointerEvent) { s.SetState(func() { s.hoverHandle = -1 }) },
					Child: GestureDetector{OnPanUpdate: func(d DragDetails) { s.resize(col, d.Delta.X) },
						Child: DecoratedBox{Color: grip}}}})
		}
		heads = append(heads, KeyedSubtree{ID: i, Child: SizedBox{Width: s.widths[i], Height: headH, Child: Stack{Children: layers}}})
		if l := line(); l != nil {
			heads = append(heads, KeyedSubtree{ID: -1 - i, Child: l})
		}
	}
	header := DecoratedBox{Color: st.HeaderColor, Child: SizedBox{Height: headH,
		Child: ScrollView{Horizontal: true, Controller: s.hh, strictAxis: true, Child: Row{Children: heads}}}}

	// Rows.
	rows := ListViewBuilder{Controller: s.v, ItemCount: max(g.RowCount, 0), ItemExtent: rowH, ThumbColor: g.ThumbColor,
		Builder: func(ctx BuildContext, r int) Widget {
			kids := make([]Widget, 0, 2*len(g.Columns))
			for c := range g.Columns {
				var content Widget
				switch {
				case g.Cell != nil:
					content = g.Cell(ctx, r, c)
				case g.CellText != nil:
					content = Text{Text: g.CellText(r, c), Style: st.TextStyle, MaxLines: 1, Ellipsis: true, Align: g.Columns[c].Align}
				default:
					content = SizedBox{}
				}
				kids = append(kids, cell(c, content))
				if l := line(); l != nil {
					kids = append(kids, l)
				}
			}
			bg := st.RowColor
			if r%2 == 1 && st.AltRowColor.A > 0 {
				bg = st.AltRowColor
			}
			if r == g.Selected {
				bg = st.SelectedColor
			}
			var row Widget = DecoratedBox{Color: bg, Child: Row{Children: kids}}
			if g.OnRowTap != nil {
				row = GestureDetector{OnTap: func() { g.OnRowTap(r) }, Child: row}
			}
			return row
		}}
	lines := float32(0)
	if st.LineColor.A > 0 {
		lines = float32(len(g.Columns))
	}
	body := ScrollView{Horizontal: true, Controller: s.h, ThumbColor: g.ThumbColor, strictAxis: true,
		Child: SizedBox{Width: total + lines, Child: rows}}
	kids := []Widget{header}
	if st.LineColor.A > 0 {
		kids = append(kids, SizedBox{Height: 1, Child: DecoratedBox{Color: st.LineColor}})
	}
	kids = append(kids, Expanded{Child: body})
	return Column{Cross: CrossStretch, Children: kids}
}
