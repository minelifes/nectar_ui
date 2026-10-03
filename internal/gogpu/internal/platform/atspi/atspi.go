//go:build linux

// Package atspi exposes an app's accessible content to Linux screen readers
// (Orca) and other assistive technology through AT-SPI2 over D-Bus.
//
// The app registers with the accessibility bus as an application whose
// children are its windows (frames), whose children are the accessible
// nodes the app describes (platform.AXTree). Assistive technology reads
// them with the org.a11y.atspi.Accessible, Component, Action and Value
// interfaces, and is told about focus and content changes with
// org.a11y.atspi.Event.Object signals.
package atspi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/axtypes"
)

const (
	pathPrefix = "/org/a11y/atspi/accessible/"
	subtree    = dbus.ObjectPath("/org/a11y/atspi/accessible")
	rootPath   = dbus.ObjectPath("/org/a11y/atspi/accessible/root")
	nullPath   = dbus.ObjectPath("/org/a11y/atspi/null")

	ifaceAccessible  = "org.a11y.atspi.Accessible"
	ifaceComponent   = "org.a11y.atspi.Component"
	ifaceAction      = "org.a11y.atspi.Action"
	ifaceValue       = "org.a11y.atspi.Value"
	ifaceApplication = "org.a11y.atspi.Application"
	ifaceProps       = "org.freedesktop.DBus.Properties"
	ifaceEventObject = "org.a11y.atspi.Event.Object"
)

// AT-SPI roles (AtspiRole).
const (
	roleCheckBox    = 7
	roleFrame       = 23
	roleImage       = 27
	roleLabel       = 29
	roleList        = 31
	roleListItem    = 32
	roleMenu        = 33
	roleMenuItem    = 35
	rolePageTab     = 37
	rolePageTabList = 38
	rolePanel       = 39
	rolePassword    = 40
	roleProgressBar = 42
	rolePushButton  = 43
	roleRadioButton = 44
	roleSlider      = 51
	roleText        = 61
	roleToggle      = 62
	roleApplication = 75
	roleEntry       = 79
	roleHeading     = 83
	roleLink        = 88
	roleTreeItem    = 91
)

// AT-SPI states (AtspiStateType).
const (
	stateActive     = 1
	stateChecked    = 4
	stateEditable   = 7
	stateEnabled    = 8
	stateFocusable  = 11
	stateFocused    = 12
	stateSelectable = 22
	stateSelected   = 23
	stateSensitive  = 24
	stateShowing    = 25
	stateSingleLine = 26
	stateVisible    = 30
	stateCheckable  = 41
)

type ref struct {
	Name string
	Path dbus.ObjectPath
}

type window struct {
	id     uint64
	tree   *axtypes.Tree
	action func(uint64)
}

// Bridge is the app's connection to the accessibility bus.
type Bridge struct {
	conn    *dbus.Conn
	appName string
	parent  ref // the desktop, from the registry

	mu      sync.Mutex
	windows map[uint64]*window
	order   []uint64
}

// BusAddress finds the accessibility bus: $AT_SPI_BUS_ADDRESS, else the
// address org.a11y.Bus gives on the session bus at sessionAddress.
func BusAddress(sessionAddress string) (string, error) {
	if a := os.Getenv("AT_SPI_BUS_ADDRESS"); a != "" {
		return a, nil
	}
	if sessionAddress == "" {
		return "", errors.New("atspi: no session bus")
	}
	sess, err := dbus.Connect(sessionAddress)
	if err != nil {
		return "", err
	}
	defer sess.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var addr string
	if err := sess.Object("org.a11y.Bus", "/org/a11y/bus").CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&addr); err != nil {
		return "", err
	}
	if addr == "" {
		return "", errors.New("atspi: no accessibility bus")
	}
	return addr, nil
}

// Connect joins the accessibility bus at address and registers the app.
func Connect(address, appName string) (*Bridge, error) {
	conn, err := dbus.Connect(address)
	if err != nil {
		return nil, err
	}
	b := &Bridge{conn: conn, appName: appName, windows: map[uint64]*window{}}
	b.parent = ref{"", nullPath}
	for _, iface := range []string{ifaceAccessible, ifaceComponent, ifaceAction, ifaceValue, ifaceApplication, ifaceProps} {
		if err := conn.ExportSubtreeMethodTable(b.methods(iface), subtree, iface); err != nil {
			conn.Close()
			return nil, err
		}
	}
	// Register with the registry: it embeds our root under the desktop.
	var parent ref
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	call := conn.Object("org.a11y.atspi.Registry", rootPath).CallWithContext(ctx, "org.a11y.atspi.Socket.Embed", 0, ref{conn.Names()[0], rootPath})
	if call.Err == nil && call.Store(&parent) == nil {
		b.parent = parent
	}
	return b, nil
}

