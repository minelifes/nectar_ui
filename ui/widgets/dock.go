package widgets

import (
	"encoding/json"
	"slices"
	"strconv"
	"sync"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// DockPanel is content that can live in a Dock: a file editor, a tool
// window. Panels are identified by ID in the layout.
type DockPanel struct {
	ID       string
	Title    string
	Closable bool
	// Build makes the panel's content. Panels in background tabs stay
	// built (offstage), so their state survives tab switches.
	Build func(ctx BuildContext) Widget
}

// DockNode is one node of a dock layout: a split (Children) or a tab group
// (Tabs). Layouts are plain data: save them with json.Marshal and restore
// them with DockController.SetLayout.
type DockNode struct {
	// Split: children side by side (stacked when Vertical) with Sizes.
	Vertical bool        `json:"vertical,omitempty"`
	Children []*DockNode `json:"children,omitempty"`
	Sizes    []float32   `json:"sizes,omitempty"`
	// Tab group: panel IDs in tab order and the selected one.
	Tabs   []string `json:"tabs,omitempty"`
	Active string   `json:"active,omitempty"`
	// ID identifies a group while it lives (assigned automatically).
	ID string `json:"id,omitempty"`
}

// IsGroup reports whether n is a tab group.
func (n *DockNode) IsGroup() bool { return len(n.Children) == 0 }

func (n *DockNode) clone() *DockNode {
	if n == nil {
		return nil
	}
	c := &DockNode{Vertical: n.Vertical, Active: n.Active, ID: n.ID,
		Sizes: slices.Clone(n.Sizes), Tabs: slices.Clone(n.Tabs)}
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.clone())
	}
	return c
}

// DropZone is where a panel goes relative to a target.
type DropZone uint8

const (
	DropCenter DropZone = iota // into the target's group
	DropLeft                   // a new group left of the target's group
	DropRight
	DropTop
	DropBottom
)

// DockController owns a dock's layout: which panels are open, in which
// groups and splits. It's a Listenable and safe for concurrent use.
type DockController struct {
	mu        sync.Mutex
	groups    int // last group ID
	root      *DockNode
	listeners map[int]func()
	next      int
}

// NewDockController starts with layout (nil = one empty group).
func NewDockController(layout *DockNode) *DockController {
	c := &DockController{}
	c.SetLayout(layout)
	return c
}

// Subscribe implements Listenable.
func (c *DockController) Subscribe(fn func()) func() {
	c.mu.Lock()
	if c.listeners == nil {
		c.listeners = map[int]func(){}
	}
	id := c.next
	c.next++
	c.listeners[id] = fn
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.listeners, id); c.mu.Unlock() }
}

func (c *DockController) changed() {
	c.mu.Lock()
	fns := make([]func(), 0, len(c.listeners))
	for _, fn := range c.listeners {
		fns = append(fns, fn)
	}
	c.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// Layout returns a copy of the current layout.
func (c *DockController) Layout() *DockNode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.root.clone()
}

// SetLayout replaces the layout (a copy of n is kept).
func (c *DockController) SetLayout(n *DockNode) {
	c.mu.Lock()
	if n == nil {
		n = &DockNode{}
	}
	c.root = c.normalize(n.clone())
	c.mu.Unlock()
	c.changed()
}

// MarshalJSON saves the layout.
func (c *DockController) MarshalJSON() ([]byte, error) { return json.Marshal(c.Layout()) }

// UnmarshalJSON restores a saved layout.
func (c *DockController) UnmarshalJSON(data []byte) error {
	var n DockNode
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	c.SetLayout(&n)
	return nil
}

func (c *DockController) update(fn func(root *DockNode) bool) bool {
	c.mu.Lock()
	ok := fn(c.root)
	if ok {
		c.root = c.normalize(c.root)
	}
	c.mu.Unlock()
	if ok {
		c.changed()
	}
	return ok
}

