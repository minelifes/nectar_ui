package commands

import (
	"sync"

	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Host runs commands for one window: the global Registry, the user's
// Keymap, the scopes in the widget tree, and the state of a key sequence
// being typed. Put it in the tree with the Commands widget.
type Host struct {
	mvvm.Notifier
	Registry *Registry
	Keymap   *Keymap

	mu      sync.Mutex
	focus   *widgets.FocusManager
	scopes  map[*widgets.FocusNode]*scopeState
	pending Sequence  // chords typed so far of a longer sequence
	cands   []Command // the commands that sequence can still become
	recent  []string  // IDs of recently run commands, newest first
}

// maxRecent is how many recently run commands Recent remembers.
const maxRecent = 20

// Recent returns the IDs of the most recently run commands, newest first.
func (h *Host) Recent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.recent...)
}

// run runs c and remembers it as recently used.
func (h *Host) run(c Command) {
	h.mu.Lock()
	r := []string{c.ID}
	for _, id := range h.recent {
		if id != c.ID && len(r) < maxRecent {
			r = append(r, id)
		}
	}
	h.recent = r
	h.mu.Unlock()
	c.Run()
}

// NewHost returns a host with an empty registry and keymap.
func NewHost() *Host {
	return &Host{Registry: NewRegistry(), Keymap: NewKeymap(), scopes: map[*widgets.FocusNode]*scopeState{}}
}

// Pending returns the chords of an unfinished key sequence (Ctrl+K, waiting
// for the next key); nil when none. A status bar can show it.
func (h *Host) Pending() Sequence {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pending
}

func (h *Host) setPending(seq Sequence, cands []Command) {
	h.mu.Lock()
	h.pending, h.cands = seq, cands
	h.mu.Unlock()
	h.Notify()
}

// Available returns the commands that apply where the focus is now: those
// of the scopes around the focused widget (innermost first) and then the
// global ones. A command ID defined in several places resolves to the
// innermost. Disabled commands are included (menus show them grayed out).
func (h *Host) Available() []Command {
	return h.availableFrom(h.focusedNode())
}

func (h *Host) focusedNode() *widgets.FocusNode {
	h.mu.Lock()
	fm := h.focus
	h.mu.Unlock()
	if fm == nil {
		return nil
	}
	return fm.Primary()
}

// availableFrom lists the commands of the scopes from n outward, then the
// registry's.
func (h *Host) availableFrom(n *widgets.FocusNode) []Command {
	seen := map[string]bool{}
	var out []Command
	add := func(cs []Command) {
		for _, c := range cs {
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c)
			}
		}
	}
	for ; n != nil; n = n.Parent() {
		h.mu.Lock()
		sc := h.scopes[n]
		h.mu.Unlock()
		if sc != nil {
			add(sc.commands())
		}
	}
	add(h.Registry.All())
	return out
}

// Snapshot captures where the focus is now, so a command palette that takes
// the focus can still list and run the commands that applied before it
// opened (see Snapshot.Commands and Snapshot.Run).
func (h *Host) Snapshot() *Snapshot {
	n := h.focusedNode()
	return &Snapshot{host: h, node: n, cmds: h.availableFrom(n)}
}

// Snapshot is the set of commands available at one moment (see
// Host.Snapshot).
type Snapshot struct {
	host *Host
	node *widgets.FocusNode
	cmds []Command
}

// Commands returns the captured commands.
func (s *Snapshot) Commands() []Command { return s.cmds }

