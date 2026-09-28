package widgets

import (
	"runtime"
	"slices"

	"github.com/minelifes/nectar_ui/ui/render"
)

// KeyCode identifies a physical key.
type KeyCode uint16

const (
	KeyUnknown KeyCode = iota
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyEscape
	KeyTab
	KeyBackspace
	KeyEnter
	KeySpace
	KeyInsert
	KeyDelete
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
)

// Modifiers are the modifier keys held during a key event.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModControl
	ModAlt
	ModSuper // Cmd on macOS, Windows key elsewhere
)

func (m Modifiers) Shift() bool { return m&ModShift != 0 }
func (m Modifiers) Alt() bool   { return m&ModAlt != 0 }

// Shortcut reports the platform's shortcut modifier (Cmd on macOS, Ctrl
// elsewhere).
func (m Modifiers) Shortcut() bool {
	if runtime.GOOS == "darwin" {
		return m&ModSuper != 0
	}
	return m&ModControl != 0
}

// KeyEvent is a key press (or auto-repeat).
type KeyEvent struct {
	Key  KeyCode
	Mods Modifiers
}

// FocusNode is something that can hold keyboard focus. Key events go to the
// focused node first and bubble up through parent nodes until handled.
type FocusNode struct {
	// OnKey handles a key press; return true to stop bubbling.
	OnKey func(KeyEvent) bool
	// OnText receives typed text (already composed, e.g. "é").
	OnText func(string)
	// OnFocusChange is called when the node gains or loses focus.
	OnFocusChange func(focused bool)
	// SkipTraversal excludes the node from Tab navigation.
	SkipTraversal bool
	// UnfocusOnTapOutside drops focus when the user clicks elsewhere.
	UnfocusOnTapOutside bool
	// CatchAll nodes get unhandled keys even when not focused (the most
	// recently registered first). Navigator uses it for Escape.
	CatchAll bool

	manager *FocusManager
	parent  *FocusNode
	element Element
}

// HasFocus reports whether this node is the focused one.
func (n *FocusNode) HasFocus() bool { return n.manager != nil && n.manager.primary == n }

// RequestFocus focuses the node.
func (n *FocusNode) RequestFocus() {
	if n.manager != nil {
		n.manager.focus(n)
	}
}

// Unfocus removes focus from the node (if it has it).
func (n *FocusNode) Unfocus() {
	if n.HasFocus() {
		n.manager.focus(nil)
	}
}

// FocusManager tracks the focused node and routes keyboard input.
type FocusManager struct {
	owner   *BuildOwner
	primary *FocusNode
	nodes   []*FocusNode

	inPointer        bool
	pointerRequested bool
}

// Focus returns the owner's focus manager.
func (o *BuildOwner) Focus() *FocusManager {
	if o.focus == nil {
		o.focus = &FocusManager{owner: o}
	}
	return o.focus
}

// Primary returns the focused node, or nil.
func (m *FocusManager) Primary() *FocusNode { return m.primary }

func (m *FocusManager) register(n *FocusNode) {
	n.manager = m
	m.nodes = append(m.nodes, n)
}

func (m *FocusManager) unregister(n *FocusNode) {
	if m.primary == n {
		m.primary = nil
	}
	m.nodes = slices.DeleteFunc(m.nodes, func(x *FocusNode) bool { return x == n })
	n.manager = nil
}

func (m *FocusManager) focus(n *FocusNode) {
	if m.inPointer {
		m.pointerRequested = true
	}
	if m.primary == n {
		return
	}
	old := m.primary
	m.primary = n
	if old != nil && old.OnFocusChange != nil {
		old.OnFocusChange(false)
	}
	if n != nil && n.OnFocusChange != nil {
		n.OnFocusChange(true)
	}
	m.owner.requestFrame()
}

// HandleKey routes a key press. Returns whether something handled it.
func (m *FocusManager) HandleKey(e KeyEvent) bool {
	for n := m.primary; n != nil; n = n.parent {
		if n.OnKey != nil && n.OnKey(e) {
			return true
		}
	}
	if e.Key == KeyTab {
		m.traverse(!e.Mods.Shift())
		return true
	}
	for i := len(m.nodes) - 1; i >= 0; i-- {
		if n := m.nodes[i]; n.CatchAll && n.OnKey != nil && n.OnKey(e) {
			return true
		}
	}
	return false
}