// Close leaves the bus.
func (b *Bridge) Close() error { return b.conn.Close() }

// UniqueName is the bridge's name on the bus.
func (b *Bridge) UniqueName() string { return b.conn.Names()[0] }

// SetWindow publishes the accessible content of a window (nil removes the
// window) and emits the change events assistive technology expects.
func (b *Bridge) SetWindow(id uint64, tree *axtypes.Tree, action func(uint64)) {
	b.mu.Lock()
	old := b.windows[id]
	if tree == nil {
		delete(b.windows, id)
		for i, w := range b.order {
			if w == id {
				b.order = append(b.order[:i], b.order[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
		if old != nil {
			b.emit(rootPath, "ChildrenChanged", "remove", 0, 0, ref{b.UniqueName(), framePath(id)})
		}
		return
	}
	if old == nil {
		b.order = append(b.order, id)
	}
	b.windows[id] = &window{id: id, tree: tree, action: action}
	b.mu.Unlock()

	var prev *axtypes.Tree
	if old != nil {
		prev = old.tree
	}
	b.emitChanges(id, prev, tree)
}

func (b *Bridge) emit(path dbus.ObjectPath, member, detail string, d1, d2 int32, v any) {
	_ = b.conn.Emit(path, ifaceEventObject+"."+member, detail, d1, d2, dbus.MakeVariant(v), map[string]dbus.Variant{})
}

func (b *Bridge) emitChanges(win uint64, prev, cur *axtypes.Tree) {
	if prev == nil {
		b.emit(rootPath, "ChildrenChanged", "add", 0, 0, ref{b.UniqueName(), framePath(win)})
		return
	}
	if !sameStructure(prev, cur) {
		b.emit(framePath(win), "ChildrenChanged", "add", 0, 0, ref{b.UniqueName(), framePath(win)})
	}
	for id, n := range cur.Nodes {
		o, ok := prev.Nodes[id]
		if !ok {
			continue
		}
		p := nodePath(id)
		if o.Name != n.Name {
			b.emit(p, "PropertyChange", "accessible-name", 0, 0, n.Name)
		}
		if o.Value != n.Value {
			b.emit(p, "PropertyChange", "accessible-value", 0, 0, n.Value)
		}
		if o.Checked != n.Checked {
			b.emit(p, "StateChanged", "checked", boolInt(n.Checked), 0, int32(0))
		}
		if o.Selected != n.Selected {
			b.emit(p, "StateChanged", "selected", boolInt(n.Selected), 0, int32(0))
		}
	}
	if prev.Focus != cur.Focus {
		if prev.Focus != 0 {
			b.emit(nodePath(prev.Focus), "StateChanged", "focused", 0, 0, int32(0))
		}
		if cur.Focus != 0 {
			b.emit(nodePath(cur.Focus), "StateChanged", "focused", 1, 0, int32(0))
		}
	}
}

func sameStructure(a, b *axtypes.Tree) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.Roots) != len(b.Roots) {
		return false
	}
	for i := range a.Roots {
		if a.Roots[i] != b.Roots[i] {
			return false
		}
	}
	for id, n := range b.Nodes {
		o, ok := a.Nodes[id]
		if !ok || len(o.Children) != len(n.Children) {
			return false
		}
		for i := range n.Children {
			if o.Children[i] != n.Children[i] {
				return false
			}
		}
	}
	return true
}

func boolInt(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

func framePath(win uint64) dbus.ObjectPath {
	return dbus.ObjectPath(pathPrefix + "w" + strconv.FormatUint(win, 10))
}

func nodePath(id uint64) dbus.ObjectPath {
	return dbus.ObjectPath(pathPrefix + "n" + strconv.FormatUint(id, 10))
}

// object is what a path names: the application, a window frame or a node.
type object struct {
	kind byte // 'a' app, 'w' window, 'n' node
	win  *window
	node *axtypes.Node
}

// resolve finds the object of a path (under b.mu).
func (b *Bridge) resolve(path dbus.ObjectPath) (object, bool) {
	s := string(path)
	if path == rootPath {
		return object{kind: 'a'}, true
	}
	rest := strings.TrimPrefix(s, pathPrefix)
	if rest == s || rest == "" {
		return object{}, false
	}
	id, err := strconv.ParseUint(rest[1:], 10, 64)
	if err != nil {
		return object{}, false
	}
	switch rest[0] {
	case 'w':
		w, ok := b.windows[id]
		return object{kind: 'w', win: w}, ok
	case 'n':
		for _, w := range b.windows {
			if n, ok := w.tree.Nodes[id]; ok {
				return object{kind: 'n', win: w, node: n}, true
			}
		}
	}
	return object{}, false
}

func (b *Bridge) self(path dbus.ObjectPath) ref { return ref{b.UniqueName(), path} }

func (b *Bridge) children(o object) []ref {
	var out []ref
	switch o.kind {
	case 'a':
		for _, id := range b.order {
			out = append(out, b.self(framePath(id)))
		}
	case 'w':
		for _, id := range o.win.tree.Roots {
			out = append(out, b.self(nodePath(id)))
		}
	case 'n':
		for _, id := range o.node.Children {
			out = append(out, b.self(nodePath(id)))
		}
	}
	return out
}

func (b *Bridge) parentOf(o object) ref {
	switch o.kind {
	case 'a':
		return b.parent
	case 'w':
		return b.self(rootPath)
	}
	for id, n := range o.win.tree.Nodes {
		for _, c := range n.Children {
			if c == o.node.ID {
				return b.self(nodePath(id))
			}
		}
	}
	return b.self(framePath(o.win.id))
}

func (b *Bridge) indexInParent(o object) int32 {
	p := b.parentOf(o)
	b2 := p.Path
	po, ok := b.resolve(b2)
	if !ok {
		return -1
	}
	for i, c := range b.children(po) {
		if c.Path == pathOf(o) {
			return int32(i)
		}
	}
	return -1
}

func pathOf(o object) dbus.ObjectPath {
	switch o.kind {
	case 'a':
		return rootPath
	case 'w':
		return framePath(o.win.id)
	}
	return nodePath(o.node.ID)
}

func roleOf(o object) uint32 {
	switch o.kind {
	case 'a':
		return roleApplication
	case 'w':
		return roleFrame
	}
	switch o.node.Role {
	case "button":
		if o.node.Checkable {
			return roleToggle
		}
		return rolePushButton
	case "checkbox":
		return roleCheckBox
	case "radio":
		return roleRadioButton
	case "switch":
		return roleToggle
	case "textfield":
		return roleEntry
	case "password":
		return rolePassword
	case "slider":
		return roleSlider
	case "tab":
		return rolePageTab
	case "tablist":
		return rolePageTabList
	case "menuitem":
		return roleMenuItem
	case "menu":
		return roleMenu
	case "header", "heading":
		return roleHeading
	case "image":
		return roleImage
	case "link":
		return roleLink
	case "text", "label":
		return roleLabel
	case "list":
		return roleList
	case "listitem":
		return roleListItem
	case "treeitem":
		return roleTreeItem
	case "progressbar":
		return roleProgressBar
	}
	return rolePanel
}

var roleNames = map[uint32]string{
	roleCheckBox: "check box", roleFrame: "frame", roleImage: "image", roleLabel: "label", roleList: "list",
	roleListItem: "list item", roleMenu: "menu", roleMenuItem: "menu item", rolePageTab: "page tab",
	rolePageTabList: "page tab list", rolePanel: "panel", rolePassword: "password text", roleProgressBar: "progress bar",
	rolePushButton: "push button", roleRadioButton: "radio button", roleSlider: "slider", roleText: "text",
	roleToggle: "toggle button", roleApplication: "application", roleEntry: "entry", roleHeading: "heading",
	roleLink: "link", roleTreeItem: "tree item",
}

func stateOf(o object) []uint32 {
	var bits uint64
	set := func(s uint) { bits |= 1 << s }
	set(stateVisible)
	set(stateShowing)
	switch o.kind {
	case 'a':
	case 'w':
		set(stateActive)
		set(stateEnabled)
		set(stateSensitive)
	default:
		n := o.node
		if !n.Disabled {
			set(stateEnabled)
			set(stateSensitive)
		}
		if n.Actionable || roleOf(o) == roleEntry {
			set(stateFocusable)
		}
		if n.Focused {
			set(stateFocused)
		}
		if n.Checkable {
			set(stateCheckable)
		}
		if n.Checked {
			set(stateChecked)
		}
		if n.Selected {
			set(stateSelected)
			set(stateSelectable)
		}
		if roleOf(o) == roleEntry {
			set(stateEditable)
			set(stateSingleLine)
		}
	}
	return []uint32{uint32(bits), uint32(bits >> 32)}
}

func (b *Bridge) nameOf(o object) string {
	switch o.kind {
	case 'a':
		return b.appName
	case 'w':
		return o.win.tree.Title
	}
	return o.node.Name
}

func errNoObject(path dbus.ObjectPath) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.UnknownObject", []any{fmt.Sprintf("no accessible object %s", path)})
}

// methods returns the D-Bus methods of one interface; every call finds
// its object by the message's path.
func (b *Bridge) methods(iface string) map[string]any {
	with := func(msg dbus.Message, fn func(o object) *dbus.Error) *dbus.Error {
		path, _ := msg.Headers[dbus.FieldPath].Value().(dbus.ObjectPath)
		b.mu.Lock()
		defer b.mu.Unlock()
		o, ok := b.resolve(path)
		if !ok {
			return errNoObject(path)
		}
		return fn(o)
	}
	switch iface {
	case ifaceAccessible:
		return map[string]any{
			"GetChildAtIndex": func(msg dbus.Message, i int32) (r ref, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error {
					c := b.children(o)
					if i < 0 || int(i) >= len(c) {
						r = ref{"", nullPath}
						return nil
					}
					r = c[i]
					return nil
				})
				return
			},
			"GetChildren": func(msg dbus.Message) (r []ref, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = b.children(o); return nil })
				if r == nil {
					r = []ref{}
				}
				return
			},
			"GetIndexInParent": func(msg dbus.Message) (r int32, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = b.indexInParent(o); return nil })
				return
			},
			"GetRelationSet": func(msg dbus.Message) ([]struct {
				Type    uint32
				Targets []ref
			}, *dbus.Error) {
				return []struct {
					Type    uint32
					Targets []ref
				}{}, nil
			},
			"GetRole": func(msg dbus.Message) (r uint32, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = roleOf(o); return nil })
				return
			},
			"GetRoleName": func(msg dbus.Message) (r string, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = roleNames[roleOf(o)]; return nil })
				return
			},
			"GetLocalizedRoleName": func(msg dbus.Message) (r string, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = roleNames[roleOf(o)]; return nil })
				return
			},
			"GetState": func(msg dbus.Message) (r []uint32, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = stateOf(o); return nil })
				return
			},
			"GetAttributes": func(msg dbus.Message) (map[string]string, *dbus.Error) {
				return map[string]string{"toolkit": b.appName}, nil
			},
			"GetApplication": func(msg dbus.Message) (ref, *dbus.Error) { return b.self(rootPath), nil },
			"GetInterfaces": func(msg dbus.Message) (r []string, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error {
					r = []string{ifaceAccessible, ifaceComponent}
					switch {
					case o.kind == 'a':
						r = []string{ifaceAccessible, ifaceApplication}
					case o.kind == 'n' && o.node.Actionable:
						r = append(r, ifaceAction)
					}
					if o.kind == 'n' && roleOf(o) == roleSlider {
						r = append(r, ifaceValue)
					}
					return nil
				})
				return
			},
		}
	case ifaceComponent:
		ext := func(o object) (x, y, w, h int32) {
			switch o.kind {
			case 'w':
				return 0, 0, int32(o.win.tree.Width), int32(o.win.tree.Height)
			case 'n':
				n := o.node
				return int32(n.X), int32(n.Y), int32(n.W + 0.5), int32(n.H + 0.5)
			}
			return 0, 0, 0, 0
		}
		return map[string]any{
			"GetExtents": func(msg dbus.Message, coordType uint32) (r struct{ X, Y, W, H int32 }, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r.X, r.Y, r.W, r.H = ext(o); return nil })
				return
			},
			"GetPosition": func(msg dbus.Message, coordType uint32) (x, y int32, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { x, y, _, _ = ext(o); return nil })
				return
			},
			"GetSize": func(msg dbus.Message) (w, h int32, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { _, _, w, h = ext(o); return nil })
				return
			},
			"Contains": func(msg dbus.Message, x, y int32, coordType uint32) (r bool, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error {
					ox, oy, w, h := ext(o)
					r = x >= ox && y >= oy && x < ox+w && y < oy+h
					return nil
				})
				return
			},
			"GetAccessibleAtPoint": func(msg dbus.Message, x, y int32, coordType uint32) (r ref, e *dbus.Error) {
				r = ref{"", nullPath}
				e = with(msg, func(o object) *dbus.Error {
					if o.win == nil {
						return nil
					}
					var best *axtypes.Node
					for _, n := range o.win.tree.Nodes {
						if float64(x) >= n.X && float64(y) >= n.Y && float64(x) < n.X+n.W && float64(y) < n.Y+n.H {
							if best == nil || n.W*n.H < best.W*best.H {
								best = n
							}
						}
					}
					if best != nil {
						r = b.self(nodePath(best.ID))
					}
					return nil
				})
				return
			},
			"GetLayer":      func(msg dbus.Message) (uint32, *dbus.Error) { return 3, nil }, // WIDGET
			"GetMDIZOrder":  func(msg dbus.Message) (int16, *dbus.Error) { return 0, nil },
			"GetAlpha":      func(msg dbus.Message) (float64, *dbus.Error) { return 1, nil },
			"GrabFocus":     func(msg dbus.Message) (bool, *dbus.Error) { return false, nil },
			"ScrollTo":      func(msg dbus.Message, t uint32) (bool, *dbus.Error) { return false, nil },
			"ScrollToPoint": func(msg dbus.Message, t uint32, x, y int32) (bool, *dbus.Error) { return false, nil },
		}
	case ifaceAction:
		return map[string]any{
			"GetActions": func(msg dbus.Message) (r []struct{ Name, Desc, Key string }, e *dbus.Error) {
				r = []struct{ Name, Desc, Key string }{}
				e = with(msg, func(o object) *dbus.Error {
					if o.kind == 'n' && o.node.Actionable {
						r = append(r, struct{ Name, Desc, Key string }{"click", "", ""})
					}
					return nil
				})
				return
			},
			"GetName":          func(msg dbus.Message, i int32) (string, *dbus.Error) { return "click", nil },
			"GetLocalizedName": func(msg dbus.Message, i int32) (string, *dbus.Error) { return "click", nil },
			"GetDescription":   func(msg dbus.Message, i int32) (string, *dbus.Error) { return "", nil },
			"GetKeyBinding":    func(msg dbus.Message, i int32) (string, *dbus.Error) { return "", nil },
			"DoAction": func(msg dbus.Message, i int32) (r bool, e *dbus.Error) {
				var act func(uint64)
				var id uint64
				e = with(msg, func(o object) *dbus.Error {
					if o.kind == 'n' && o.node.Actionable && !o.node.Disabled && i == 0 {
						act, id = o.win.action, o.node.ID
					}
					return nil
				})
				if act != nil {
					act(id)
					r = true
				}
				return
			},
		}
	case ifaceValue:
		return map[string]any{
			"SetCurrentValue": func(msg dbus.Message, v float64) *dbus.Error { return nil },
		}
	case ifaceApplication:
		return map[string]any{
			"GetLocale": func(msg dbus.Message, lctype uint32) (string, *dbus.Error) { return os.Getenv("LANG"), nil },
		}
	case ifaceProps:
		return map[string]any{
			"Get": func(msg dbus.Message, iface, prop string) (r dbus.Variant, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error {
					props := b.props(o, iface)
					v, ok := props[prop]
					if !ok {
						return dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{iface + "." + prop})
					}
					r = v
					return nil
				})
				return
			},
			"GetAll": func(msg dbus.Message, iface string) (r map[string]dbus.Variant, e *dbus.Error) {
				e = with(msg, func(o object) *dbus.Error { r = b.props(o, iface); return nil })
				return
			},
			"Set": func(msg dbus.Message, iface, prop string, v dbus.Variant) *dbus.Error { return nil },
		}
	}
	return nil
}

