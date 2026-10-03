//go:build darwin

package darwin

import (
	"strconv"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"

	"github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/axtypes"
)

// VoiceOver support: each accessible node of a view becomes an
// NSAccessibilityElement (a GoGPUAXElement, which adds the press action),
// configured with the NSAccessibility setters and attached to the view
// with setAccessibilityChildren:. Elements are kept across updates so
// VoiceOver's cursor stays put, and the changes it cares about are posted
// with NSAccessibilityPostNotification.

// axWindowViews maps windows to their content views (main thread only).
var axWindowViews = map[*Window]ID{}

type axView struct {
	tree     *axtypes.Tree
	action   func(uint64)
	elements map[uint64]ID
	focused  ID
}

var ax struct {
	once      sync.Once
	class     Class
	post      unsafe.Pointer
	cifPost   *types.CallInterface
	mu        sync.Mutex
	views     map[ID]*axView
	byElement map[ID]struct {
		view ID
		node uint64
	}
}

func initAX() bool {
	ax.once.Do(func() {
		if initRuntime() != nil {
			return
		}
		initSelectors()
		initClasses()
		ax.views = map[ID]*axView{}
		ax.byElement = map[ID]struct {
			view ID
			node uint64
		}{}
		super := GetClass("NSAccessibilityElement")
		if super == 0 {
			return
		}
		cls := AllocateClassPair(super, "GoGPUAXElement")
		if cls == 0 {
			cls = GetClass("GoGPUAXElement")
		} else {
			// -(BOOL)accessibilityPerformPress
			press := ffi.NewCallback(func(self, _ uintptr) uintptr {
				ax.mu.Lock()
				e, ok := ax.byElement[ID(self)]
				var act func(uint64)
				if ok {
					if v := ax.views[e.view]; v != nil {
						act = v.action
					}
				}
				ax.mu.Unlock()
				if act == nil {
					return 0
				}
				act(e.node)
				return 1
			})
			ClassAddMethod(cls, RegisterSelector("accessibilityPerformPress"), press, "B@:")
			RegisterClassPair(cls)
		}
		ax.class = cls

		// The view reports the focused node as its focused element.
		if vc, err := GoGPUViewClass(); err == nil {
			focused := ffi.NewCallback(func(self, _ uintptr) uintptr {
				ax.mu.Lock()
				defer ax.mu.Unlock()
				if v := ax.views[ID(self)]; v != nil && v.focused != 0 {
					return uintptr(v.focused)
				}
				return self
			})
			ClassAddMethod(vc, RegisterSelector("accessibilityFocusedUIElement"), focused, "@@:")
		}

		if sym, err := ffi.GetSymbol(objcRT.appKit, "NSAccessibilityPostNotification"); err == nil {
			cif := &types.CallInterface{}
			if ffi.PrepareCallInterface(cif, types.DefaultCall, types.VoidTypeDescriptor,
				[]*types.TypeDescriptor{types.PointerTypeDescriptor, types.PointerTypeDescriptor}) == nil {
				ax.post, ax.cifPost = sym, cif
			}
		}
	})
	return ax.class != 0
}

func axPost(element ID, notification string) {
	if ax.post == nil || element == 0 {
		return
	}
	name := NewNSString(notification)
	defer name.Release()
	args := struct{ el, name uintptr }{uintptr(element), uintptr(name.ID())}
	_, _ = ffi.CallFunction(ax.cifPost, ax.post, unsafe.Pointer(new(uintptr)),
		[]unsafe.Pointer{unsafe.Pointer(&args.el), unsafe.Pointer(&args.name)})
}

func axString(obj ID, sel string, s string) {
	str := NewNSString(s)
	obj.SendPtr(RegisterSelector(sel), uintptr(str.ID()))
	str.Release()
}

