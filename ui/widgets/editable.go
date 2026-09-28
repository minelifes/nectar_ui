package widgets

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// TextEditingController holds the text and selection of an editable field.
// Share one between a field and your code to read or set the value.
type TextEditingController struct {
	text         string
	base, extent int
	listeners    []func()
}

// NewTextController creates a controller with initial text (caret at end).
func NewTextController(initial string) *TextEditingController {
	return &TextEditingController{text: initial, base: len(initial), extent: len(initial)}
}

func (c *TextEditingController) Text() string { return c.text }

// Selection returns the selection as byte offsets (base, extent).
func (c *TextEditingController) Selection() (base, extent int) { return c.base, c.extent }

// SetText replaces the text and puts the caret at the end.
func (c *TextEditingController) SetText(s string) {
	c.text, c.base, c.extent = s, len(s), len(s)
	c.notify()
}

// SetSelection sets the selection (clamped to rune boundaries).
func (c *TextEditingController) SetSelection(base, extent int) {
	c.base, c.extent = clampOffset(c.text, base), clampOffset(c.text, extent)
	c.notify()
}

// AddListener registers fn to run after every change.
func (c *TextEditingController) AddListener(fn func()) { c.listeners = append(c.listeners, fn) }

func (c *TextEditingController) notify() {
	for _, fn := range c.listeners {
		fn()
	}
}

func (c *TextEditingController) selRange() (int, int) {
	return min(c.base, c.extent), max(c.base, c.extent)
}

// replaceSelection inserts s over the selection.
func (c *TextEditingController) replaceSelection(s string) {
	lo, hi := c.selRange()
	c.text = c.text[:lo] + s + c.text[hi:]
	c.base = lo + len(s)
	c.extent = c.base
}

func clampOffset(s string, i int) int {
	i = max(0, min(i, len(s)))
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

// EditableText is the low-level text input: caret, selection, keyboard and
// clipboard. It has no decoration; material.TextField builds on it.
type EditableText struct {
	Controller     *TextEditingController // nil = internal
	Focus          *FocusNode             // nil = internal
	Style          text.Style
	CursorColor    geom.Color
	SelectionColor geom.Color
	Multiline      bool
	MinLines       int
	Obscure        bool // password dots
	ReadOnly       bool
	Autofocus      bool
	OnChanged      func(string)
	OnSubmitted    func(string)
	OnFocusChange  func(bool)
}

func (EditableText) CreateState() State { return &editableState{} }

type editableState struct {
	StateBase
	ctrl      *TextEditingController
	node      *FocusNode
	caretOn   bool
	stopBlink chan struct{}
	ro        *render.RenderEditable
	dragFrom  int
}

func (s *editableState) w() EditableText { return WidgetOf[EditableText](s) }

func (s *editableState) InitState() {
	w := s.w()
	s.ctrl = w.Controller
	if s.ctrl == nil {
		s.ctrl = NewTextController("")
	}
	s.ctrl.AddListener(func() {
		if s.Mounted() {
			s.SetState(nil)
		}
	})
	s.node = w.Focus
	if s.node == nil {
		s.node = &FocusNode{}
	}
	s.node.UnfocusOnTapOutside = true
	s.node.OnKey = s.onKey
	s.node.OnText = s.onText
	s.node.OnFocusChange = s.onFocus
}

func (s *editableState) Dispose() { s.blink(false) }

func (s *editableState) onFocus(f bool) {
	s.blink(f)
	if cb := s.w().OnFocusChange; cb != nil {
		cb(f)
	}
	s.SetState(nil)
}

// blink starts/stops the caret blink loop (a goroutine posting to the UI).
func (s *editableState) blink(on bool) {
	if s.stopBlink != nil {
		close(s.stopBlink)
		s.stopBlink = nil
	}
	s.caretOn = on
	if !on {
		return
	}
	stop := make(chan struct{})
	s.stopBlink = stop
	go func() {
		t := time.NewTicker(530 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.Post(func() { s.caretOn = !s.caretOn })
			}
		}
	}()
}

