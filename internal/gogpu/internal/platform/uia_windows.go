//go:build windows && (amd64 || arm64)

package platform

// uia_windows.go — UI Automation (Narrator, NVDA, JAWS) for the app's
// accessible tree. The window answers WM_GETOBJECT with a fragment root;
// every node is a fragment implementing IRawElementProviderSimple and
// IRawElementProviderFragment, plus IInvokeProvider, IToggleProvider and
// IValueProvider where they apply.
//
// The COM objects live in GlobalAlloc memory (the GC never sees them):
// one slot per interface vtable, so each interface pointer is the object
// base plus the interface's slot. UIA calls them from its own threads;
// all state is guarded by uiaMu.

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/axtypes"
)

var (
	uiaCore                                    = windows.NewLazySystemDLL("uiautomationcore.dll")
	procUiaHostProviderFromHwnd                = uiaCore.NewProc("UiaHostProviderFromHwnd")
	procUiaReturnRawElementProvider            = uiaCore.NewProc("UiaReturnRawElementProvider")
	procUiaRaiseAutomationEvent                = uiaCore.NewProc("UiaRaiseAutomationEvent")
	procUiaRaiseAutomationPropertyChangedEvent = uiaCore.NewProc("UiaRaiseAutomationPropertyChangedEvent")
	procUiaRaiseStructureChangedEvent          = uiaCore.NewProc("UiaRaiseStructureChangedEvent")
	procUiaDisconnectProvider                  = uiaCore.NewProc("UiaDisconnectProvider")
	oleaut32                                   = windows.NewLazySystemDLL("oleaut32.dll")
	procSysAllocString                         = oleaut32.NewProc("SysAllocString")
	procSafeArrayCreateVector                  = oleaut32.NewProc("SafeArrayCreateVector")
	procSafeArrayPutElement                    = oleaut32.NewProc("SafeArrayPutElement")
	iidIRawElementProviderSimple               = comGUID{0xD6DD68D1, 0x86FD, 0x4332, [8]byte{0x86, 0x66, 0x9A, 0xBE, 0xDE, 0xA2, 0xD2, 0x4C}}
	iidIRawElementProviderFragment             = comGUID{0xF7063DA8, 0x8359, 0x439C, [8]byte{0x92, 0x97, 0xBB, 0xC5, 0x29, 0x9A, 0x7D, 0x87}}
	iidIRawElementProviderFragmentRoot         = comGUID{0x620CE2A5, 0xAB8F, 0x40A9, [8]byte{0x86, 0xCB, 0xDE, 0x3C, 0x75, 0x59, 0x9B, 0x58}}
	iidIInvokeProvider                         = comGUID{0x54FCB24B, 0xE18E, 0x47A2, [8]byte{0xB4, 0xD3, 0xEC, 0xCB, 0xE7, 0x75, 0x99, 0xA2}}
	iidIToggleProvider                         = comGUID{0x56D00BD0, 0xC4F4, 0x433C, [8]byte{0xA8, 0x36, 0x1A, 0x52, 0xA5, 0x7E, 0x08, 0x92}}
	iidIValueProvider                          = comGUID{0xC7935180, 0x6FB3, 0x4201, [8]byte{0xB1, 0x74, 0x7D, 0xF7, 0x3A, 0xDB, 0xF6, 0x4A}}
)