// groupOf returns the group holding panel id and its parent chain.
func groupOf(root *DockNode, id string) (g *DockNode, path []*DockNode) {
	var walk func(n *DockNode, p []*DockNode) bool
	walk = func(n *DockNode, p []*DockNode) bool {
		if n.IsGroup() {
			if slices.Contains(n.Tabs, id) {
				g, path = n, p
				return true
			}
			return false
		}
		for _, ch := range n.Children {
			if walk(ch, append(p, n)) {
				return true
			}
		}
		return false
	}
	walk(root, nil)
	return g, path
}

func firstGroup(n *DockNode) *DockNode {
	for !n.IsGroup() {
		n = n.Children[0]
	}
	return n
}

// IsOpen reports whether panel id is in the layout.
func (c *DockController) IsOpen(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, _ := groupOf(c.root, id)
	return g != nil
}

// Panels returns the IDs of the open panels.
func (c *DockController) Panels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	var walk func(n *DockNode)
	walk = func(n *DockNode) {
		out = append(out, n.Tabs...)
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(c.root)
	return out
}

// Open shows panel id: it's selected if already open, else added next to
// the panel beside ("" = the first group) at zone.
func (c *DockController) Open(id, beside string, zone DropZone) {
	c.update(func(root *DockNode) bool {
		if g, _ := groupOf(root, id); g != nil {
			g.Active = id
			return true
		}
		place(root, id, beside, zone, -1)
		return true
	})
}

// Activate selects panel id in its group.
func (c *DockController) Activate(id string) {
	c.update(func(root *DockNode) bool {
		g, _ := groupOf(root, id)
		if g == nil || g.Active == id {
			return false
		}
		g.Active = id
		return true
	})
}

// Close removes panel id from the layout.
func (c *DockController) Close(id string) {
	c.update(func(root *DockNode) bool { return remove(root, id) })
}

// Move takes panel id to zone of the group holding target: DropCenter
// inserts it before target in that group (target "" or id itself: at the
// end), the edges split the group and put id in a new group there.
func (c *DockController) Move(id, target string, zone DropZone) {
	c.update(func(root *DockNode) bool {
		if g, _ := groupOf(root, id); g == nil || (id == target && zone != DropCenter) {
			return false
		}
		if zone != DropCenter {
			if tg, _ := groupOf(root, target); tg != nil && len(tg.Tabs) == 1 && tg.Tabs[0] == id {
				return false // splitting a group off itself
			}
		}
		idx := -1
		if zone == DropCenter && target != id {
			if tg, _ := groupOf(root, target); tg != nil {
				idx = slices.Index(tg.Tabs, target)
				// Removing id first shifts the index when it sits before
				// target in the same group.
				if j := slices.Index(tg.Tabs, id); j >= 0 && j < idx {
					idx--
				}
			}
		}
		remove(root, id)
		place(root, id, target, zone, idx)
		return true
	})
}

// MoveToGroup takes panel id to the end of the group holding member.
func (c *DockController) MoveToGroup(id, member string) {
	c.update(func(root *DockNode) bool {
		g, _ := groupOf(root, member)
		if g == nil || (len(g.Tabs) > 0 && g.Tabs[len(g.Tabs)-1] == id) {
			return false
		}
		if src, _ := groupOf(root, id); src == nil {
			return false
		}
		remove(root, id)
		if g, _ = groupOf(root, member); g == nil {
			g = firstGroup(root)
		}
		g.Tabs = append(g.Tabs, id)
		g.Active = id
		return true
	})
}

// remove takes panel id out of its group.
func remove(root *DockNode, id string) bool {
	g, _ := groupOf(root, id)
	if g == nil {
		return false
	}
	i := slices.Index(g.Tabs, id)
	g.Tabs = slices.Delete(g.Tabs, i, i+1)
	if g.Active == id {
		g.Active = ""
		if len(g.Tabs) > 0 {
			g.Active = g.Tabs[min(i, len(g.Tabs)-1)]
		}
	}
	return true
}

