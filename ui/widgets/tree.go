package widgets

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/vector"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// TreeNode is one item of a TreeView.
type TreeNode struct {
	// Key identifies the node (e.g. a file path) for expansion and
	// selection. Empty = parent's key + "/" + Label.
	Key   string
	Label string
	// Icon is drawn before the label; ExpandedIcon replaces it while the
	// node is expanded (e.g. an open folder). IconColor overrides the
	// style's icon color.
	Icon, ExpandedIcon *vector.Icon
	IconColor          geom.Color

	// Children are the static children. Load, if set, is called the first
	// time the node is expanded and its result is used instead (cached
	// until TreeController.Reload).
	Children []TreeNode
	Load     func() []TreeNode
	// Branch makes the node expandable even without children (an empty
	// folder). Nodes with Children or Load are always expandable.
	Branch bool
	// Expanded is the initial state (the controller wins once toggled).
	Expanded bool

	Data any // anything you want back in callbacks
}

// Expandable reports whether the node can have children.
func (n TreeNode) Expandable() bool { return n.Branch || len(n.Children) > 0 || n.Load != nil }

// TreeRow describes one visible row, for custom row builders.
type TreeRow struct {
	Node       TreeNode
	Key        string
	Depth      int
	Index      int // position among visible rows
	Expandable bool
	Expanded   bool
	Selected   bool
	Hovered    bool
	Focused    bool // the tree has keyboard focus
	Toggle     func()
}

// TreeStyle styles the default rows.
type TreeStyle struct {
	Text          text.Style
	SelectedText  geom.Color // label color on the selected row; zero = Text.Color
	IconColor     geom.Color // zero = Text.Color at 70%
	SelectedColor geom.Color // selected row background
	HoverColor    geom.Color
	FocusColor    geom.Color // outline of the selected row while focused; zero = none
	GuideColor    geom.Color // indent guide lines; zero = none
	Radius        float32    // highlight corner radius
	Inset         float32    // horizontal margin of the highlight
}

// TreeController holds expansion and selection so they can be changed from
// code; share one between rebuilds (and with the TreeView).
type TreeController struct {
	expanded  map[string]bool
	selected  string
	loaded    map[string][]TreeNode
	listeners []func()

	collapseAll bool // ignore TreeNode.Expanded after CollapseAll
	owned       bool // created by the TreeView itself
}

func NewTreeController() *TreeController {
	return &TreeController{expanded: map[string]bool{}, loaded: map[string][]TreeNode{}}
}

func (c *TreeController) AddListener(fn func()) { c.listeners = append(c.listeners, fn) }

func (c *TreeController) notify() {
	for _, fn := range c.listeners {
		fn()
	}
}

// IsExpanded reports whether key is expanded (false if never set).
func (c *TreeController) IsExpanded(key string) bool { return c.expanded[key] }

// SetExpanded expands or collapses key.
func (c *TreeController) SetExpanded(key string, open bool) {
	if v, ok := c.expanded[key]; ok && v == open {
		return
	}
	c.expanded[key] = open
	c.notify()
}

func (c *TreeController) Expand(key string)   { c.SetExpanded(key, true) }
func (c *TreeController) Collapse(key string) { c.SetExpanded(key, false) }

// CollapseAll collapses every node (including initially expanded ones).
func (c *TreeController) CollapseAll() {
	for k := range c.expanded {
		c.expanded[k] = false
	}
	c.collapseAll = true
	c.notify()
}

// Selected returns the selected key ("" = none).
func (c *TreeController) Selected() string { return c.selected }

// Select selects key (it doesn't expand its parents).
func (c *TreeController) Select(key string) {
	if c.selected != key {
		c.selected = key
		c.notify()
	}
}

// Reload forgets the lazily loaded children of key (all nodes when key is
// ""), so Load runs again, e.g. after files changed on disk.
func (c *TreeController) Reload(key string) {
	if key == "" {
		clear(c.loaded)
	} else {
		delete(c.loaded, key)
	}
	c.notify()
}

