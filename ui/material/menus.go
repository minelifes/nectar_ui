package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// MenuItem is one entry of a menu.
type MenuItem struct {
	Label    string
	Leading  *vector.Icon
	Trailing string // e.g. a shortcut "⌘C"
	OnTap    func()
	Disabled bool
	Divider  bool // draws a divider instead of an item
	Selected bool
}

// MenuOptions configure ShowMenu.
type MenuOptions struct {
	Width float32 // 0 = fit content (112..280)
	// Above opens the menu above the anchor when there's room.
	Above bool
}

// ShowMenu opens a menu anchored below anchor (window coordinates). It
// closes on selection, outside tap or Escape.
func ShowMenu(ctx w.BuildContext, anchor geom.Rect, items []MenuItem, opts MenuOptions) {
	ov := w.OverlayOf(ctx)
	if ov == nil {
		return
	}
	th := ThemeOf(ctx)
	mt := th.Menu
	ts := pickT(mt.TextStyle, th.Text.LabelLarge)
	itemH := pickF(mt.ItemHeight, 48)
	win := ov.Context().RenderObject().Base().Size()
	width := opts.Width
	if width == 0 {
		for _, it := range items {
			if it.Divider {
				continue
			}
			iw := text.Layout(it.Label, ts, text.Options{}).Width + 24
			if it.Leading != nil {
				iw += 36
			}
			if it.Trailing != "" {
				iw += text.Layout(it.Trailing, ts, text.Options{}).Width + 24
			}
			width = max(width, iw)
		}
		width = min(max(width, 112), 280)
	}
	h := float32(16)
	for _, it := range items {
		if it.Divider {
			h += 17
		} else {
			h += itemH
		}
	}
	x := min(max(anchor.X, 8), win.W-width-8)
	y := anchor.Bottom()
	if opts.Above || y+h > win.H-8 {
		if anchor.Y-h >= 8 {
			y = anchor.Y - h
		} else {
			y = max(8, win.H-h-8)
		}
	}
	var entry *w.OverlayEntry
	closeMenu := func() {
		if entry != nil {
			entry.Remove()
			entry = nil
		}
	}
	entry = &w.OverlayEntry{Builder: func(ctx w.BuildContext) w.Widget {
		return w.Stack{Expand: true, Children: []w.Widget{
			w.PositionedFill(w.GestureDetector{OnTapDown: func(w.TapDetails) { closeMenu() }, Child: w.AbsorbPointer{}}),
			w.Positioned{Left: w.At(x), Top: w.At(y), Width: w.At(width),
				Child: menuPanel{items: items, close: closeMenu}},
		}}
	}}
	ov.Insert(entry)
}

type menuPanel struct {
	items []MenuItem
	close func()
}

func (menuPanel) CreateState() w.State { return &menuPanelState{} }

type menuPanelState struct {
	w.StateBase
	t *w.Animated
}

func (s *menuPanelState) InitState() {
	s.t = w.NewAnimated(s, 150*time.Millisecond, w.EmphasizedDecelerate, 0)
	s.t.Set(1)
}

func (s *menuPanelState) Build(ctx w.BuildContext) w.Widget {
	m := w.WidgetOf[menuPanel](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	mt := th.Menu
	itemH := pickF(mt.ItemHeight, 48)
	rows := []w.Widget{}
	for _, it := range m.items {
		if it.Divider {
			rows = append(rows, w.Padding{Padding: geom.InsetsHV(0, 8), Child: Divider{}})
			continue
		}
		fg, ic := pick(mt.TextColor, sc.OnSurface), pick(mt.IconColor, sc.OnSurfaceVariant)
		if it.Disabled {
			fg, _ = disabledColors(sc)
			ic = fg
		}
		kids := []w.Widget{}
		if it.Leading != nil {
			kids = append(kids, w.Icon{Icon: it.Leading, Size: 24, Color: ic})
		}
		kids = append(kids, w.Expanded{Child: w.Text{Text: it.Label, Style: pickTC(mt.TextStyle, th.Text.LabelLarge, fg), MaxLines: 1, Ellipsis: true}})
		if it.Trailing != "" {
			kids = append(kids, w.Text{Text: it.Trailing, Style: pickTC(mt.TextStyle, th.Text.LabelLarge, ic)})
		}
		var tap func()
		if !it.Disabled {
			it := it
			tap = func() {
				m.close()
				if it.OnTap != nil {
					it.OnTap()
				}
			}
		}
		bg := geom.Transparent
		if it.Selected {
			bg = pick(mt.SelectedColor, sc.OnSurface.WithAlpha(0.10))
		}
		rows = append(rows, InkSurface{OnTap: tap, Color: bg, ContentColor: sc.OnSurface, Disabled: it.Disabled,
			Child: w.SizedBox{Height: itemH, Child: w.Padding{Padding: geom.InsetsHV(12, 0),
				Child: w.Row{Cross: w.CrossCenter, Spacing: 12, Children: kids}}}})
	}
	t := s.t.Value()
	return w.Focus{Autofocus: true, OnKey: func(e w.KeyEvent) bool {
		if e.Key == w.KeyEscape {
			m.close()
			return true
		}
		return false
	}, Child: w.Opacity{Opacity: t, Child: w.SizeTransition{Factor: 0.6 + 0.4*t,
		Child: w.AbsorbPointer{Child: Surface{Color: pick(mt.BackgroundColor, sc.SurfaceContainer), Radius: pickF(mt.Radius, CornerExtraSmall),
			Elevation: pickI(mt.Elevation, 2), ShadowColor: mt.ShadowColor,
			Child: w.Padding{Padding: geom.InsetsHV(0, 8), Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Children: rows}}}}}}}
}

