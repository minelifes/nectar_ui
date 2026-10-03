//go:build windows && (amd64 || arm64)

package platform

// drop_target_windows.go — incoming drag-and-drop through OLE IDropTarget,
// so a window gets enter / move / leave events while files are dragged
// over it (WM_DROPFILES only reports the drop). Falls back to
// WM_DROPFILES when RegisterDragDrop fails.

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")

	iidIDropTarget = comGUID{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	dropTargetVtbl     uintptr
	dropTargetVtblOnce sync.Once

	// dropTargets keeps the targets alive while COM holds them.
	dropTargetsMu sync.Mutex
	dropTargets   = map[uintptr]*dropTarget{}
)

// dropTarget implements IDropTarget for one window. vtbl must stay first.
type dropTarget struct {
	vtbl     uintptr
	refCount int32
	p        *windowsPlatform
	w        *win32Window
	paths    []string // of the drag in progress
}

func dropTargetVTable() uintptr {
	dropTargetVtblOnce.Do(func() {
		mem, _, _ := procGlobalAlloc.Call(uintptr(gmemFixed), 7*unsafe.Sizeof(uintptr(0)))
		if mem == 0 {
			return
		}
		v := unsafe.Slice((*uintptr)(unsafe.Pointer(mem)), 7) //nolint:govet // GlobalAlloc returns non-GC memory
		v[0] = syscall.NewCallback(dropTargetQueryInterface)
		v[1] = syscall.NewCallback(dropTargetAddRef)
		v[2] = syscall.NewCallback(dropTargetRelease)
		v[3] = syscall.NewCallback(dropTargetDragEnter)
		v[4] = syscall.NewCallback(dropTargetDragOver)
		v[5] = syscall.NewCallback(dropTargetDragLeave)
		v[6] = syscall.NewCallback(dropTargetDrop)
		dropTargetVtbl = mem
	})
	return dropTargetVtbl
}

// registerDropTarget makes w an OLE drop target. Returns false if OLE
// refused (the caller keeps WM_DROPFILES then).
func registerDropTarget(p *windowsPlatform, w *win32Window) bool {
	vt := dropTargetVTable()
	if vt == 0 {
		return false
	}
	t := &dropTarget{vtbl: vt, refCount: 1, p: p, w: w}
	hr, _, _ := procRegisterDragDrop.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(t)))
	if hr != 0 {
		return false
	}
	dropTargetsMu.Lock()
	dropTargets[uintptr(w.hwnd)] = t
	dropTargetsMu.Unlock()
	return true
}

// revokeDropTarget unregisters w (before the window is destroyed).
func revokeDropTarget(w *win32Window) {
	dropTargetsMu.Lock()
	_, ok := dropTargets[uintptr(w.hwnd)]
	delete(dropTargets, uintptr(w.hwnd))
	dropTargetsMu.Unlock()
	if ok {
		procRevokeDragDrop.Call(uintptr(w.hwnd))
	}
}

func targetOf(this uintptr) *dropTarget { return (*dropTarget)(unsafe.Pointer(this)) } //nolint:govet // COM this pointer

func dropTargetQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return 0x80070057 // E_INVALIDARG
	}
	guid := (*comGUID)(unsafe.Pointer(riid)) //nolint:govet // COM REFIID pointer cast
	if guidEqual(guid, &iidIUnknown) || guidEqual(guid, &iidIDropTarget) {
		*(*uintptr)(unsafe.Pointer(ppv)) = this //nolint:govet // COM out-pointer write
		dropTargetAddRef(this)
		return 0
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = 0 //nolint:govet // COM out-pointer write
	return 0x80004002                    // E_NOINTERFACE
}

func dropTargetAddRef(this uintptr) uintptr {
	t := targetOf(this)
	t.refCount++
	return uintptr(t.refCount)
}

func dropTargetRelease(this uintptr) uintptr {
	t := targetOf(this)
	t.refCount--
	return uintptr(max(t.refCount, 0))
}

