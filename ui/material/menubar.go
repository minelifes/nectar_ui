package material

import (
	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/menu"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// MenuItemsOf converts menu items (package menu) to material menu items,
// resolving the commands they name against snap (nil = none; take one with
// commands.HostOf(ctx).Snapshot() before the menu opens).
func MenuItemsOf(items []menu.Item, snap *commands.Snapshot) []MenuItem {
	items = menu.Resolve(items, snap)
	out := make([]MenuItem, 0, len(items))
	for _, it := range items {
		if it.Separator {
			out = append(out, MenuItem{Divider: true})
			continue
		}
		out = append(out, MenuItem{Label: it.Label, Trailing: it.Shortcut, OnTap: it.Action,
			Disabled: it.Disabled || (it.Action == nil && len(it.Submenu) == 0), Checked: it.Checked,
			Submenu: MenuItemsOf(it.Submenu, snap)})
	}
	return out
}

// snapshotOf captures the commands available at the focus, if a
// commands.Host is above ctx.
func snapshotOf(ctx w.BuildContext) *commands.Snapshot {
	if h := commands.HostOf(ctx); h != nil {
		return h.Snapshot()
	}
	return nil
}

// ShowContextMenu opens items at a point (window coordinates).
func ShowContextMenu(ctx w.BuildContext, at geom.Offset, items []menu.Item) {
	ShowMenu(ctx, geom.Rect{X: at.X, Y: at.Y}, MenuItemsOf(items, snapshotOf(ctx)), MenuOptions{})
}

// ContextMenuRegion opens a menu where Child is right-clicked (or
// long-pressed, on touch screens). Items is called each time, so it can
// depend on what was clicked and on the current state.
type ContextMenuRegion struct {
	Items func() []menu.Item
	Child w.Widget
}

func (c ContextMenuRegion) Build(ctx w.BuildContext) w.Widget {
	return w.Listener{OnEvent: func(e w.PointerEvent) {
		if e.Kind == w.PointerDown && e.Button == w.ButtonSecondary && c.Items != nil {
			if items := c.Items(); len(items) > 0 {
				ShowContextMenu(ctx, e.Position, items)
			}
		}
	}, Child: c.Child}
}

// MenuBar is an application menu bar: File, Edit, View... Click a title to
// open its menu, then move along the bar by hovering or with ←/→. Items
// that name commands show their shortcuts and run them (see menu.Item).
// On macOS consider ui.App.SetNativeMenu instead (or as well).
type MenuBar struct {
	Menus []menu.Menu
	// Style overrides Theme.MenuBar.
	Style MenuBarTheme
}

func (MenuBar) CreateState() w.State { return &menuBarState{} }

type menuBarState struct {
	w.StateBase
	open  int // open menu (-1 = none)
	hd    *menuHandle
	ctxs  []w.BuildContext
	hover int
}

// menuBarLink lets an open menu talk to its bar: hovering or tapping
// another title switches menus, ←/→ move along the bar.
type menuBarLink struct{ s *menuBarState }

func (b *menuBarLink) titleAt(p geom.Offset) int {
	for i, c := range b.s.ctxs {
		if c != nil && anchorRect(c).Contains(p) {
			return i
		}
	}
	return -1
}

func (b *menuBarLink) hoverAt(p geom.Offset) {
	if i := b.titleAt(p); i >= 0 && i != b.s.open {
		b.s.openMenu(i, false)
	}
}

// tapAt handles a tap on the barrier: on the open title it closes the
// menu, on another title it switches. Returns false elsewhere.
func (b *menuBarLink) tapAt(p geom.Offset) bool {
	i := b.titleAt(p)
	if i < 0 {
		return false
	}
	if i == b.s.open {
		b.s.closeMenu()
	} else {
		b.s.openMenu(i, false)
	}
	return true
}

func (b *menuBarLink) move(d int) {
	n := len(w.WidgetOf[MenuBar](b.s).Menus)
	if n == 0 {
		return
	}
	b.s.openMenu(((b.s.open+d)%n+n)%n, true)
}

func (s *menuBarState) InitState() { s.open, s.hover = -1, -1 }

func (s *menuBarState) Dispose() { s.closeMenu() }

func (s *menuBarState) closeMenu() {
	if s.hd != nil {
		hd := s.hd
		s.hd = nil
		hd.closeAll()
	}
	if s.Mounted() {
		s.SetState(func() { s.open = -1 })
	}
}

func (s *menuBarState) openMenu(i int, focusFirst bool) {
	mb := w.WidgetOf[MenuBar](s)
	if i < 0 || i >= len(mb.Menus) || i >= len(s.ctxs) || s.ctxs[i] == nil {
		return
	}
	if s.hd != nil {
		old := s.hd
		s.hd = nil
		old.onClose = nil
		old.closeAll()
	}
	link := &menuBarLink{s}
	items := MenuItemsOf(mb.Menus[i].Items, snapshotOf(s.Context()))
	var hd *menuHandle
	hd = openMenu(s.ctxs[i], anchorRect(s.ctxs[i]), items, MenuOptions{bar: link, OnClose: func() {
		if s.hd == hd {
			s.hd = nil
			if s.Mounted() {
				s.SetState(func() { s.open = -1 })
			}
		}
	}})
	s.hd = hd
	if hd != nil {
		hd.focusFirst = focusFirst
	}
	s.SetState(func() { s.open = i })
}

func (s *menuBarState) Build(ctx w.BuildContext) w.Widget {
	mb := w.WidgetOf[MenuBar](s)
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.MenuBar, mb.Style)
	ts := pickTC(st.TextStyle, th.Text.LabelLarge, sc.OnSurface)
	hl := pick(st.HighlightColor, sc.OnSurface.WithAlpha(0.10))
	h := pickF(st.Height, 32)
	if len(s.ctxs) != len(mb.Menus) {
		s.ctxs = make([]w.BuildContext, len(mb.Menus))
	}
	titles := make([]w.Widget, len(mb.Menus))
	for i, m := range mb.Menus {
		i, m := i, m
		bg := geom.Transparent
		if i == s.open || (s.open < 0 && i == s.hover) {
			bg = hl
		}
		titles[i] = w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
			s.ctxs[i] = ctx
			return w.MouseRegion{
				OnEnter: func(w.PointerEvent) { s.SetState(func() { s.hover = i }) },
				OnExit: func(w.PointerEvent) {
					s.SetState(func() {
						if s.hover == i {
							s.hover = -1
						}
					})
				},
				Child: w.GestureDetector{OnTap: func() {
					if s.open == i {
						s.closeMenu()
					} else {
						s.openMenu(i, false)
					}
				}, Child: w.DecoratedBox{Color: bg, Border: &geom.Border{Radius: pickF(st.Radius, CornerExtraSmall)},
					Child: w.Padding{Padding: pickE(st.ItemPadding, geom.InsetsHV(10, 0)),
						Child: w.Align{Alignment: geom.Center, Child: w.Text{Text: m.Title, Style: ts}}}}},
			}
		}}
	}
	return w.DecoratedBox{Color: pick(st.BackgroundColor, sc.SurfaceContainer),
		Child: w.SizedBox{Height: h, Child: w.Padding{Padding: geom.InsetsHV(4, 2), Child: w.Row{Cross: w.CrossStretch, Spacing: 2, Children: titles}}}}
}
