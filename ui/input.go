package ui

import (
	"sync"

	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Pixels per scroll "line" / "page" when the platform reports those units.
const (
	scrollLinePx = 40
	scrollPagePx = 400
)

// inputQueue collects platform events (which may arrive on another thread)
// and replays them on the UI thread at the start of the next frame.
type inputQueue struct {
	mu      sync.Mutex
	pending []queuedEvent

	dispatcher *render.PointerDispatcher
	cursor     render.Cursor

	// afterShortcut is set by a key press held with a shortcut modifier:
	// the text the platform sends with it isn't typing (see deliver).
	afterShortcut bool
}

// queuedEvent is one platform event: pointer/scroll, key press, text or
// an IME composition step.
type queuedEvent struct {
	pointer *render.PointerEvent
	key     *widgets.KeyEvent
	text    string
	ime     *widgets.IMEEvent
}

// setupInput subscribes to gogpu's pointer, scroll, key and text events.
func (a *App) setupInput() {
	a.input = &inputQueue{dispatcher: render.NewPointerDispatcher()}
	es := a.gpuApp.EventSource()
	if pes, ok := es.(gpucontext.PointerEventSource); ok {
		pes.OnPointer(func(ev gpucontext.PointerEvent) {
			if e, ok := convertPointer(ev); ok {
				a.input.push(e)
				a.gpuApp.RequestRedraw()
			}
		})
	}
	if ses, ok := es.(gpucontext.ScrollEventSource); ok {
		ses.OnScrollEvent(func(ev gpucontext.ScrollEvent) {
			a.input.push(convertScroll(ev))
			a.gpuApp.RequestRedraw()
		})
	}
	es.OnKeyPress(func(k gpucontext.Key, m gpucontext.Modifiers) {
		e := widgets.KeyEvent{Key: convertKey(k), Mods: convertMods(m)}
		a.input.enqueue(queuedEvent{key: &e})
		a.gpuApp.RequestRedraw()
	})
	es.OnTextInput(func(s string) {
		a.input.enqueue(queuedEvent{text: s})
		a.gpuApp.RequestRedraw()
	})
	// IME composition (CJK etc.). The platform reports the caret inside the
	// preedit in runes; widgets use byte offsets.
	ime := func(e widgets.IMEEvent) {
		a.input.enqueue(queuedEvent{ime: &e})
		a.gpuApp.RequestRedraw()
	}
	es.OnIMECompositionStart(func() { ime(widgets.IMEEvent{Kind: widgets.IMEStart}) })
	es.OnIMECompositionUpdate(func(st gpucontext.IMEState) {
		ime(widgets.IMEEvent{Kind: widgets.IMEUpdate, Text: st.CompositionText, Cursor: runeToByte(st.CompositionText, st.CursorPos)})
	})
	es.OnIMECompositionEnd(func(committed string) { ime(widgets.IMEEvent{Kind: widgets.IMEEnd, Text: committed}) })
	a.buildOwner.Clipboard = appClipboard{a}
}

// appClipboard adapts gogpu's clipboard to widgets.Clipboard.
type appClipboard struct{ a *App }

func (c appClipboard) ReadText() (string, error) { return c.a.gpuApp.ClipboardRead() }
func (c appClipboard) WriteText(s string) error  { return c.a.gpuApp.ClipboardWrite(s) }

func (q *inputQueue) push(e render.PointerEvent) { q.enqueue(queuedEvent{pointer: &e}) }

func (q *inputQueue) enqueue(e queuedEvent) {
	q.mu.Lock()
	q.pending = append(q.pending, e)
	q.mu.Unlock()
}

// processInput dispatches queued events against the last laid-out tree.
// Runs on the UI thread before build, so handlers can call SetState.
func (a *App) processInput() {
	q := a.input
	if q == nil {
		return
	}
	q.mu.Lock()
	events := q.pending
	q.pending = nil
	q.mu.Unlock()
	if len(events) == 0 {
		return
	}
	root := a.pipeline.Root()
	fm := a.buildOwner.Focus()
	for _, e := range events {
		switch {
		case e.pointer != nil:
			q.afterShortcut = false
			down := e.pointer.Kind == render.PointerDown
			if down {
				fm.BeginPointerDown()
			}
			q.dispatcher.Dispatch(root, *e.pointer)
			if down {
				fm.EndPointerDown()
			}
		default:
			q.deliver(fm, e)
		}
	}
	if c := q.dispatcher.Cursor(); c != q.cursor {
		q.cursor = c
		a.gpuApp.SetCursor(cursorShape(c))
	}
}

// deliver routes a keyboard event (key, text or IME) to the focus manager.
//
// Platforms send a key press and, separately, the text it types. For
// shortcuts that text is bogus: macOS reports ⌘V as KeyV+⌘ plus the text
// "v", X11 and Wayland do the same for Ctrl+V. Delivered, it would type the
// letter over the selection right after the copy or paste. So text that
// follows a key press held with ⌘/Super, or with Ctrl but not Alt, is
// dropped until the next key press. (Ctrl+Alt is AltGr on Windows and types
// real characters such as @ and €; Alt alone is macOS Option, é and ©.)
func (q *inputQueue) deliver(fm *widgets.FocusManager, e queuedEvent) {
	switch {
	case e.key != nil:
		q.afterShortcut = isShortcutChord(e.key.Mods)
		fm.HandleKey(*e.key)
	case e.text != "":
		if q.afterShortcut {
			return
		}
		fm.HandleText(e.text)
	case e.ime != nil:
		q.afterShortcut = false
		fm.HandleIME(*e.ime)
	}
}

// isShortcutChord reports whether keys pressed with mods are shortcuts
// rather than typing.
func isShortcutChord(m widgets.Modifiers) bool {
	return m&widgets.ModSuper != 0 || m&widgets.ModControl != 0 && m&widgets.ModAlt == 0
}

func convertPointer(ev gpucontext.PointerEvent) (render.PointerEvent, bool) {
	e := render.PointerEvent{
		ID:       ev.PointerID,
		Position: geom.Offset{X: float32(ev.X), Y: float32(ev.Y)},
		Button:   int(ev.Button),
		Touch:    ev.PointerType == gpucontext.PointerTypeTouch,
		Mods:     uint8(convertMods(ev.Modifiers)),
	}
	switch ev.Type {
	case gpucontext.PointerDown:
		e.Kind = render.PointerDown
	case gpucontext.PointerUp:
		e.Kind = render.PointerUp
	case gpucontext.PointerMove:
		e.Kind = render.PointerHover
		if ev.Buttons != 0 || e.Touch {
			e.Kind = render.PointerMove
		}
	case gpucontext.PointerCancel:
		e.Kind = render.PointerCancel
	case gpucontext.PointerLeave:
		e.Kind = render.PointerExit
	default: // PointerEnter: hover state updates on the next move
		return e, false
	}
	return e, true
}

func convertScroll(ev gpucontext.ScrollEvent) render.PointerEvent {
	unit := float32(1)
	switch ev.DeltaMode {
	case gpucontext.ScrollDeltaLine:
		unit = scrollLinePx
	case gpucontext.ScrollDeltaPage:
		unit = scrollPagePx
	}
	return render.PointerEvent{
		Kind:     render.PointerScroll,
		Position: geom.Offset{X: float32(ev.X), Y: float32(ev.Y)},
		Scroll:   geom.Offset{X: float32(ev.DeltaX) * unit, Y: float32(ev.DeltaY) * unit},
		Button:   -1,
		Mods:     uint8(convertMods(ev.Modifiers)),
	}
}

func cursorShape(c render.Cursor) gpucontext.CursorShape {
	switch c {
	case render.CursorPointer:
		return gpucontext.CursorPointer
	case render.CursorText:
		return gpucontext.CursorText
	case render.CursorCrosshair:
		return gpucontext.CursorCrosshair
	case render.CursorMove:
		return gpucontext.CursorMove
	case render.CursorResizeNS:
		return gpucontext.CursorResizeNS
	case render.CursorResizeEW:
		return gpucontext.CursorResizeEW
	case render.CursorNotAllowed:
		return gpucontext.CursorNotAllowed
	}
	return gpucontext.CursorDefault
}

func convertMods(m gpucontext.Modifiers) widgets.Modifiers {
	var out widgets.Modifiers
	if m&gpucontext.ModShift != 0 {
		out |= widgets.ModShift
	}
	if m&gpucontext.ModControl != 0 {
		out |= widgets.ModControl
	}
	if m&gpucontext.ModAlt != 0 {
		out |= widgets.ModAlt
	}
	if m&gpucontext.ModSuper != 0 {
		out |= widgets.ModSuper
	}
	return out
}

func convertKey(k gpucontext.Key) widgets.KeyCode {
	switch {
	case k >= gpucontext.KeyA && k <= gpucontext.KeyZ:
		return widgets.KeyA + widgets.KeyCode(k-gpucontext.KeyA)
	case k >= gpucontext.Key0 && k <= gpucontext.Key9:
		return widgets.Key0 + widgets.KeyCode(k-gpucontext.Key0)
	case k >= gpucontext.KeyEscape && k <= gpucontext.KeyDown:
		// Same order in both enums: Escape Tab Backspace Enter Space Insert
		// Delete Home End PageUp PageDown Left Right Up Down.
		return widgets.KeyEscape + widgets.KeyCode(k-gpucontext.KeyEscape)
	case k >= gpucontext.KeyF1 && k <= gpucontext.KeyF12:
		return widgets.KeyF1 + widgets.KeyCode(k-gpucontext.KeyF1)
	case k >= gpucontext.KeyMinus && k <= gpucontext.KeySlash:
		// Same order: - = [ ] \ ; ' ` , . /
		return widgets.KeyMinus + widgets.KeyCode(k-gpucontext.KeyMinus)
	case k == gpucontext.KeyNumpadEnter:
		return widgets.KeyEnter
	}
	return widgets.KeyUnknown
}

func runeToByte(s string, runes int) int {
	if runes <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == runes {
			return i
		}
		n++
	}
	return len(s)
}

// imeController is what a platform offers to steer its input method
// (gpucontext.IMEController); used when the host implements it.
type imeController interface {
	SetIMEPosition(x, y int)
	SetIMEEnabled(enabled bool)
}

// syncIME tells the platform whether the focused widget takes IME input
// and where its caret is, so candidate windows open next to the text. It
// runs after paint (render thread); the platform calls are queued for the
// main thread like other window operations.
func (a *App) syncIME(scale float32) {
	if _, ok := any(a.gpuApp).(imeController); !ok {
		return
	}
	fm := a.buildOwner.Focus()
	if want := fm.WantsIME(); want != a.imeEnabled {
		a.imeEnabled = want
		a.window.do(func(g *gogpu.App) { any(g).(imeController).SetIMEEnabled(want) })
	}
	if r, ok := fm.IMERect(); ok {
		x, y := int(r.X*scale+0.5), int(r.Bottom()*scale+0.5)
		if x != a.imePos[0] || y != a.imePos[1] {
			a.imePos = [2]int{x, y}
			a.window.do(func(g *gogpu.App) { any(g).(imeController).SetIMEPosition(x, y) })
		}
	}
}