// clientPos converts a POINTL (passed by value: x in the low half) in
// screen coordinates to logical client coordinates.
func (t *dropTarget) clientPos(pt uintptr) (float64, float64) {
	p := point{x: int32(uint32(pt)), y: int32(uint32(pt >> 32))}
	procScreenToClient.Call(uintptr(t.w.hwnd), uintptr(unsafe.Pointer(&p)))
	s := t.w.scaleFactor()
	if s <= 0 {
		s = 1
	}
	return float64(p.x) / s, float64(p.y) / s
}

func setEffect(pdwEffect uintptr, ok bool) {
	if pdwEffect == 0 {
		return
	}
	e := uint32(dropEffectNone)
	if ok {
		e = dropEffectCopy
	}
	*(*uint32)(unsafe.Pointer(pdwEffect)) = e //nolint:govet // COM out-pointer write
}

// IDropTarget::DragEnter(pDataObj, grfKeyState, pt, pdwEffect)
func dropTargetDragEnter(this, pDataObj, keyState, pt, pdwEffect uintptr) uintptr {
	t := targetOf(this)
	t.paths = filesOf(pDataObj)
	setEffect(pdwEffect, len(t.paths) > 0)
	if len(t.paths) > 0 {
		x, y := t.clientPos(pt)
		t.p.queueEvent(Event{WindowID: t.w.id, Type: EventDragEnter, DragPaths: t.paths, DragX: x, DragY: y})
	}
	return 0
}

// IDropTarget::DragOver(grfKeyState, pt, pdwEffect)
func dropTargetDragOver(this, keyState, pt, pdwEffect uintptr) uintptr {
	t := targetOf(this)
	setEffect(pdwEffect, len(t.paths) > 0)
	if len(t.paths) > 0 {
		x, y := t.clientPos(pt)
		t.p.queueEvent(Event{WindowID: t.w.id, Type: EventDragMove, DragX: x, DragY: y})
	}
	return 0
}

// IDropTarget::DragLeave()
func dropTargetDragLeave(this uintptr) uintptr {
	t := targetOf(this)
	if len(t.paths) > 0 {
		t.p.queueEvent(Event{WindowID: t.w.id, Type: EventDragLeave})
	}
	t.paths = nil
	return 0
}

// IDropTarget::Drop(pDataObj, grfKeyState, pt, pdwEffect)
func dropTargetDrop(this, pDataObj, keyState, pt, pdwEffect uintptr) uintptr {
	t := targetOf(this)
	paths := filesOf(pDataObj)
	setEffect(pdwEffect, len(paths) > 0)
	if len(paths) > 0 {
		x, y := t.clientPos(pt)
		t.p.queueEvent(Event{WindowID: t.w.id, Type: EventDragDrop, DragPaths: paths, DragX: x, DragY: y})
	}
	t.paths = nil
	return 0
}

// filesOf reads the CF_HDROP file list of an IDataObject (nil if none).
func filesOf(dataObj uintptr) []string {
	if dataObj == 0 {
		return nil
	}
	fe := formatETC{cfFormat: cfHDROP, dwAspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal}
	var stg stgMedium
	vtbl := *(*uintptr)(unsafe.Pointer(dataObj))                               //nolint:govet // COM vtable read
	getData := *(*uintptr)(unsafe.Pointer(vtbl + 3*unsafe.Sizeof(uintptr(0)))) //nolint:govet // IDataObject::GetData
	hr, _, _ := syscall.SyscallN(getData, dataObj, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&stg)))
	if hr != 0 || stg.hGlobal == 0 {
		return nil
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg)))
	hDrop := stg.hGlobal
	count, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	paths := make([]string, 0, count)
	buf := make([]uint16, 260)
	for i := uintptr(0); i < count; i++ {
		n, _, _ := procDragQueryFileW.Call(hDrop, i, 0, 0)
		if n == 0 {
			continue
		}
		if n+1 > uintptr(len(buf)) {
			buf = make([]uint16, n+1)
		}
		procDragQueryFileW.Call(hDrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		paths = append(paths, syscall.UTF16ToString(buf[:n]))
	}
	return paths
}