// Find returns the captured command with the given ID.
func (s *Snapshot) Find(id string) (Command, bool) {
	for _, c := range s.cmds {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}

// Keys returns the first shortcut of the captured command id (nil if none).
func (s *Snapshot) Keys(id string) Sequence {
	if c, ok := s.Find(id); ok {
		if ks := s.host.Keymap.KeysFor(c); len(ks) > 0 {
			return ks[0]
		}
	}
	return nil
}

// ShortcutLabel returns the first shortcut of the captured command id,
// formatted for a menu; "" if none.
func (s *Snapshot) ShortcutLabel(id string) string {
	if c, ok := s.Find(id); ok {
		if ks := s.host.Keymap.KeysFor(c); len(ks) > 0 {
			return ks[0].String()
		}
	}
	return ""
}

// Run gives the focus back to where it was and runs the command with the
// given ID, if it's enabled. Returns whether it ran.
func (s *Snapshot) Run(id string) bool {
	if s.node != nil {
		s.node.RequestFocus()
	}
	for _, c := range s.cmds {
		if c.ID == id {
			if !c.IsEnabled() {
				return false
			}
			s.host.run(c)
			return true
		}
	}
	return false
}

// Find returns the command with the given ID available at the focus.
func (h *Host) Find(id string) (Command, bool) {
	for _, c := range h.Available() {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}

// Execute runs the command with the given ID available at the focus, if
// it's enabled. Returns whether it ran.
func (h *Host) Execute(id string) bool {
	c, ok := h.Find(id)
	if !ok || !c.IsEnabled() {
		return false
	}
	h.run(c)
	return true
}

// KeysFor returns the effective shortcuts of the command with the given ID.
func (h *Host) KeysFor(id string) []Sequence {
	if c, ok := h.Find(id); ok {
		return h.Keymap.KeysFor(c)
	}
	if c, ok := h.Registry.Get(id); ok {
		return h.Keymap.KeysFor(c)
	}
	return nil
}

// ShortcutLabel returns the first shortcut of the command, formatted for a
// menu ("Ctrl+S", "⌘S"); "" if it has none.
func (h *Host) ShortcutLabel(id string) string {
	if ks := h.KeysFor(id); len(ks) > 0 {
		return ks[0].String()
	}
	return ""
}

// handle tries the key against cmds (a scope's, or the global ones).
func (h *Host) handle(e widgets.KeyEvent, cmds []Command) bool {
	if h.Pending() != nil {
		return false // the interceptor finishes sequences
	}
	return h.match(Sequence{ChordOf(e)}, cmds)
}

// match runs the command bound to seq, or starts waiting for more keys if
// seq begins a longer binding. A longer binding wins over an exact one
// (Ctrl+K vs Ctrl+K Ctrl+S): the sequence must be finished or cancelled.
func (h *Host) match(seq Sequence, cmds []Command) bool {
	var exact *Command
	var cands []Command
	for i := range cmds {
		c := cmds[i]
		if !c.IsEnabled() {
			continue
		}
		for _, k := range h.Keymap.KeysFor(c) {
			switch {
			case k.Equal(seq):
				if exact == nil {
					exact = &cmds[i]
				}
			case k.hasPrefix(seq):
				cands = append(cands, c)
			}
		}
	}
	if len(cands) > 0 {
		if exact != nil {
			cands = append(cands, *exact)
		}
		h.setPending(seq, cands)
		return true
	}
	if exact != nil {
		h.run(*exact)
		return true
	}
	return false
}

// intercept continues a pending sequence: the next chord goes to it before
// anything else. A key that doesn't continue it cancels the sequence and
// is swallowed (Escape just cancels).
func (h *Host) intercept(e widgets.KeyEvent) bool {
	h.mu.Lock()
	pending, cands := h.pending, h.cands
	h.mu.Unlock()
	if pending == nil {
		return false
	}
	if isModifierOnly(e.Key) {
		return true
	}
	h.setPending(nil, nil)
	if e.Key == widgets.KeyEscape && e.Mods == 0 {
		return true
	}
	seq := append(append(Sequence{}, pending...), ChordOf(e))
	h.match(seq, cands)
	return true
}

func isModifierOnly(k widgets.KeyCode) bool { return k == widgets.KeyUnknown }

func (h *Host) attach(fm *widgets.FocusManager) {
	h.mu.Lock()
	h.focus = fm
	h.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Widgets

// Commands puts a Host in the tree: its global commands' shortcuts work
// anywhere below (even with nothing focused), and HostOf finds it.
type Commands struct {
	Host  *Host
	Child widgets.Widget
}

func (Commands) CreateState() widgets.State { return &commandsState{} }

type commandsState struct {
	widgets.StateBase
	node   *widgets.FocusNode
	remove func()
	host   *Host
}

func (s *commandsState) InitState() {
	s.node = &widgets.FocusNode{CatchAll: true, SkipTraversal: true}
	s.node.OnKey = func(e widgets.KeyEvent) bool {
		h := s.host
		return h != nil && h.handle(e, h.Registry.All())
	}
	s.bind()
}

func (s *commandsState) bind() {
	h := widgets.WidgetOf[Commands](s).Host
	if h == s.host {
		return
	}
	if s.remove != nil {
		s.remove()
		s.remove = nil
	}
	s.host = h
	if h != nil {
		fm := s.Context().Owner().Focus()
		h.attach(fm)
		s.remove = fm.AddInterceptor(h.intercept)
	}
}

func (s *commandsState) DidUpdateWidget(widgets.Widget) { s.bind() }

func (s *commandsState) Dispose() {
	if s.remove != nil {
		s.remove()
	}
}

func (s *commandsState) Build(widgets.BuildContext) widgets.Widget {
	return widgets.Focus{Node: s.node, Child: widgets.Provider[*Host]{Value: s.host, Child: widgets.WidgetOf[Commands](s).Child}}
}

// HostOf returns the Host of the nearest Commands widget, or nil.
func HostOf(ctx widgets.BuildContext) *Host {
	h, _ := widgets.Of[*Host](ctx)
	return h
}

// Scope makes Commands available while the focus is inside Child: their
// shortcuts work there (before the global ones), and the command palette
// lists them. Commands are taken from each build, so their closures can
// use the latest state.
type Scope struct {
	Name     string // informational ("editor")
	Commands []Command
	Child    widgets.Widget
}

func (Scope) CreateState() widgets.State { return &scopeState{} }

type scopeState struct {
	widgets.StateBase
	node *widgets.FocusNode
	host *Host
	mu   sync.Mutex
	cmds []Command
}

func (s *scopeState) commands() []Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmds
}

func (s *scopeState) InitState() {
	s.node = &widgets.FocusNode{SkipTraversal: true}
	s.node.OnKey = func(e widgets.KeyEvent) bool {
		return s.host != nil && s.host.handle(e, s.commands())
	}
}

func (s *scopeState) Dispose() {
	if s.host != nil {
		s.host.mu.Lock()
		delete(s.host.scopes, s.node)
		s.host.mu.Unlock()
	}
}

func (s *scopeState) Build(ctx widgets.BuildContext) widgets.Widget {
	sc := widgets.WidgetOf[Scope](s)
	s.mu.Lock()
	s.cmds = sc.Commands
	s.mu.Unlock()
	h := HostOf(ctx)
	if h != s.host {
		s.Dispose()
		s.host = h
		if h != nil {
			h.mu.Lock()
			h.scopes[s.node] = s
			h.mu.Unlock()
		}
	}
	return widgets.Focus{Node: s.node, Child: sc.Child}
}