// TreeView shows hierarchical data (files and folders, outlines, …) as
// indented rows with expand arrows. Only the visible rows are built, so it
// handles large trees; it scrolls and must get a bounded height, unless
// ShrinkWrap is set.
//
// Mouse: clicking a row selects it and toggles a branch (unless
// TapSelectsOnly: then use the arrow or a double-click); double-clicking
// activates it. Keyboard: ↑/↓, Home/End, PageUp/PageDown move the
// selection; → expands (or goes to the first child); ← collapses (or goes
// to the parent); Enter activates; Space toggles.
type TreeView struct {
	Roots      []TreeNode
	Controller *TreeController

	OnSelect   func(node TreeNode)
	OnActivate func(node TreeNode) // double-click / Enter (branches also toggle)
	OnToggle   func(node TreeNode, expanded bool)

	RowHeight      float32 // default 28
	Indent         float32 // per level; default 16
	TapSelectsOnly bool
	ShrinkWrap     bool // size to all rows instead of scrolling
	Style          TreeStyle
	ThumbColor     geom.Color

	// Row replaces the default row content (the tree still handles taps,
	// hover and keys). The row is RowHeight tall and full width.
	Row func(ctx BuildContext, row TreeRow) Widget
}

func (TreeView) CreateState() State { return &treeState{} }

type treeState struct {
	StateBase
	ctrl    *TreeController
	scroll  *ScrollController
	node    *FocusNode
	focused bool
	hovered int
	rows    []treeRow

	lastTapKey  string
	lastTapTime time.Time
}

type treeRow struct {
	node   TreeNode
	key    string
	parent int // index of the parent row, -1 for roots
	depth  int
}

func (s *treeState) tv() TreeView { return WidgetOf[TreeView](s) }

func (s *treeState) InitState() {
	s.hovered = -1
	s.scroll = NewScrollController()
	s.node = &FocusNode{OnKey: s.onKey, OnFocusChange: func(f bool) { s.SetState(func() { s.focused = f }) }}
	s.attach()
}

func (s *treeState) DidUpdateWidget(Widget) { s.attach() }

func (s *treeState) attach() {
	c := s.tv().Controller
	if c == nil {
		if s.ctrl != nil && s.ctrl.owned {
			return
		}
		c = NewTreeController()
		c.owned = true
	}
	if c == s.ctrl {
		return
	}
	s.ctrl = c
	c.AddListener(func() {
		if s.ctrl == c && s.Mounted() {
			s.SetState(nil)
		}
	})
}

func (s *treeState) expanded(n TreeNode, key string) bool {
	if v, ok := s.ctrl.expanded[key]; ok {
		return v
	}
	return n.Expanded && !s.ctrl.collapseAll
}

func (s *treeState) children(n TreeNode, key string) []TreeNode {
	if n.Load == nil {
		return n.Children
	}
	if kids, ok := s.ctrl.loaded[key]; ok {
		return kids
	}
	kids := n.Load()
	s.ctrl.loaded[key] = kids
	return kids
}

func nodeKey(parent string, n TreeNode) string {
	if n.Key != "" {
		return n.Key
	}
	return parent + "/" + n.Label
}

// flatten lists the visible rows in order.
func (s *treeState) flatten() {
	s.rows = s.rows[:0]
	var walk func(nodes []TreeNode, parentKey string, parent, depth int)
	walk = func(nodes []TreeNode, parentKey string, parent, depth int) {
		for _, n := range nodes {
			key := nodeKey(parentKey, n)
			s.rows = append(s.rows, treeRow{node: n, key: key, parent: parent, depth: depth})
			if n.Expandable() && s.expanded(n, key) {
				walk(s.children(n, key), key, len(s.rows)-1, depth+1)
			}
		}
	}
	walk(s.tv().Roots, "", -1, 0)
}

func (s *treeState) indexOf(key string) int {
	for i, r := range s.rows {
		if r.key == key {
			return i
		}
	}
	return -1
}

func (s *treeState) toggle(i int) {
	r := s.rows[i]
	if !r.node.Expandable() {
		return
	}
	open := !s.expanded(r.node, r.key)
	s.ctrl.SetExpanded(r.key, open)
	if cb := s.tv().OnToggle; cb != nil {
		cb(r.node, open)
	}
}

func (s *treeState) selectRow(i int) {
	if i < 0 || i >= len(s.rows) {
		return
	}
	r := s.rows[i]
	if s.ctrl.selected == r.key {
		return
	}
	s.ctrl.Select(r.key)
	if cb := s.tv().OnSelect; cb != nil {
		cb(r.node)
	}
}

func (s *treeState) activate(i int) {
	r := s.rows[i]
	if cb := s.tv().OnActivate; cb != nil {
		cb(r.node)
	}
	if r.node.Expandable() {
		s.toggle(i)
	}
}

