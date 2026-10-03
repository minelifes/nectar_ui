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
	Checked  bool       // a check mark in the leading slot
	Submenu  []MenuItem // opens beside the item (hover, tap or →)
}

// MenuOptions configure ShowMenu.
type MenuOptions struct {
	Width float32 // 0 = fit content (112..280)
	// Above opens the menu above the anchor when there's room.
	Above bool
	// OnClose is called when the menu closes (picked, dismissed).
	OnClose func()

	side   bool        // open beside the anchor (submenus)
	parent *menuHandle // the menu a submenu belongs to
	bar    *menuBarLink
}

// menuHandle is one open menu (a submenu has a parent).
type menuHandle struct {
	entry         *w.OverlayEntry
	parent, child *menuHandle
	node          *w.FocusNode
	closed        bool
	onClose       func()
	bar           *menuBarLink
	focusFirst    bool // highlight the first item (opened by keyboard)
}

// close closes h and its submenus; focus returns to the parent menu.
func (h *menuHandle) close() {
	if h == nil || h.closed {
		return
	}
	h.closed = true
	h.child.close()
	h.entry.Remove()
	if h.parent != nil {
		h.parent.child = nil
		h.parent.node.RequestFocus()
	} else if h.onClose != nil {
		h.onClose()
	}
}

// closeAll closes the whole menu chain.
func (h *menuHandle) closeAll() {
	for h.parent != nil {
		h = h.parent
	}
	h.close()
}

// ShowMenu opens a menu anchored below anchor (window coordinates). It
// closes on selection, outside tap or Escape. ↑/↓ move between items,
// Enter picks, → opens a submenu and ← closes it.
func ShowMenu(ctx w.BuildContext, anchor geom.Rect, items []MenuItem, opts MenuOptions) {
	openMenu(ctx, anchor, items, opts)
}

func openMenu(ctx w.BuildContext, anchor geom.Rect, items []MenuItem, opts MenuOptions) *menuHandle {
	ov := w.OverlayOf(ctx)
	if ov == nil {
		return nil
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
			if it.Leading != nil || it.Checked {
				iw += 36
			}
			if it.Trailing != "" {
				iw += text.Layout(it.Trailing, ts, text.Options{}).Width + 24
			}
			if len(it.Submenu) > 0 {
				iw += 36
			}
			width = max(width, iw)
		}
		width = min(max(width, 112), 320)
	}
	h := float32(16)
	for _, it := range items {
		if it.Divider {
			h += 17
		} else {
			h += itemH
		}
	}
	var x, y float32
	if opts.side {
		// Beside the anchor (a submenu item), flipped left if no room.
		x, y = anchor.Right(), anchor.Y-8
		if x+width > win.W-8 {
			x = anchor.X - width
		}
		x = min(max(x, 8), win.W-width-8)
		y = min(max(y, 8), max(8, win.H-h-8))
	} else {
		x = min(max(anchor.X, 8), win.W-width-8)
		y = anchor.Bottom()
		if opts.Above || y+h > win.H-8 {
			if anchor.Y-h >= 8 {
				y = anchor.Y - h
			} else {
				y = max(8, win.H-h-8)
			}
		}
	}
	hd := &menuHandle{parent: opts.parent, node: &w.FocusNode{}, onClose: opts.OnClose, bar: opts.bar}
	if hd.parent != nil {
		hd.parent.child.close()
		hd.parent.child = hd
		hd.bar = hd.parent.bar
	}
	panel := w.Positioned{Left: w.At(x), Top: w.At(y), Width: w.At(width),
		Child: menuPanel{items: items, handle: hd}}
	hd.entry = &w.OverlayEntry{Builder: func(ctx w.BuildContext) w.Widget {
		if hd.parent != nil {
			return w.Stack{Expand: true, Children: []w.Widget{panel}}
		}
		// The root menu has the barrier: a tap outside closes everything.
		barrier := w.Widget(w.GestureDetector{OnTapDown: func(d w.TapDetails) {
			if b := hd.bar; b != nil && b.tapAt(d.Global) {
				return
			}
			hd.closeAll()
		}, Child: w.AbsorbPointer{}})
		if b := hd.bar; b != nil {
			barrier = w.MouseRegion{OnHover: func(e w.PointerEvent) { b.hoverAt(e.Position) }, Child: barrier}
		}
		return w.Stack{Expand: true, Children: []w.Widget{w.PositionedFill(barrier), panel}}
	}}
	ov.Insert(hd.entry)
	return hd
}

type menuPanel struct {
	items  []MenuItem
	handle *menuHandle
}

func (menuPanel) CreateState() w.State { return &menuPanelState{} }

type menuPanelState struct {
	w.StateBase
	t   *w.Animated
	hi  int // highlighted item (-1 = none)
	ctx []w.BuildContext
}

func (s *menuPanelState) InitState() {
	s.hi = -1
	s.t = w.NewAnimated(s, 150*time.Millisecond, w.EmphasizedDecelerate, 0)
	s.t.Set(1)
}

func (s *menuPanelState) panel() menuPanel { return w.WidgetOf[menuPanel](s) }

func selectable(it MenuItem) bool { return !it.Divider && !it.Disabled }