// anchorRect returns ctx's widget rect in window coordinates.
func anchorRect(ctx w.BuildContext) geom.Rect {
	ro := ctx.RenderObject()
	if ro == nil {
		return geom.Rect{}
	}
	return geom.RectFrom(render.GlobalOrigin(ro), ro.Base().Size())
}

// PopupMenuButton is an icon button that opens a menu.
type PopupMenuButton struct {
	Icon  *vector.Icon // default: more_vert
	Items []MenuItem
}

func (p PopupMenuButton) Build(ctx w.BuildContext) w.Widget {
	ic := p.Icon
	if ic == nil {
		ic = iconMoreVert
	}
	return IconButton{Icon: ic, OnPressed: func() {
		ShowMenu(ctx, anchorRect(ctx), p.Items, MenuOptions{})
	}}
}

// DropdownEntry is one option of a DropdownMenu.
type DropdownEntry struct {
	Label string
	Value any // comparable
	Icon  *vector.Icon
}

// DropdownMenu is an outlined field showing the selection; tap to choose.
type DropdownMenu struct {
	Label      string
	Entries    []DropdownEntry
	Selected   any
	OnSelected func(v any) // nil = disabled
	Width      float32     // default Style / 200
	// Style overrides Theme.DropdownMenu (the menu itself uses Theme.Menu).
	Style DropdownMenuTheme
}

func (d DropdownMenu) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.DropdownMenu, d.Style)
	width := d.Width
	if width == 0 {
		width = pickF(st.Width, 200)
	}
	radius := pickF(st.Radius, CornerExtraSmall)
	sel := ""
	for _, e := range d.Entries {
		if e.Value == d.Selected {
			sel = e.Label
		}
	}
	fg, border := pick(st.TextStyle.Color, sc.OnSurface), pick(st.BorderColor, sc.Outline)
	if d.OnSelected == nil {
		fg, _ = disabledColors(sc)
		border = fg
	}
	labelSt := pickTC(st.LabelStyle, th.Text.BodySmall, sc.OnSurfaceVariant)
	labelW := text.Layout(d.Label, labelSt, text.Options{}).Width + 8
	bgNotch := pick(th.ScaffoldBackground, sc.Surface)
	kids := []w.Widget{
		w.Expanded{Child: w.Text{Text: sel, Style: pickTC(st.TextStyle, th.Text.BodyLarge, fg), MaxLines: 1, Ellipsis: true}},
		w.Icon{Icon: iconArrowDrop, Size: 24, Color: pick(st.IconColor, sc.OnSurfaceVariant)},
	}
	var open func()
	if d.OnSelected != nil {
		open = func() {
			items := make([]MenuItem, len(d.Entries))
			for i, e := range d.Entries {
				e := e
				items[i] = MenuItem{Label: e.Label, Leading: e.Icon, Selected: e.Value == d.Selected, OnTap: func() { d.OnSelected(e.Value) }}
			}
			ShowMenu(ctx, anchorRect(ctx), items, MenuOptions{Width: width})
		}
	}
	field := w.CustomPaint{
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			r := geom.RectFrom(o, size)
			c.StrokeRoundRect(r, radius, 1, border)
			if d.Label != "" {
				c.FillRect(geom.Rect{X: o.X + 12, Y: o.Y - 2, W: labelW, H: 4}, bgNotch)
			}
		},
		Child: w.SizedBox{Height: 56, Child: w.Padding{Padding: geom.InsetsLTRB(16, 0, 12, 0), Child: w.Row{Cross: w.CrossCenter, Children: kids}}},
	}
	stack := []w.Widget{InkSurface{OnTap: open, ContentColor: sc.OnSurface, Radius: radius, Child: field}}
	if d.Label != "" {
		stack = append(stack, w.Positioned{Left: w.At(16), Top: w.At(-8),
			Child: w.IgnorePointer{Ignoring: true, Child: w.Text{Text: d.Label, Style: labelSt}}})
	}
	return w.SizedBox{Width: width, Child: w.Stack{Children: stack}}
}