// place adds id at zone of the group holding target (or the first group).
// idx is the tab position for DropCenter (-1 = the end).
func place(root *DockNode, id, target string, zone DropZone, idx int) {
	g, path := groupOf(root, target)
	if g == nil {
		g, path = firstGroup(root), nil
		if !root.IsGroup() {
			// Find the path to that first group.
			for n := root; !n.IsGroup(); n = n.Children[0] {
				path = append(path, n)
			}
		}
	}
	if zone == DropCenter || len(g.Tabs) == 0 {
		if idx < 0 || idx > len(g.Tabs) {
			idx = len(g.Tabs)
		}
		g.Tabs = slices.Insert(g.Tabs, idx, id)
		g.Active = id
		return
	}
	vertical := zone == DropTop || zone == DropBottom
	before := zone == DropLeft || zone == DropTop
	ng := &DockNode{Tabs: []string{id}, Active: id}
	var parent *DockNode
	if len(path) > 0 {
		parent = path[len(path)-1]
	}
	if parent != nil && parent.Vertical == vertical {
		// Add a sibling in the existing split, sharing g's size.
		i := slices.Index(parent.Children, g)
		at := i + 1
		if before {
			at = i
		}
		parent.Children = slices.Insert(parent.Children, at, ng)
		if len(parent.Sizes) == len(parent.Children)-1 {
			half := parent.Sizes[i] / 2
			parent.Sizes[i] = half
			parent.Sizes = slices.Insert(parent.Sizes, at, half)
		} else {
			parent.Sizes = nil
		}
		return
	}
	// Replace g by a split of g and the new group (in place, so the
	// parent's pointer stays valid).
	old := &DockNode{Tabs: g.Tabs, Active: g.Active, ID: g.ID}
	kids := []*DockNode{old, ng}
	if before {
		kids = []*DockNode{ng, old}
	}
	*g = DockNode{Vertical: vertical, Children: kids}
}