// restartBlink shows the caret immediately after an edit.
func (s *editableState) restartBlink() {
	if s.node.HasFocus() {
		s.blink(true)
	}
}

func (s *editableState) changed() {
	s.restartBlink()
	s.ctrl.notify()
	if cb := s.w().OnChanged; cb != nil {
		cb(s.ctrl.text)
	}
}

func (s *editableState) onText(t string) {
	if s.w().ReadOnly {
		return
	}
	t = strings.Map(func(r rune) rune {
		if r == '\n' && s.w().Multiline {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, t)
	if t == "" {
		return
	}
	s.ctrl.replaceSelection(t)
	s.changed()
}

func (s *editableState) onKey(e KeyEvent) bool {
	c := s.ctrl
	w := s.w()
	move := func(to int) {
		to = clampOffset(c.text, to)
		c.extent = to
		if !e.Mods.Shift() {
			c.base = to
		}
		s.restartBlink()
		c.notify()
	}
	lo, hi := c.selRange()
	switch {
	case e.Mods.Shortcut() && e.Key == KeyA:
		c.base, c.extent = 0, len(c.text)
		c.notify()
	case e.Mods.Shortcut() && e.Key == KeyC:
		s.copy(false)
	case e.Mods.Shortcut() && e.Key == KeyX:
		s.copy(!w.ReadOnly)
	case e.Mods.Shortcut() && e.Key == KeyV:
		if cb := s.Context().Owner().Clipboard; cb != nil && !w.ReadOnly {
			if t, err := cb.ReadText(); err == nil {
				s.onText(t)
			}
		}
	case e.Key == KeyBackspace && !w.ReadOnly:
		if lo == hi {
			lo = text.PrevRune(c.text, lo)
			if e.Mods.Alt() || e.Mods.Shortcut() {
				lo = wordStart(c.text, hi)
			}
		}
		c.text = c.text[:lo] + c.text[hi:]
		c.base, c.extent = lo, lo
		s.changed()
	case e.Key == KeyDelete && !w.ReadOnly:
		if lo == hi {
			hi = text.NextRune(c.text, hi)
		}
		c.text = c.text[:lo] + c.text[hi:]
		c.base, c.extent = lo, lo
		s.changed()
	case e.Key == KeyLeft:
		if lo != hi && !e.Mods.Shift() {
			move(lo)
		} else if e.Mods.Alt() {
			move(wordStart(c.text, c.extent))
		} else {
			move(text.PrevRune(c.text, c.extent))
		}
	case e.Key == KeyRight:
		if lo != hi && !e.Mods.Shift() {
			move(hi)
		} else if e.Mods.Alt() {
			move(wordEnd(c.text, c.extent))
		} else {
			move(text.NextRune(c.text, c.extent))
		}
	case e.Key == KeyHome:
		if s.ro != nil {
			st, _ := s.ro.LineBounds(s.displayToText(c.extent, true))
			move(s.displayToText(st, false))
		}
	case e.Key == KeyEnd:
		if s.ro != nil {
			_, en := s.ro.LineBounds(s.displayToText(c.extent, true))
			move(s.displayToText(en, false))
		}
	case (e.Key == KeyUp || e.Key == KeyDown) && w.Multiline && s.ro != nil:
		dir := 1
		if e.Key == KeyUp {
			dir = -1
		}
		if to, ok := s.ro.VerticalMove(s.displayToText(c.extent, true), dir); ok {
			move(s.displayToText(to, false))
		}
	case e.Key == KeyEnter:
		if w.Multiline && !w.ReadOnly {
			c.replaceSelection("\n")
			s.changed()
		} else if w.OnSubmitted != nil {
			w.OnSubmitted(c.text)
		}
	default:
		return false
	}
	return true
}

func (s *editableState) copy(cut bool) {
	lo, hi := s.ctrl.selRange()
	cb := s.Context().Owner().Clipboard
	if lo == hi || cb == nil || s.w().Obscure {
		return
	}
	_ = cb.WriteText(s.ctrl.text[lo:hi])
	if cut {
		s.ctrl.replaceSelection("")
		s.changed()
	}
}

func wordStart(s string, i int) int {
	for i > 0 && unicode.IsSpace(rune(s[i-1])) {
		i = text.PrevRune(s, i)
	}
	for i > 0 {
		p := text.PrevRune(s, i)
		r, _ := utf8.DecodeRuneInString(s[p:])
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			break
		}
		i = p
	}
	return i
}

func wordEnd(s string, i int) int {
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i = text.NextRune(s, i)
	}
	for i < len(s) {
		r, _ := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			break
		}
		i = text.NextRune(s, i)
	}
	return i
}