func axArray(items []ID) ID {
	arr := GetClass("NSArray")
	if len(items) == 0 {
		return ID(arr).Send(RegisterSelector("array"))
	}
	ptrs := make([]uintptr, len(items))
	for i, it := range items {
		ptrs[i] = uintptr(it)
	}
	return msgSend(ID(arr), RegisterSelector("arrayWithObjects:count:"),
		uintptr(unsafe.Pointer(&ptrs[0])), uintptr(len(ptrs)))
}

// axRole maps a node role to an NSAccessibility role and subrole.
func axRole(n *axtypes.Node) (role, subrole string) {
	switch n.Role {
	case "button":
		if n.Checkable {
			return "AXCheckBox", "AXToggle"
		}
		return "AXButton", ""
	case "checkbox":
		return "AXCheckBox", ""
	case "switch":
		return "AXCheckBox", "AXSwitch"
	case "radio", "tab":
		return "AXRadioButton", ""
	case "tablist":
		return "AXTabGroup", ""
	case "textfield":
		return "AXTextField", ""
	case "password":
		return "AXTextField", "AXSecureTextField"
	case "slider":
		return "AXSlider", ""
	case "progressbar":
		return "AXProgressIndicator", ""
	case "menu":
		return "AXMenu", ""
	case "menuitem":
		return "AXMenuItem", ""
	case "header", "heading":
		return "AXHeading", ""
	case "image":
		return "AXImage", ""
	case "link":
		return "AXLink", ""
	case "text", "label":
		return "AXStaticText", ""
	case "list":
		return "AXList", ""
	case "listitem", "treeitem":
		return "AXGroup", ""
	}
	return "AXGroup", ""
}

// SetWindowAccessibility publishes the accessible content of a window's
// content view (nil clears it). action is called, on the main thread,
// when VoiceOver presses a node. The tree must not be modified afterwards.
func SetWindowAccessibility(w *Window, tree *axtypes.Tree, action func(uint64)) {
	if w == nil {
		return
	}
	_ = PerformOnMain(func() {
		// A window being closed isn't messaged: its view is remembered.
		view, ok := axWindowViews[w]
		if tree == nil {
			delete(axWindowViews, w)
		} else if !ok {
			view = w.ContentView()
			axWindowViews[w] = view
		}
		if view != 0 {
			setViewAccessibility(view, tree, action)
		}
	}, false)
}