const (
	wmGetObject     = 0x003D
	uiaRootObjectID = -25

	sOK            = 0
	eNoInterface   = 0x80004002
	eInvalidArg    = 0x80070057
	eNotImpl       = 0x80004001
	uiaENotEnabled = 0x80040200 // UIA_E_ELEMENTNOTENABLED

	providerServerSide = 1

	patternInvoke = 10000
	patternValue  = 10002
	patternToggle = 10015

	propRuntimeID           = 30000
	propControlType         = 30003
	propName                = 30005
	propHasKeyboardFocus    = 30008
	propIsKeyboardFocusable = 30009
	propIsEnabled           = 30010
	propAutomationID        = 30011
	propHelpText            = 30013
	propIsPassword          = 30019
	propValueValue          = 30045
	propToggleState         = 30086
	propSelectionIsSelected = 30079

	eventFocusChanged = 20005

	ctButton      = 50000
	ctCheckBox    = 50002
	ctEdit        = 50004
	ctHyperlink   = 50005
	ctImage       = 50006
	ctListItem    = 50007
	ctList        = 50008
	ctMenu        = 50009
	ctMenuItem    = 50011
	ctProgressBar = 50012
	ctRadioButton = 50013
	ctSlider      = 50015
	ctTab         = 50018
	ctTabItem     = 50019
	ctText        = 50020
	ctTreeItem    = 50024
	ctGroup       = 50026
	ctHeader      = 50034

	vtEmpty   = 0
	vtI4      = 3
	vtR8      = 5
	vtBSTR    = 8
	vtBool    = 11
	vtUnknown = 13

	structureChildrenInvalidated = 2
	uiaAppendRuntimeID           = 3
)

// Interface slots in a provider object.
const (
	slotSimple = iota
	slotFragment
	slotFragmentRoot
	slotInvoke
	slotToggle
	slotValue
	slotRefs // reference count
	slotGone // 1 once the node left the tree
	slotCount
)

const ptrSize = unsafe.Sizeof(uintptr(0))

// variant is the Win32 VARIANT (24 bytes on 64-bit).
type variant struct {
	vt  uint16
	_   [3]uint16
	val uint64
	_   uint64
}

// uiaRect is the UiaRect structure (screen pixels).
type uiaRect struct{ left, top, width, height float64 }

type uiaWindow struct {
	w       *win32Window
	tree    *axtypes.Tree
	action  func(uint64)
	root    uintptr // fragment root provider (node id 0)
	objs    map[uint64]uintptr
	parents map[uint64]uint64
}

type uiaObj struct {
	win *uiaWindow
	id  uint64 // 0 = the fragment root
}

var (
	uiaMu      sync.Mutex
	uiaWindows = map[windows.HWND]*uiaWindow{}
	uiaObjs    = map[uintptr]uiaObj{}

	uiaVtblOnce sync.Once
	uiaVtbls    [slotRefs]uintptr
)

func slots(base uintptr) []uintptr {
	return unsafe.Slice((*uintptr)(unsafe.Pointer(base)), slotCount) //nolint:govet // GlobalAlloc memory
}

func outPtr(pp uintptr, v uintptr) {
	if pp != 0 {
		*(*uintptr)(unsafe.Pointer(pp)) = v //nolint:govet // COM out-pointer write
	}
}

func uiaAlloc(win *uiaWindow, id uint64) uintptr {
	mem, _, _ := procGlobalAlloc.Call(uintptr(gmemFixed), uintptr(slotCount)*ptrSize)
	if mem == 0 {
		return 0
	}
	s := slots(mem)
	for i := 0; i < slotRefs; i++ {
		s[i] = uiaVtbls[i]
	}
	s[slotRefs] = 1 // held by the tree
	s[slotGone] = 0
	uiaObjs[mem] = uiaObj{win: win, id: id}
	return mem
}

func uiaAddRef(base uintptr) uintptr {
	return uintptr(atomic.AddUintptr(&slots(base)[slotRefs], 1))
}

func uiaRelease(base uintptr) uintptr {
	s := slots(base)
	n := atomic.AddUintptr(&s[slotRefs], ^uintptr(0))
	if n == 0 && atomic.LoadUintptr(&s[slotGone]) == 1 {
		uiaMu.Lock()
		delete(uiaObjs, base)
		uiaMu.Unlock()
		procGlobalFree.Call(base)
	}
	return n
}

// drop removes an object from its tree; COM may still hold it.
func uiaDrop(base uintptr) {
	atomic.StoreUintptr(&slots(base)[slotGone], 1)
	procUiaDisconnectProvider.Call(base + slotSimple*ptrSize)
	uiaRelease(base)
}