// step moves the highlight to the next selectable item in direction d.
func (s *menuPanelState) step(d int) {
	items := s.panel().items
	n := len(items)
	if n == 0 {
		return
	}
	i := s.hi
	if i < 0 && d < 0 {
		i = n
	}
	for k := 0; k < n; k++ {
		i = ((i+d)%n + n) % n
		if selectable(items[i]) {
			s.SetState(func() { s.hi = i })
			return
		}
	}
}

func (s *menuPanelState) highlight(i int) {
	if s.hi != i {
		s.SetState(func() { s.hi = i })
	}
	m := s.panel()
	if i >= 0 && i < len(m.items) && len(m.items[i].Submenu) > 0 && !m.items[i].Disabled {
		s.openSub(i, false)
	} else if m.handle.child != nil {
		m.handle.child.close()
	}
}

// openSub opens item i's submenu (focused: highlight its first item).
func (s *menuPanelState) openSub(i int, focus bool) {
	m := s.panel()
	if i >= len(s.ctx) || s.ctx[i] == nil {
		return
	}
	sub := openMenu(s.ctx[i], anchorRect(s.ctx[i]), m.items[i].Submenu, MenuOptions{side: true, parent: m.handle})
	if sub != nil && focus {
		sub.focusFirst = true
	}
}

func (s *menuPanelState) activate(i int) {
	m := s.panel()
	if i < 0 || i >= len(m.items) || !selectable(m.items[i]) {
		return
	}
	it := m.items[i]
	if len(it.Submenu) > 0 {
		s.openSub(i, true)
		return
	}
	m.handle.closeAll()
	if it.OnTap != nil {
		it.OnTap()
	}
}

func (s *menuPanelState) onKey(e w.KeyEvent) bool {
	m := s.panel()
	switch e.Key {
	case w.KeyEscape:
		if m.handle.parent != nil {
			m.handle.close()
		} else {
			m.handle.closeAll()
		}
	case w.KeyDown:
		s.step(1)
	case w.KeyUp:
		s.step(-1)
	case w.KeyHome:
		s.hi = -1
		s.step(1)
	case w.KeyEnd:
		s.hi = 0
		s.step(-1)
	case w.KeyEnter, w.KeySpace:
		s.activate(s.hi)
	case w.KeyRight:
		if s.hi >= 0 && s.hi < len(m.items) && len(m.items[s.hi].Submenu) > 0 && !m.items[s.hi].Disabled {
			s.openSub(s.hi, true)
		} else if b := m.handle.bar; b != nil {
			b.move(1)
		}
	case w.KeyLeft:
		if m.handle.parent != nil {
			m.handle.close()
		} else if b := m.handle.bar; b != nil {
			b.move(-1)
		}
	default:
		return false
	}
	return true
}

func (s *menuPanelState) Build(ctx w.BuildContext) w.Widget {
	m := s.panel()
	if m.handle.focusFirst && s.hi < 0 {
		m.handle.focusFirst = false
		s.hi = -1
		for i, it := range m.items {
			if selectable(it) {
				s.hi = i
				break
			}
		}
	}
	th := ThemeOf(ctx)
	sc := th.Scheme
	mt := th.Menu
	itemH := pickF(mt.ItemHeight, 48)
	rows := []w.Widget{}
	if len(s.ctx) != len(m.items) {
		s.ctx = make([]w.BuildContext, len(m.items))
	}
	for i, it := range m.items {
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
		switch {
		case it.Leading != nil:
			kids = append(kids, w.Icon{Icon: it.Leading, Size: 24, Color: ic})
		case it.Checked:
			kids = append(kids, w.Icon{Icon: iconCheck, Size: 24, Color: ic})
		}
		kids = append(kids, w.Expanded{Child: w.Text{Text: it.Label, Style: pickTC(mt.TextStyle, th.Text.LabelLarge, fg), MaxLines: 1, Ellipsis: true}})
		if it.Trailing != "" {
			kids = append(kids, w.Text{Text: it.Trailing, Style: pickTC(mt.TextStyle, th.Text.LabelLarge, ic)})
		}
		if len(it.Submenu) > 0 {
			kids = append(kids, w.Icon{Icon: iconChevronRight, Size: 20, Color: ic})
		}
		var tap func()
		if !it.Disabled {
			i := i
			tap = func() { s.activate(i) }
		}
		bg := geom.Transparent
		switch {
		case it.Selected:
			bg = pick(mt.SelectedColor, sc.OnSurface.WithAlpha(0.10))
		case i == s.hi && !it.Disabled:
			bg = sc.OnSurface.WithAlpha(0.08)
		}
		i := i
		rows = append(rows, w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
			s.ctx[i] = ctx
			return w.MouseRegion{OnEnter: func(w.PointerEvent) {
				if !it.Disabled {
					s.highlight(i)
				}
			}, Child: InkSurface{OnTap: tap, Color: bg, ContentColor: sc.OnSurface, Disabled: it.Disabled, NoFocus: true,
				Child: w.SizedBox{Height: itemH, Child: w.Padding{Padding: geom.InsetsHV(12, 0),
					Child: w.Row{Cross: w.CrossCenter, Spacing: 12, Children: kids}}}}}
		}})
	}
	t := s.t.Value()
	return w.Focus{Node: m.handle.node, Autofocus: true, OnKey: s.onKey, Child: w.Opacity{Opacity: t, Child: w.SizeTransition{Factor: 0.6 + 0.4*t,
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