func setViewAccessibility(view ID, tree *axtypes.Tree, action func(uint64)) {
	if !initAX() {
		return
	}
	ax.mu.Lock()
	v := ax.views[view]
	if tree == nil {
		if v != nil {
			for _, el := range v.elements {
				delete(ax.byElement, el)
				el.Send(selectors.release)
			}
			delete(ax.views, view)
		}
		ax.mu.Unlock()
		view.SendPtr(RegisterSelector("setAccessibilityChildren:"), uintptr(axArray(nil)))
		return
	}
	if v == nil {
		v = &axView{elements: map[uint64]ID{}}
		ax.views[view] = v
	}
	prev := v.tree
	v.tree, v.action = tree, action

	// Create the new nodes' elements and drop the removed ones.
	var created []ID
	for id := range tree.Nodes {
		if _, ok := v.elements[id]; !ok {
			el := ID(ax.class).Send(selectors.alloc).Send(selectors.init)
			if el == 0 {
				continue
			}
			v.elements[id] = el
			ax.byElement[el] = struct {
				view ID
				node uint64
			}{view, id}
			created = append(created, el)
		}
	}
	for id, el := range v.elements {
		if _, ok := tree.Nodes[id]; !ok {
			delete(v.elements, id)
			delete(ax.byElement, el)
			el.Send(selectors.release)
		}
	}
	parents := map[uint64]*axtypes.Node{}
	for _, n := range tree.Nodes {
		for _, c := range n.Children {
			parents[c] = n
		}
	}
	elements := make(map[uint64]ID, len(v.elements))
	for id, el := range v.elements {
		elements[id] = el
	}
	oldFocus := v.focused
	v.focused = elements[tree.Focus]
	ax.mu.Unlock()

	viewH := tree.Height
	for id, n := range tree.Nodes {
		el := elements[id]
		if el == 0 {
			continue
		}
		role, sub := axRole(n)
		el.SendBool(RegisterSelector("setAccessibilityElement:"), true)
		axString(el, "setAccessibilityRole:", role)
		if sub != "" {
			axString(el, "setAccessibilitySubrole:", sub)
		}
		axString(el, "setAccessibilityLabel:", n.Name)
		if n.Description != "" {
			axString(el, "setAccessibilityHelp:", n.Description)
		}
		switch {
		case n.Checkable || role == "AXCheckBox" || role == "AXRadioButton":
			on := int64(0)
			if n.Checked || n.Selected {
				on = 1
			}
			num := ID(GetClass("NSNumber")).SendInt(RegisterSelector("numberWithInteger:"), on)
			el.SendPtr(RegisterSelector("setAccessibilityValue:"), uintptr(num))
		case role == "AXSlider" || role == "AXProgressIndicator":
			f, _ := strconv.ParseFloat(n.Value, 64)
			num := ID(GetClass("NSNumber")).SendDouble(RegisterSelector("numberWithDouble:"), f)
			el.SendPtr(RegisterSelector("setAccessibilityValue:"), uintptr(num))
		case n.Value != "":
			str := NewNSString(n.Value)
			el.SendPtr(RegisterSelector("setAccessibilityValue:"), uintptr(str.ID()))
			str.Release()
		}
		el.SendBool(RegisterSelector("setAccessibilityEnabled:"), !n.Disabled)
		el.SendBool(RegisterSelector("setAccessibilitySelected:"), n.Selected)
		el.SendBool(RegisterSelector("setAccessibilityFocused:"), n.Focused || id == tree.Focus)

		// Frames are in the parent's space, y up: relative to the parent
		// node's bottom-left, or to the (unflipped) view.
		var r NSRect
		if p := parents[id]; p != nil {
			el.SendPtr(RegisterSelector("setAccessibilityParent:"), uintptr(elements[p.ID]))
			r = NSRect{Origin: NSPoint{X: CGFloat(n.X - p.X), Y: CGFloat(p.Y + p.H - n.Y - n.H)}}
		} else {
			el.SendPtr(RegisterSelector("setAccessibilityParent:"), uintptr(view))
			r = NSRect{Origin: NSPoint{X: CGFloat(n.X), Y: CGFloat(viewH - n.Y - n.H)}}
		}
		r.Size = NSSize{Width: CGFloat(n.W), Height: CGFloat(n.H)}
		el.SendRect(RegisterSelector("setAccessibilityFrameInParentSpace:"), r)

		kids := make([]ID, 0, len(n.Children))
		for _, c := range n.Children {
			if e := elements[c]; e != 0 {
				kids = append(kids, e)
			}
		}
		el.SendPtr(RegisterSelector("setAccessibilityChildren:"), uintptr(axArray(kids)))

		if prev != nil {
			if o := prev.Nodes[id]; o != nil && (o.Value != n.Value || o.Checked != n.Checked) {
				axPost(el, "AXValueChanged")
			}
			if o := prev.Nodes[id]; o != nil && o.Name != n.Name {
				axPost(el, "AXTitleChanged")
			}
		}
	}
	roots := make([]ID, 0, len(tree.Roots))
	for _, id := range tree.Roots {
		if e := elements[id]; e != 0 {
			roots = append(roots, e)
		}
	}
	view.SendPtr(RegisterSelector("setAccessibilityChildren:"), uintptr(axArray(roots)))
	if len(created) > 0 || prev == nil {
		axPost(view, "AXLayoutChanged")
	}
	if v.focused != oldFocus && v.focused != 0 {
		axPost(v.focused, "AXFocusedUIElementChanged")
	}
}