// lookup finds an object under uiaMu; ok is false once it left the tree.
func lookup(base uintptr) (uiaObj, *axtypes.Node, bool) {
	o, ok := uiaObjs[base]
	if !ok || o.win.tree == nil || atomic.LoadUintptr(&slots(base)[slotGone]) == 1 {
		return uiaObj{}, nil, false
	}
	if o.id == 0 {
		return o, nil, true
	}
	n := o.win.tree.Nodes[o.id]
	return o, n, n != nil
}

// iface returns the interface pointer of a node (AddRef'd), or 0.
func (u *uiaWindow) iface(id uint64, slot uintptr) uintptr {
	base := u.root
	if id != 0 {
		base = u.objs[id]
	}
	if base == 0 {
		return 0
	}
	uiaAddRef(base)
	return base + slot*ptrSize
}

func controlType(n *axtypes.Node) int32 {
	switch n.Role {
	case "button":
		return ctButton
	case "checkbox", "switch":
		return ctCheckBox
	case "radio":
		return ctRadioButton
	case "textfield", "password":
		return ctEdit
	case "slider":
		return ctSlider
	case "progressbar":
		return ctProgressBar
	case "tab":
		return ctTabItem
	case "tablist":
		return ctTab
	case "menu":
		return ctMenu
	case "menuitem":
		return ctMenuItem
	case "header", "heading":
		return ctHeader
	case "image":
		return ctImage
	case "link":
		return ctHyperlink
	case "text", "label":
		return ctText
	case "list":
		return ctList
	case "listitem":
		return ctListItem
	case "treeitem":
		return ctTreeItem
	}
	return ctGroup
}

func hasPattern(n *axtypes.Node, pattern uintptr) bool {
	switch pattern {
	case patternInvoke:
		return n.Actionable && !n.Checkable
	case patternToggle:
		return n.Checkable
	case patternValue:
		return n.Role == "textfield" || n.Role == "password" || n.Value != ""
	}
	return false
}

func bstr(s string) uintptr {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	r, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	return r
}

func setVariant(pv uintptr, vt uint16, val uint64) {
	if pv == 0 {
		return
	}
	v := (*variant)(unsafe.Pointer(pv)) //nolint:govet // COM VARIANT out-pointer
	*v = variant{vt: vt, val: val}
}

func vBool(b bool) uint64 {
	if b {
		return 0xFFFF // VARIANT_TRUE
	}
	return 0
}