// Obscured text is drawn as one bullet per rune, so offsets differ between
// the real text and what's displayed.
const bullet = "•"

func (s *editableState) display() string {
	if !s.w().Obscure {
		return s.ctrl.text
	}
	return strings.Repeat(bullet, utf8.RuneCountInString(s.ctrl.text))
}

// displayToText converts offsets between real and displayed text.
// toDisplay=true maps text→display.
func (s *editableState) displayToText(i int, toDisplay bool) int {
	if !s.w().Obscure {
		return i
	}
	if toDisplay {
		return utf8.RuneCountInString(s.ctrl.text[:i]) * len(bullet)
	}
	n := i / len(bullet)
	off := 0
	for k := 0; k < n && off < len(s.ctrl.text); k++ {
		off = text.NextRune(s.ctrl.text, off)
	}
	return off
}

func (s *editableState) Build(BuildContext) Widget {
	w := s.w()
	c := s.ctrl
	style := w.Style
	if def, ok := DependOn[DefaultTextStyle](s.Context()); ok {
		style = mergeStyle(def.Style, style)
	}
	cursor := w.CursorColor
	if cursor == (geom.Color{}) {
		cursor = style.Resolved().Color
	}
	sel := w.SelectionColor
	if sel == (geom.Color{}) {
		sel = cursor.WithAlpha(0.3)
	}
	minLines := max(w.MinLines, 1)
	ed := editable{
		text: s.display(), style: style,
		base: s.displayToText(c.base, true), extent: s.displayToText(c.extent, true),
		focused: s.node.HasFocus(), caret: s.caretOn,
		cursor: cursor, selection: sel, minLines: minLines, state: s,
	}
	return Focus{Node: s.node, Autofocus: w.Autofocus, Child: MouseRegion{Cursor: CursorText, Child: GestureDetector{
		OnTapDown: func(d TapDetails) {
			s.node.RequestFocus()
			if s.ro != nil {
				i := s.displayToText(s.ro.OffsetAt(d.Local), false)
				s.dragFrom = i
				c.base, c.extent = i, i
				s.restartBlink()
				c.notify()
			}
		},
		OnTap: func() { s.node.RequestFocus() },
		OnPanStart: func(d DragDetails) {
			s.node.RequestFocus()
			if s.ro != nil {
				s.dragFrom = s.displayToText(s.ro.OffsetAt(d.Local), false)
			}
		},
		OnPanUpdate: func(d DragDetails) {
			if s.ro != nil {
				c.base = s.dragFrom
				c.extent = s.displayToText(s.ro.OffsetAt(d.Local), false)
				c.notify()
			}
		},
		Child: ed,
	}}}
}

// editable is the render-object widget behind EditableText.
type editable struct {
	text              string
	style             text.Style
	base, extent      int
	focused, caret    bool
	cursor, selection geom.Color
	minLines          int
	state             *editableState
}

func (w editable) CreateRenderObject(BuildContext) render.RenderObject {
	r := &render.RenderEditable{}
	w.UpdateRenderObject(nil, r)
	return r
}

func (w editable) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	r := ro.(*render.RenderEditable)
	w.state.ro = r
	r.Update(w.text, w.style, w.base, w.extent, w.focused, w.caret, w.cursor, w.selection, w.minLines)
}