// props returns the properties of o for an interface.
func (b *Bridge) props(o object, iface string) map[string]dbus.Variant {
	v := dbus.MakeVariant
	switch iface {
	case ifaceAccessible:
		desc := ""
		if o.kind == 'n' {
			desc = o.node.Description
		}
		return map[string]dbus.Variant{
			"Name": v(b.nameOf(o)), "Description": v(desc), "Parent": v(b.parentOf(o)),
			"ChildCount": v(int32(len(b.children(o)))), "Locale": v(""), "AccessibleId": v(string(pathOf(o))),
			"HelpText": v(desc),
		}
	case ifaceApplication:
		return map[string]dbus.Variant{"ToolkitName": v(b.appName), "Version": v("1.0"), "AtspiVersion": v("2.1"), "Id": v(int32(0))}
	case ifaceAction:
		n := int32(0)
		if o.kind == 'n' && o.node.Actionable {
			n = 1
		}
		return map[string]dbus.Variant{"NActions": v(n)}
	case ifaceValue:
		cur, _ := 0.0, error(nil)
		if o.kind == 'n' {
			cur, _ = strconv.ParseFloat(o.node.Value, 64)
		}
		return map[string]dbus.Variant{"CurrentValue": v(cur), "MinimumValue": v(0.0), "MaximumValue": v(0.0), "MinimumIncrement": v(0.0),
			"Text": v(nameOrValue(o))}
	}
	return map[string]dbus.Variant{}
}

func nameOrValue(o object) string {
	if o.kind == 'n' {
		return o.node.Value
	}
	return ""
}