func uiaVTables() {
	uiaVtblOnce.Do(func() {
		mk := func(fns ...any) uintptr {
			mem, _, _ := procGlobalAlloc.Call(uintptr(gmemFixed), uintptr(len(fns))*ptrSize)
			if mem == 0 {
				return 0
			}
			v := unsafe.Slice((*uintptr)(unsafe.Pointer(mem)), len(fns)) //nolint:govet // GlobalAlloc memory
			for i, fn := range fns {
				v[i] = syscall.NewCallback(fn)
			}
			return mem
		}
		unknown := func(slot uintptr) (any, any, any) {
			return func(this, riid, ppv uintptr) uintptr { return uiaQueryInterface(this-slot*ptrSize, riid, ppv) },
				func(this uintptr) uintptr { return uiaAddRef(this - slot*ptrSize) },
				func(this uintptr) uintptr { return uiaRelease(this - slot*ptrSize) }
		}
		qi, ar, rl := unknown(slotSimple)
		uiaVtbls[slotSimple] = mk(qi, ar, rl,
			func(this, pRet uintptr) uintptr { // get_ProviderOptions
				if pRet != 0 {
					*(*int32)(unsafe.Pointer(pRet)) = providerServerSide //nolint:govet // out-pointer
				}
				return sOK
			},
			func(this, pattern, pRet uintptr) uintptr {
				return uiaPattern(this-slotSimple*ptrSize, uintptr(int32(pattern)), pRet)
			},
			func(this, prop, pRet uintptr) uintptr { return uiaProperty(this-slotSimple*ptrSize, int32(prop), pRet) },
			func(this, pRet uintptr) uintptr { return uiaHost(this-slotSimple*ptrSize, pRet) },
		)
		qi, ar, rl = unknown(slotFragment)
		uiaVtbls[slotFragment] = mk(qi, ar, rl,
			func(this, dir, pRet uintptr) uintptr { return uiaNavigate(this-slotFragment*ptrSize, int32(dir), pRet) },
			func(this, pRet uintptr) uintptr { return uiaRuntimeID(this-slotFragment*ptrSize, pRet) },
			func(this, pRet uintptr) uintptr { return uiaBounds(this-slotFragment*ptrSize, pRet) },
			func(this, pRet uintptr) uintptr { outPtr(pRet, 0); return sOK }, // GetEmbeddedFragmentRoots
			func(this uintptr) uintptr { return sOK },                        // SetFocus
			func(this, pRet uintptr) uintptr { return uiaFragmentRoot(this-slotFragment*ptrSize, pRet) },
		)
		qi, ar, rl = unknown(slotFragmentRoot)
		uiaVtbls[slotFragmentRoot] = mk(qi, ar, rl,
			// ElementProviderFromPoint(double x, double y, out): the
			// doubles travel in float registers, which callbacks can't
			// read, so the point is taken from the cursor (what Narrator
			// and inspect tools probe). The out pointer is the second
			// integer argument on arm64 and the fourth on amd64.
			func(this, a, b, c uintptr) uintptr {
				out := c
				if runtime.GOARCH == "arm64" {
					out = a
				}
				return uiaFromPoint(this-slotFragmentRoot*ptrSize, out)
			},
			func(this, pRet uintptr) uintptr { return uiaGetFocus(this-slotFragmentRoot*ptrSize, pRet) },
		)
		qi, ar, rl = unknown(slotInvoke)
		uiaVtbls[slotInvoke] = mk(qi, ar, rl,
			func(this uintptr) uintptr { return uiaAct(this - slotInvoke*ptrSize) },
		)
		qi, ar, rl = unknown(slotToggle)
		uiaVtbls[slotToggle] = mk(qi, ar, rl,
			func(this uintptr) uintptr { return uiaAct(this - slotToggle*ptrSize) },
			func(this, pRet uintptr) uintptr {
				uiaMu.Lock()
				_, n, ok := lookup(this - slotToggle*ptrSize)
				uiaMu.Unlock()
				if pRet != 0 {
					st := int32(0)
					if ok && n.Checked {
						st = 1
					}
					*(*int32)(unsafe.Pointer(pRet)) = st //nolint:govet // out-pointer
				}
				return sOK
			},
		)
		qi, ar, rl = unknown(slotValue)
		uiaVtbls[slotValue] = mk(qi, ar, rl,
			func(this, s uintptr) uintptr { return eNotImpl }, // SetValue
			func(this, pRet uintptr) uintptr {
				uiaMu.Lock()
				_, n, ok := lookup(this - slotValue*ptrSize)
				val := ""
				if ok {
					val = n.Value
				}
				uiaMu.Unlock()
				outPtr(pRet, bstr(val))
				return sOK
			},
			func(this, pRet uintptr) uintptr { // get_IsReadOnly
				if pRet != 0 {
					*(*int32)(unsafe.Pointer(pRet)) = 1 //nolint:govet // out-pointer
				}
				return sOK
			},
		)
	})
}