func (s *treeState) rowHeight() float32 {
	if h := s.tv().RowHeight; h > 0 {
		return h
	}
	return 28
}

// reveal scrolls row i into view.
func (s *treeState) reveal(i int) {
	if s.tv().ShrinkWrap {
		return
	}
	h := s.rowHeight()
	top, bottom := float32(i)*h, float32(i+1)*h
	off, view := s.scroll.Offset(), s.scroll.ViewportExtent()
	switch {
	case top < off:
		s.scroll.JumpTo(top)
	case view > 0 && bottom > off+view:
		s.scroll.JumpTo(bottom - view)
	}
}

func (s *treeState) move(i int) {
	i = max(0, min(i, len(s.rows)-1))
	s.selectRow(i)
	s.reveal(i)
}

func (s *treeState) onKey(e KeyEvent) bool {
	if len(s.rows) == 0 {
		return false
	}
	cur := s.indexOf(s.ctrl.selected)
	page := 10
	if view := s.scroll.ViewportExtent(); view > 0 {
		page = max(1, int(view/s.rowHeight())-1)
	}
	switch e.Key {
	case KeyDown:
		s.move(cur + 1)
	case KeyUp:
		if cur < 0 {
			cur = len(s.rows)
		}
		s.move(cur - 1)
	case KeyHome:
		s.move(0)
	case KeyEnd:
		s.move(len(s.rows) - 1)
	case KeyPageDown:
		s.move(cur + page)
	case KeyPageUp:
		s.move(cur - page)
	case KeyRight:
		if cur < 0 {
			s.move(0)
			break
		}
		r := s.rows[cur]
		if !r.node.Expandable() {
			break
		}
		if !s.expanded(r.node, r.key) {
			s.toggle(cur)
		} else if cur+1 < len(s.rows) && s.rows[cur+1].parent == cur {
			s.move(cur + 1)
		}
	case KeyLeft:
		if cur < 0 {
			break
		}
		r := s.rows[cur]
		if r.node.Expandable() && s.expanded(r.node, r.key) {
			s.toggle(cur)
		} else if r.parent >= 0 {
			s.move(r.parent)
		}
	case KeyEnter:
		if cur >= 0 {
			s.activate(cur)
		}
	case KeySpace:
		if cur >= 0 {
			s.toggle(cur)
		}
	default:
		return false
	}
	return true
}

func (s *treeState) onTap(i int) {
	if i >= len(s.rows) {
		return
	}
	s.node.RequestFocus()
	r := s.rows[i]
	now := s.Context().Owner().now()
	double := r.key == s.lastTapKey && now.Sub(s.lastTapTime) < 400*time.Millisecond
	s.lastTapKey, s.lastTapTime = r.key, now
	if double {
		s.lastTapKey = "" // a third click starts over
		if cb := s.tv().OnActivate; cb != nil {
			cb(r.node)
		}
		if s.tv().TapSelectsOnly {
			s.toggle(i)
		}
		return
	}
	s.selectRow(i)
	if !s.tv().TapSelectsOnly {
		s.toggle(i)
	}
}

func (s *treeState) Build(ctx BuildContext) Widget {
	tv := s.tv()
	s.flatten()
	if s.hovered >= len(s.rows) {
		s.hovered = -1
	}
	h := s.rowHeight()
	build := func(ctx BuildContext, i int) Widget { return s.buildRow(ctx, i) }
	var body Widget
	if tv.ShrinkWrap {
		kids := make([]Widget, len(s.rows))
		for i := range s.rows {
			kids[i] = KeyedSubtree{ID: i, Child: SizedBox{Height: h, Child: build(ctx, i)}}
		}
		body = Column{Cross: CrossStretch, ShrinkMain: true, Children: kids}
	} else {
		body = ListViewBuilder{Controller: s.scroll, ItemCount: len(s.rows), ItemExtent: h, Builder: build, ThumbColor: tv.ThumbColor}
	}
	return Focus{Node: s.node, Child: body}
}

func (s *treeState) buildRow(ctx BuildContext, i int) Widget {
	tv := s.tv()
	r := s.rows[i]
	expandable := r.node.Expandable()
	row := TreeRow{
		Node: r.node, Key: r.key, Depth: r.depth, Index: i,
		Expandable: expandable, Expanded: expandable && s.expanded(r.node, r.key),
		Selected: r.key == s.ctrl.selected, Hovered: i == s.hovered, Focused: s.focused,
		Toggle: func() { s.toggle(i) },
	}
	var content Widget
	if tv.Row != nil {
		content = tv.Row(ctx, row)
	} else {
		content = s.defaultRow(row)
	}
	return MouseRegion{
		OnEnter: func(PointerEvent) { s.SetState(func() { s.hovered = i }) },
		OnExit: func(PointerEvent) {
			s.SetState(func() {
				if s.hovered == i {
					s.hovered = -1
				}
			})
		},
		Child: GestureDetector{OnTap: func() { s.onTap(i) }, Child: content},
	}
}