// HandleText routes typed text to the focused node.
func (m *FocusManager) HandleText(s string) {
	if n := m.primary; n != nil && n.OnText != nil {
		n.OnText(s)
	}
}

// BeginPointerDown / EndPointerDown bracket a pointer-down dispatch so that
// clicking outside a text field can drop its focus.
func (m *FocusManager) BeginPointerDown() { m.inPointer, m.pointerRequested = true, false }

func (m *FocusManager) EndPointerDown() {
	m.inPointer = false
	if !m.pointerRequested && m.primary != nil && m.primary.UnfocusOnTapOutside {
		m.focus(nil)
	}
}

// traverse moves focus to the next/previous node in reading order.
func (m *FocusManager) traverse(forward bool) {
	type cand struct {
		n    *FocusNode
		x, y float32
	}
	var list []cand
	for _, n := range m.nodes {
		if n.SkipTraversal || n.element == nil {
			continue
		}
		ro := n.element.RenderObject()
		if ro == nil || ro.Base().Owner() == nil {
			continue
		}
		o := render.GlobalOrigin(ro)
		list = append(list, cand{n, o.X, o.Y})
	}
	if len(list) == 0 {
		return
	}
	slices.SortStableFunc(list, func(a, b cand) int {
		if d := a.y - b.y; d < -1 || d > 1 {
			if d < 0 {
				return -1
			}
			return 1
		}
		switch {
		case a.x < b.x:
			return -1
		case a.x > b.x:
			return 1
		}
		return 0
	})
	idx := -1
	for i, c := range list {
		if c.n == m.primary {
			idx = i
		}
	}
	switch {
	case idx < 0 && forward:
		idx = 0
	case idx < 0:
		idx = len(list) - 1
	case forward:
		idx = (idx + 1) % len(list)
	default:
		idx = (idx - 1 + len(list)) % len(list)
	}
	m.focus(list[idx].n)
}

// ---------------------------------------------------------------------------
// Focus widget

// Focus makes its subtree focusable. Pass a Node to control it from outside,
// or let the widget create one and use OnKey/OnFocusChange.
type Focus struct {
	Node          *FocusNode
	Autofocus     bool
	OnKey         func(KeyEvent) bool
	OnFocusChange func(bool)
	Child         Widget
}

func (Focus) CreateState() State { return &focusState{} }

type focusState struct {
	StateBase
	own  *FocusNode
	node *FocusNode
}

func (s *focusState) InitState() {
	w := WidgetOf[Focus](s)
	s.attach(w)
	if w.Autofocus {
		s.node.RequestFocus()
	}
}

func (s *focusState) attach(w Focus) {
	if w.Node != nil {
		s.node = w.Node
	} else {
		if s.own == nil {
			s.own = &FocusNode{}
		}
		s.node = s.own
	}
	if w.OnKey != nil {
		s.node.OnKey = w.OnKey
	}
	if w.OnFocusChange != nil {
		s.node.OnFocusChange = w.OnFocusChange
	}
	if s.node.manager == nil {
		s.Context().Owner().Focus().register(s.node)
	}
	s.node.element = s.element
	if p, ok := DependOn[focusScope](s.Context()); ok {
		s.node.parent = p.node
	} else {
		s.node.parent = nil
	}
}

func (s *focusState) DidUpdateWidget(old Widget) {
	w := WidgetOf[Focus](s)
	if o := old.(Focus); o.Node != w.Node && s.node.manager != nil {
		s.node.manager.unregister(s.node)
	}
	s.attach(w)
}

func (s *focusState) Dispose() {
	if s.node != nil && s.node.manager != nil {
		s.node.manager.unregister(s.node)
	}
}

func (s *focusState) Build(BuildContext) Widget {
	return focusScope{node: s.node, child: WidgetOf[Focus](s).Child}
}

// focusScope tells descendant Focus widgets who their parent node is.
type focusScope struct {
	node  *FocusNode
	child Widget
}

func (w focusScope) ChildWidget() Widget                { return w.child }
func (w focusScope) UpdateShouldNotify(old Widget) bool { return old.(focusScope).node != w.node }