func uiaQueryInterface(base, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return eInvalidArg
	}
	g := (*comGUID)(unsafe.Pointer(riid)) //nolint:govet // REFIID
	uiaMu.Lock()
	o, n, ok := lookup(base)
	uiaMu.Unlock()
	slot := uintptr(1 << 16)
	switch {
	case guidEqual(g, &iidIUnknown), guidEqual(g, &iidIRawElementProviderSimple):
		slot = slotSimple
	case guidEqual(g, &iidIRawElementProviderFragment):
		slot = slotFragment
	case guidEqual(g, &iidIRawElementProviderFragmentRoot) && o.id == 0:
		slot = slotFragmentRoot
	case guidEqual(g, &iidIInvokeProvider) && ok && n != nil && hasPattern(n, patternInvoke):
		slot = slotInvoke
	case guidEqual(g, &iidIToggleProvider) && ok && n != nil && hasPattern(n, patternToggle):
		slot = slotToggle
	case guidEqual(g, &iidIValueProvider) && ok && n != nil && hasPattern(n, patternValue):
		slot = slotValue
	}
	if slot > slotValue {
		outPtr(ppv, 0)
		return eNoInterface
	}
	uiaAddRef(base)
	outPtr(ppv, base+slot*ptrSize)
	return sOK
}

func uiaPattern(base, pattern, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	defer uiaMu.Unlock()
	_, n, ok := lookup(base)
	if !ok || n == nil || !hasPattern(n, pattern) {
		return sOK
	}
	slot := map[uintptr]uintptr{patternInvoke: slotInvoke, patternToggle: slotToggle, patternValue: slotValue}[pattern]
	uiaAddRef(base)
	outPtr(pRet, base+slot*ptrSize)
	return sOK
}

func uiaProperty(base uintptr, prop int32, pRet uintptr) uintptr {
	setVariant(pRet, vtEmpty, 0)
	uiaMu.Lock()
	o, n, ok := lookup(base)
	if !ok || n == nil {
		uiaMu.Unlock()
		return sOK
	}
	focus := o.win.tree.Focus == n.ID || n.Focused
	uiaMu.Unlock()
	switch prop {
	case propControlType:
		setVariant(pRet, vtI4, uint64(uint32(controlType(n))))
	case propName:
		setVariant(pRet, vtBSTR, uint64(bstr(n.Name)))
	case propHelpText:
		if n.Description != "" {
			setVariant(pRet, vtBSTR, uint64(bstr(n.Description)))
		}
	case propAutomationID:
		setVariant(pRet, vtBSTR, uint64(bstr("n"+strconv.FormatUint(n.ID, 10))))
	case propIsEnabled:
		setVariant(pRet, vtBool, vBool(!n.Disabled))
	case propHasKeyboardFocus:
		setVariant(pRet, vtBool, vBool(focus))
	case propIsKeyboardFocusable:
		setVariant(pRet, vtBool, vBool(n.Actionable || n.Role == "textfield" || n.Role == "password"))
	case propIsPassword:
		setVariant(pRet, vtBool, vBool(n.Role == "password"))
	case propSelectionIsSelected:
		setVariant(pRet, vtBool, vBool(n.Selected))
	}
	return sOK
}

func uiaHost(base, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	o, _, ok := lookup(base)
	uiaMu.Unlock()
	if ok && o.id == 0 {
		procUiaHostProviderFromHwnd.Call(uintptr(o.win.w.hwnd), pRet)
	}
	return sOK
}

func uiaNavigate(base uintptr, dir int32, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	defer uiaMu.Unlock()
	o, n, ok := lookup(base)
	if !ok {
		return sOK
	}
	u := o.win
	children := func(id uint64) []uint64 {
		if id == 0 {
			return u.tree.Roots
		}
		if c := u.tree.Nodes[id]; c != nil {
			return c.Children
		}
		return nil
	}
	const (
		navParent = iota
		navNext
		navPrev
		navFirst
		navLast
	)
	switch dir {
	case navParent:
		if n != nil {
			outPtr(pRet, u.iface(u.parents[n.ID], slotFragment))
		}
	case navFirst, navLast:
		c := children(o.id)
		if len(c) > 0 {
			id := c[0]
			if dir == navLast {
				id = c[len(c)-1]
			}
			outPtr(pRet, u.iface(id, slotFragment))
		}
	case navNext, navPrev:
		if n == nil {
			return sOK
		}
		sib := children(u.parents[n.ID])
		for i, id := range sib {
			if id != n.ID {
				continue
			}
			j := i + 1
			if dir == navPrev {
				j = i - 1
			}
			if j >= 0 && j < len(sib) {
				outPtr(pRet, u.iface(sib[j], slotFragment))
			}
			break
		}
	}
	return sOK
}