var (
	treeChevronRight = vector.NewIcon("chevron_right", 24, "M10 6 8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z")
	treeExpandMore   = vector.NewIcon("expand_more", 24, "M16.59 8.59 12 13.17 7.41 8.59 6 10l6 6 6-6z")
)

const treeChevronW = 20

func (s *treeState) defaultRow(row TreeRow) Widget {
	tv := s.tv()
	st := tv.Style
	indent := tv.Indent
	if indent == 0 {
		indent = 16
	}
	txt := st.Text
	if row.Selected && st.SelectedText != (geom.Color{}) {
		txt.Color = st.SelectedText
	}
	baseCol := st.Text.Color
	if baseCol == (geom.Color{}) {
		baseCol = geom.Black
	}
	iconCol := st.IconColor
	if iconCol == (geom.Color{}) {
		iconCol = baseCol.WithAlpha(baseCol.A * 0.7)
	}
	if row.Selected && st.SelectedText != (geom.Color{}) {
		iconCol = st.SelectedText
	}
	sel := st.SelectedColor
	if sel == (geom.Color{}) {
		sel = geom.Hex(0x1A73E8).WithAlpha(0.16)
	}
	hover := st.HoverColor
	if hover == (geom.Color{}) {
		hover = baseCol.WithAlpha(0.06)
	}
	inset := st.Inset
	lead := inset + 4 // left edge of level 0

	kids := []Widget{SizedBox{Width: lead + float32(row.Depth)*indent}}
	if row.Expandable {
		ic := treeChevronRight
		if row.Expanded {
			ic = treeExpandMore
		}
		var chev Widget = Icon{Icon: ic, Size: 18, Color: iconCol}
		chev = SizedBox{Width: treeChevronW, Child: Center{Child: chev}}
		if tv.TapSelectsOnly {
			// The arrow toggles on its own when a click only selects.
			chev = GestureDetector{OnTap: func() { s.node.RequestFocus(); row.Toggle() }, Child: chev}
		}
		kids = append(kids, chev)
	} else {
		kids = append(kids, SizedBox{Width: treeChevronW})
	}
	ic := row.Node.Icon
	if row.Expanded && row.Node.ExpandedIcon != nil {
		ic = row.Node.ExpandedIcon
	}
	if ic != nil {
		c := iconCol
		if row.Node.IconColor != (geom.Color{}) && !(row.Selected && st.SelectedText != (geom.Color{})) {
			c = row.Node.IconColor
		}
		kids = append(kids, SizedBox{Width: 2}, Icon{Icon: ic, Size: 18, Color: c}, SizedBox{Width: 6})
	} else {
		kids = append(kids, SizedBox{Width: 4})
	}
	kids = append(kids, Expanded{Child: Text{Text: row.Node.Label, Style: txt, MaxLines: 1, Ellipsis: true}}, SizedBox{Width: 8 + inset})

	depth, selected, hovered, focused := row.Depth, row.Selected, row.Hovered, row.Focused
	return CustomPaint{
		Painter: func(c *render.Canvas, o geom.Offset, size geom.Size) {
			r := geom.Rect{X: o.X + inset, Y: o.Y, W: size.W - 2*inset, H: size.H}
			if st.GuideColor != (geom.Color{}) {
				for l := 0; l < depth; l++ {
					x := o.X + lead + float32(l)*indent + treeChevronW/2
					c.FillRect(geom.Rect{X: round(x), Y: o.Y, W: 1, H: size.H}, st.GuideColor)
				}
			}
			if selected {
				c.FillRoundRect(r, st.Radius, sel)
				if focused && st.FocusColor != (geom.Color{}) {
					c.StrokeRoundRect(r, st.Radius, 1, st.FocusColor)
				}
			} else if hovered {
				c.FillRoundRect(r, st.Radius, hover)
			}
		},
		Child: Row{Cross: CrossCenter, Children: kids},
	}
}

func round(v float32) float32 { return float32(int(v + 0.5)) }