// normalize cleans the tree up and gives new groups IDs.
func (c *DockController) normalize(root *DockNode) *DockNode {
	root = normalize(root, true)
	var walk func(n *DockNode)
	walk = func(n *DockNode) {
		if n.IsGroup() && n.ID == "" {
			c.groups++
			n.ID = "g" + strconv.Itoa(c.groups)
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(root)
	return root
}

// normalize drops empty groups and one-child splits, and merges nested
// splits of the same direction.
func normalize(n *DockNode, isRoot bool) *DockNode {
	if n.IsGroup() {
		if n.Active == "" && len(n.Tabs) > 0 || n.Active != "" && !slices.Contains(n.Tabs, n.Active) {
			n.Active = ""
			if len(n.Tabs) > 0 {
				n.Active = n.Tabs[0]
			}
		}
		if len(n.Tabs) == 0 && !isRoot {
			return nil
		}
		n.Children, n.Sizes, n.Vertical = nil, nil, false
		return n
	}
	var kids []*DockNode
	var sizes []float32
	keep := len(n.Sizes) == len(n.Children)
	for i, ch := range n.Children {
		ch = normalize(ch, false)
		if ch == nil {
			continue
		}
		if !ch.IsGroup() && ch.Vertical == n.Vertical {
			kids = append(kids, ch.Children...)
			if keep && len(ch.Sizes) == len(ch.Children) {
				var tot float32
				for _, s := range ch.Sizes {
					tot += s
				}
				for _, s := range ch.Sizes {
					if tot > 0 {
						sizes = append(sizes, n.Sizes[i]*s/tot)
					} else {
						sizes = append(sizes, 0)
					}
				}
			} else {
				keep = false
			}
			continue
		}
		kids = append(kids, ch)
		if keep {
			sizes = append(sizes, n.Sizes[i])
		}
	}
	switch len(kids) {
	case 0:
		if isRoot {
			return &DockNode{}
		}
		return nil
	case 1:
		return kids[0]
	}
	n.Children = kids
	n.Sizes = nil
	if keep && len(sizes) == len(kids) {
		n.Sizes = sizes
	}
	n.Tabs, n.Active, n.ID = nil, "", ""
	return n
}

// ---------------------------------------------------------------------------
// Dock widget

// DockStyle colors a Dock. Zero colors aren't painted.
type DockStyle struct {
	TabBarColor     geom.Color
	TabColor        geom.Color // inactive tabs
	ActiveTabColor  geom.Color
	TabTextStyle    text.Style
	ActiveTextStyle text.Style
	IndicatorColor  geom.Color // line under the active tab
	DividerColor    geom.Color // between groups
	DropHintColor   geom.Color // where a dragged tab will land
	TabHeight       float32    // 0 = 32
	TabPadding      geom.EdgeInsets
}

// DockTab is what a custom tab builder gets.
type DockTab struct {
	Panel  DockPanel
	Active bool
	Close  func() // nil if the panel isn't closable
}

// Dock arranges panels in tab groups and resizable splits, IDE style:
// click a tab to select it, drag tabs to reorder them, move them to
// another group, or drop them on the edge of a group to split it. The
// layout lives in the Controller (save and restore it as JSON).
type Dock struct {
	Controller *DockController
	Panels     []DockPanel
	Style      DockStyle
	// Tab draws a tab; nil = the default text tab.
	Tab func(ctx BuildContext, t DockTab) Widget
	// OnClose is called after a panel is closed from its tab.
	OnClose func(id string)
	// Empty is shown in a group without panels.
	Empty Widget
}

// dockDrag is the data of a tab being dragged.
type dockDrag struct {
	ctrl *DockController
	id   string
}

func (d Dock) Build(ctx BuildContext) Widget {
	Listen(ctx, d.Controller)
	panels := make(map[string]DockPanel, len(d.Panels))
	for _, p := range d.Panels {
		panels[p.ID] = p
	}
	return d.node(ctx, d.Controller.Layout(), panels, nil)
}

func (d Dock) node(ctx BuildContext, n *DockNode, panels map[string]DockPanel, path []int) Widget {
	if n.IsGroup() {
		return KeyedSubtree{ID: "g:" + n.ID, Child: dockGroup{dock: d, group: n, panels: panels}}
	}
	panes := make([]Pane, len(n.Children))
	for i, ch := range n.Children {
		p := Pane{Child: d.node(ctx, ch, panels, append(slices.Clone(path), i)), Min: 60}
		if i < len(n.Sizes) {
			p.Size = n.Sizes[i]
		}
		panes[i] = p
	}
	ctrl := d.Controller
	key := "s:" + groupKeys(n)
	return KeyedSubtree{ID: key, Child: SplitView{Panes: panes, Vertical: n.Vertical, DividerColor: d.Style.DividerColor,
		OnResize: func(s []float32) { ctrl.setSizes(path, s) }}}
}

// groupKeys names a split by the groups in it, so it keeps its state (the
// dragged sizes) while they stay.
func groupKeys(n *DockNode) string {
	if n.IsGroup() {
		return n.ID
	}
	k := "("
	for _, ch := range n.Children {
		k += groupKeys(ch) + ","
	}
	return k + ")"
}

// setSizes records the pane sizes of the split at path (child indices from
// the root) after the user drags a divider. The dock doesn't rebuild: the
// SplitView already shows them; Layout() returns them for saving.
func (c *DockController) setSizes(path []int, sizes []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.root
	for _, i := range path {
		if n.IsGroup() || i >= len(n.Children) {
			return
		}
		n = n.Children[i]
	}
	if !n.IsGroup() && len(sizes) == len(n.Children) {
		n.Sizes = slices.Clone(sizes)
	}
}

// dockGroup is one tab group: a tab strip over the active panel.
type dockGroup struct {
	dock   Dock
	group  *DockNode
	panels map[string]DockPanel
}

func (dockGroup) CreateState() State { return &dockGroupState{} }

type dockGroupState struct {
	StateBase
	hint    DropZone
	hinting bool
}

func (s *dockGroupState) Build(ctx BuildContext) Widget {
	w := WidgetOf[dockGroup](s)
	d, g := w.dock, w.group
	st := d.Style
	tabH := st.TabHeight
	if tabH <= 0 {
		tabH = 32
	}
	ctrl := d.Controller
	accepts := func(data any) bool {
		dd, ok := data.(dockDrag)
		return ok && dd.ctrl == ctrl
	}
	// Tabs.
	tabs := make([]Widget, 0, len(g.Tabs))
	for _, id := range g.Tabs {
		p, ok := w.panels[id]
		if !ok {
			p = DockPanel{ID: id, Title: id}
		}
		t := DockTab{Panel: p, Active: id == g.Active}
		if p.Closable {
			id := id
			t.Close = func() {
				ctrl.Close(id)
				if d.OnClose != nil {
					d.OnClose(id)
				}
			}
		}
		var look Widget
		if d.Tab != nil {
			look = d.Tab(ctx, t)
		} else {
			look = defaultDockTab(t, st, tabH)
		}
		id := id
		tabs = append(tabs, KeyedSubtree{ID: id, Child: DragTarget{
			OnWillAccept: accepts,
			OnAccept:     func(data any, _ geom.Offset) { ctrl.Move(data.(dockDrag).id, id, DropCenter) },
			Builder: func(ctx BuildContext, cand any) Widget {
				// The same structure with or without a candidate: the
				// Draggable must keep its place (it may be the one being
				// dragged over its own tab).
				bar := geom.Color{}
				if cand != nil {
					bar = st.DropHintColor // where the tab will be inserted
				}
				return Stack{Children: []Widget{
					Draggable{Data: dockDrag{ctrl, id}, Child: GestureDetector{OnTap: func() { ctrl.Activate(id) }, Child: look}},
					Positioned{Left: At(0), Top: At(0), Bottom: At(0), Width: At(2), Child: DecoratedBox{Color: bar}},
				}}
			},
		}})
	}
	strip := DecoratedBox{Color: st.TabBarColor, Child: SizedBox{Height: tabH, Child: Stack{Expand: true, Children: []Widget{
		// The empty part of the strip (under the tabs): drop there to move
		// a tab to the end of the group.
		PositionedFill(DragTarget{
			OnWillAccept: accepts,
			OnAccept: func(data any, _ geom.Offset) {
				if len(g.Tabs) > 0 {
					ctrl.MoveToGroup(data.(dockDrag).id, g.Tabs[0])
				}
			},
			Builder: func(BuildContext, any) Widget { return SizedBox{} },
		}),
		PositionedFill(Align{Alignment: geom.CenterLeft, Child: ScrollView{Horizontal: true, Child: Row{Cross: CrossStretch, Children: tabs}}}),
	}}}}

	// Content: every panel stays built; only the active one is onstage.
	layers := make([]Widget, 0, len(g.Tabs)+2)
	for _, id := range g.Tabs {
		var content Widget = SizedBox{}
		if p, ok := w.panels[id]; ok && p.Build != nil {
			content = Builder{Builder: p.Build}
		}
		layers = append(layers, KeyedSubtree{ID: id, Child: PositionedFill(Offstage{Offstage: id != g.Active, Child: content})})
	}
	if len(g.Tabs) == 0 && d.Empty != nil {
		layers = append(layers, KeyedSubtree{ID: "empty", Child: PositionedFill(d.Empty)})
	}
	target := g.Active
	if target == "" && len(g.Tabs) > 0 {
		target = g.Tabs[0]
	}
	layers = append(layers, KeyedSubtree{ID: "drop", Child: PositionedFill(DragTarget{
		OnWillAccept: accepts,
		OnMove: func(_ any, local geom.Offset) {
			if z := s.zoneAt(local); !s.hinting || z != s.hint {
				s.SetState(func() { s.hint, s.hinting = z, true })
			}
		},
		OnLeave: func(any) { s.SetState(func() { s.hinting = false }) },
		OnAccept: func(data any, local geom.Offset) {
			s.SetState(func() { s.hinting = false })
			id := data.(dockDrag).id
			z := s.zoneAt(local)
			if z == DropCenter {
				if !slices.Contains(g.Tabs, id) && len(g.Tabs) > 0 {
					ctrl.MoveToGroup(id, g.Tabs[0])
				}
				return
			}
			ctrl.Move(id, target, z)
		},
		Builder: func(ctx BuildContext, cand any) Widget {
			if cand == nil || !s.hinting || st.DropHintColor.A <= 0 {
				return SizedBox{}
			}
			return dropHint{zone: s.hint, color: st.DropHintColor}
		},
	})})
	return Column{Cross: CrossStretch, Children: []Widget{
		strip,
		Expanded{Child: Stack{Expand: true, Children: layers}},
	}}
}

// zoneAt picks the drop zone for a point in the content area: the outer
// quarter on each side splits, the middle adds a tab.
func (s *dockGroupState) zoneAt(p geom.Offset) DropZone {
	ro := s.Context().RenderObject()
	if ro == nil {
		return DropCenter
	}
	sz := ro.Base().Size()
	// The content area is below the tab strip.
	tabH := WidgetOf[dockGroup](s).dock.Style.TabHeight
	if tabH <= 0 {
		tabH = 32
	}
	h := max(sz.H-tabH, 1)
	x, y := p.X/max(sz.W, 1), p.Y/h
	switch {
	case x < 0.25 && x <= y && x <= 1-y:
		return DropLeft
	case x > 0.75 && 1-x <= y && 1-x <= 1-y:
		return DropRight
	case y < 0.25:
		return DropTop
	case y > 0.75:
		return DropBottom
	}
	return DropCenter
}

// dropHint shades the part of the group a drop would take.
type dropHint struct {
	zone  DropZone
	color geom.Color
}

func (h dropHint) Build(BuildContext) Widget {
	box := DecoratedBox{Color: h.color}
	half := func(child Widget) Widget {
		return FractionallySizedBox{WidthFactor: 0.5, HeightFactor: 1, Child: child}
	}
	switch h.zone {
	case DropLeft:
		return Align{Alignment: geom.CenterLeft, Child: half(box)}
	case DropRight:
		return Align{Alignment: geom.CenterRight, Child: half(box)}
	case DropTop:
		return Align{Alignment: geom.TopCenter, Child: FractionallySizedBox{WidthFactor: 1, HeightFactor: 0.5, Child: box}}
	case DropBottom:
		return Align{Alignment: geom.BottomCenter, Child: FractionallySizedBox{WidthFactor: 1, HeightFactor: 0.5, Child: box}}
	}
	return box
}

func defaultDockTab(t DockTab, st DockStyle, h float32) Widget {
	bg, ts := st.TabColor, st.TabTextStyle
	if t.Active {
		bg, ts = st.ActiveTabColor, st.ActiveTextStyle.Inherit(st.TabTextStyle)
	}
	pad := st.TabPadding
	if pad.IsZero() {
		pad = geom.InsetsHV(12, 0)
	}
	kids := []Widget{Text{Text: t.Panel.Title, Style: ts, MaxLines: 1}}
	if t.Close != nil {
		kids = append(kids, GestureDetector{OnTap: t.Close, Child: MouseRegion{Cursor: CursorPointer,
			Child: Padding{Padding: geom.InsetsHV(4, 0), Child: Text{Text: "×", Style: ts}}}})
	}
	layers := []Widget{Padding{Padding: pad, Child: Row{Cross: CrossCenter, Spacing: 6, Children: kids}}}
	if t.Active && st.IndicatorColor.A > 0 {
		layers = append(layers, Positioned{Left: At(0), Right: At(0), Bottom: At(0), Height: At(2), Child: DecoratedBox{Color: st.IndicatorColor}})
	}
	return DecoratedBox{Color: bg, Child: SizedBox{Height: h, Child: Stack{Children: layers}}}
}