func uiaRuntimeID(base, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	o, _, ok := lookup(base)
	uiaMu.Unlock()
	if !ok || o.id == 0 { // the host supplies the root's id
		return sOK
	}
	vals := []int32{uiaAppendRuntimeID, int32(uint32(o.id)), int32(uint32(o.id >> 32))}
	sa, _, _ := procSafeArrayCreateVector.Call(vtI4, 0, uintptr(len(vals)))
	if sa == 0 {
		return sOK
	}
	for i := range vals {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&vals[i])))
	}
	outPtr(pRet, sa)
	return sOK
}

func uiaBounds(base, pRet uintptr) uintptr {
	if pRet == 0 {
		return eInvalidArg
	}
	r := (*uiaRect)(unsafe.Pointer(pRet)) //nolint:govet // out-pointer
	*r = uiaRect{}
	uiaMu.Lock()
	o, n, ok := lookup(base)
	uiaMu.Unlock()
	if !ok || n == nil { // the host supplies the root's bounds
		return sOK
	}
	origin := point{}
	procClientToScreen.Call(uintptr(o.win.w.hwnd), uintptr(unsafe.Pointer(&origin)))
	s := o.win.w.scaleFactor()
	if s <= 0 {
		s = 1
	}
	*r = uiaRect{float64(origin.x) + n.X*s, float64(origin.y) + n.Y*s, n.W * s, n.H * s}
	return sOK
}

func uiaFragmentRoot(base, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	defer uiaMu.Unlock()
	if o, _, ok := lookup(base); ok {
		outPtr(pRet, o.win.iface(0, slotFragmentRoot))
	}
	return sOK
}

func uiaFromPoint(base, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	o, _, ok := lookup(base)
	uiaMu.Unlock()
	if !ok {
		return sOK
	}
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procScreenToClient.Call(uintptr(o.win.w.hwnd), uintptr(unsafe.Pointer(&pt)))
	s := o.win.w.scaleFactor()
	if s <= 0 {
		s = 1
	}
	x, y := float64(pt.x)/s, float64(pt.y)/s
	uiaMu.Lock()
	defer uiaMu.Unlock()
	var best *axtypes.Node
	for _, n := range o.win.tree.Nodes {
		if x >= n.X && y >= n.Y && x < n.X+n.W && y < n.Y+n.H && (best == nil || n.W*n.H < best.W*best.H) {
			best = n
		}
	}
	if best != nil {
		outPtr(pRet, o.win.iface(best.ID, slotFragment))
	}
	return sOK
}

func uiaGetFocus(base, pRet uintptr) uintptr {
	outPtr(pRet, 0)
	uiaMu.Lock()
	defer uiaMu.Unlock()
	if o, _, ok := lookup(base); ok && o.win.tree.Focus != 0 {
		outPtr(pRet, o.win.iface(o.win.tree.Focus, slotFragment))
	}
	return sOK
}

func uiaAct(base uintptr) uintptr {
	uiaMu.Lock()
	o, n, ok := lookup(base)
	var act func(uint64)
	if ok && n != nil {
		act = o.win.action
	}
	uiaMu.Unlock()
	if act == nil {
		return eInvalidArg
	}
	if n.Disabled {
		return uiaENotEnabled
	}
	act(n.ID)
	return sOK
}

// setUIATree publishes the accessible content of w (nil clears it).
func setUIATree(w *win32Window, tree *axtypes.Tree, action func(uint64)) {
	if w == nil || w.hwnd == 0 || uiaCore.Load() != nil || oleaut32.Load() != nil {
		return
	}
	uiaVTables()
	if uiaVtbls[slotSimple] == 0 {
		return
	}
	uiaMu.Lock()
	u := uiaWindows[w.hwnd]
	if tree == nil {
		if u != nil {
			delete(uiaWindows, w.hwnd)
			drop := []uintptr{u.root}
			for _, b := range u.objs {
				drop = append(drop, b)
			}
			u.tree = nil
			uiaMu.Unlock()
			for _, b := range drop {
				uiaDrop(b)
			}
			return
		}
		uiaMu.Unlock()
		return
	}
	if u == nil {
		u = &uiaWindow{w: w, objs: map[uint64]uintptr{}}
		u.root = uiaAlloc(u, 0)
		uiaWindows[w.hwnd] = u
	}
	prev := u.tree
	u.tree, u.action = tree, action
	u.parents = map[uint64]uint64{}
	for _, n := range tree.Nodes {
		for _, c := range n.Children {
			u.parents[c] = n.ID
		}
	}
	var gone []uintptr
	for id, b := range u.objs {
		if _, ok := tree.Nodes[id]; !ok {
			gone = append(gone, b)
			delete(u.objs, id)
		}
	}
	changed := len(gone) > 0
	for id := range tree.Nodes {
		if _, ok := u.objs[id]; !ok {
			if b := uiaAlloc(u, id); b != 0 {
				u.objs[id] = b
				changed = true
			}
		}
	}
	var focus uintptr
	if prev == nil || prev.Focus != tree.Focus {
		focus = u.objs[tree.Focus]
	}
	type propChange struct {
		base uintptr
		prop int32
		vt   uint16
		old  uint64
		new  uint64
	}
	var props []propChange
	if prev != nil {
		for id, n := range tree.Nodes {
			o := prev.Nodes[id]
			if o == nil {
				continue
			}
			if o.Checked != n.Checked && n.Checkable {
				props = append(props, propChange{u.objs[id], propToggleState, vtI4, uint64(boolI4(o.Checked)), uint64(boolI4(n.Checked))})
			}
			if o.Name != n.Name {
				props = append(props, propChange{u.objs[id], propName, vtBSTR, uint64(bstr(o.Name)), uint64(bstr(n.Name))})
			}
			if o.Value != n.Value {
				props = append(props, propChange{u.objs[id], propValueValue, vtBSTR, uint64(bstr(o.Value)), uint64(bstr(n.Value))})
			}
		}
	}
	root := u.root
	uiaMu.Unlock()

	for _, b := range gone {
		uiaDrop(b)
	}
	if changed && prev != nil {
		procUiaRaiseStructureChangedEvent.Call(root+slotSimple*ptrSize, structureChildrenInvalidated, 0, 0)
	}
	for _, c := range props {
		ov, nv := variant{vt: c.vt, val: c.old}, variant{vt: c.vt, val: c.new}
		// VARIANTs (24 bytes) are passed by value, which the x64 and arm64
		// conventions both turn into a pointer to a copy.
		procUiaRaiseAutomationPropertyChangedEvent.Call(c.base+slotSimple*ptrSize, uintptr(c.prop),
			uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&nv)))
	}
	if focus != 0 {
		procUiaRaiseAutomationEvent.Call(focus+slotSimple*ptrSize, eventFocusChanged)
	}
}

func boolI4(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// uiaGetObject answers WM_GETOBJECT for UI Automation. ok is false when
// the window has no accessible tree (DefWindowProc handles it then).
func uiaGetObject(hwnd windows.HWND, wParam, lParam uintptr) (uintptr, bool) {
	if int32(lParam) != uiaRootObjectID {
		return 0, false
	}
	uiaMu.Lock()
	u := uiaWindows[hwnd]
	root := uintptr(0)
	if u != nil {
		root = u.root
	}
	uiaMu.Unlock()
	if root == 0 {
		return 0, false
	}
	r, _, _ := procUiaReturnRawElementProvider.Call(uintptr(hwnd), wParam, lParam, root+slotSimple*ptrSize)
	return r, true
}
