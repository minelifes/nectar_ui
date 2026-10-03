// Package undo is a generic undo/redo history: record each change as a
// pair of functions, group several into one step, merge rapid small edits
// (typing) into one, and track whether the document has unsaved changes.
//
//	h := undo.New(200)
//	h.Do(undo.Action{
//	    Label: "Delete line",
//	    Do:    func() { doc.DeleteLine(n) },
//	    Undo:  func() { doc.InsertLine(n, text) },
//	})
//	h.Undo()
//
// A History is a Listenable: menus and toolbars can Listen to it to update
// their Undo/Redo items. It's safe for concurrent use, but Do/Undo/Redo
// call the actions on the calling goroutine (the UI's).
package undo

import (
	"sync"
	"time"

	"github.com/minelifes/nectar_ui/ui/mvvm"
)

// Action is one reversible change.
type Action struct {
	// Label names the change for menus ("Undo Typing").
	Label string
	// Do applies (or re-applies) the change.
	Do func()
	// Undo reverts it.
	Undo func()
	// MergeKey lets consecutive actions with the same non-empty key, done
	// within MergeWindow of each other, become one undo step (typing a
	// word is one step, not one per letter).
	MergeKey string
}

// MergeWindow is how close together mergeable actions must be.
var MergeWindow = time.Second

type step struct {
	label    string
	do, undo []func() // applied in order; undone in reverse
	mergeKey string
	at       time.Time
}

func (s *step) redo() {
	for _, f := range s.do {
		if f != nil {
			f()
		}
	}
}

func (s *step) revert() {
	for i := len(s.undo) - 1; i >= 0; i-- {
		if s.undo[i] != nil {
			s.undo[i]()
		}
	}
}

// History is an undo/redo stack.
type History struct {
	mvvm.Notifier
	mu    sync.Mutex
	limit int
	done  []*step
	redo  []*step
	group []*step // open groups (BeginGroup), innermost last
	clean int     // len(done) at the last MarkClean; -1 = unreachable
	now   func() time.Time
}

// New returns a history keeping at most limit steps (0 = unlimited).
func New(limit int) *History { return &History{limit: limit, now: time.Now} }

// Do runs a.Do and records it.
func (h *History) Do(a Action) {
	if a.Do != nil {
		a.Do()
	}
	h.Record(a)
}

// Record adds an action that was already applied.
func (h *History) Record(a Action) {
	h.mu.Lock()
	s := &step{label: a.Label, do: []func(){a.Do}, undo: []func(){a.Undo}, mergeKey: a.MergeKey, at: h.now()}
	if n := len(h.group); n > 0 {
		g := h.group[n-1]
		g.do = append(g.do, s.do...)
		g.undo = append(g.undo, s.undo...)
		h.mu.Unlock()
		return
	}
	h.push(s)
	h.mu.Unlock()
	h.Notify()
}

func (h *History) push(s *step) {
	h.redo = nil
	if h.clean > len(h.done) {
		h.clean = -1 // the clean state was undone and is now gone
	}
	if n := len(h.done); n > 0 && s.mergeKey != "" && n != h.clean {
		top := h.done[n-1]
		if top.mergeKey == s.mergeKey && s.at.Sub(top.at) <= MergeWindow {
			top.do = append(top.do, s.do...)
			top.undo = append(top.undo, s.undo...)
			top.at = s.at
			return
		}
	}
	h.done = append(h.done, s)
	if h.limit > 0 && len(h.done) > h.limit {
		drop := len(h.done) - h.limit
		h.done = append(h.done[:0:0], h.done[drop:]...)
		h.clean -= drop
		if h.clean < 0 {
			h.clean = -1
		}
	}
}

// BeginGroup starts collecting actions into one step labeled label, until
// the matching EndGroup. Groups nest; the outermost makes the step.
func (h *History) BeginGroup(label string) {
	h.mu.Lock()
	h.group = append(h.group, &step{label: label})
	h.mu.Unlock()
}

// EndGroup closes the innermost group.
func (h *History) EndGroup() {
	h.mu.Lock()
	n := len(h.group)
	if n == 0 {
		h.mu.Unlock()
		return
	}
	g := h.group[n-1]
	h.group = h.group[:n-1]
	if len(g.do) == 0 {
		h.mu.Unlock()
		return
	}
	if n > 1 {
		outer := h.group[n-2]
		outer.do = append(outer.do, g.do...)
		outer.undo = append(outer.undo, g.undo...)
		h.mu.Unlock()
		return
	}
	g.at = h.now()
	h.push(g)
	h.mu.Unlock()
	h.Notify()
}

// Group runs fn with its actions recorded as one step.
func (h *History) Group(label string, fn func()) {
	h.BeginGroup(label)
	defer h.EndGroup()
	fn()
}

// Undo reverts the last step. Returns false if there's nothing to undo.
func (h *History) Undo() bool {
	h.mu.Lock()
	n := len(h.done)
	if n == 0 || len(h.group) > 0 {
		h.mu.Unlock()
		return false
	}
	s := h.done[n-1]
	h.done = h.done[:n-1]
	h.redo = append(h.redo, s)
	h.mu.Unlock()
	s.revert()
	h.Notify()
	return true
}

// Redo re-applies the last undone step.
func (h *History) Redo() bool {
	h.mu.Lock()
	n := len(h.redo)
	if n == 0 || len(h.group) > 0 {
		h.mu.Unlock()
		return false
	}
	s := h.redo[n-1]
	h.redo = h.redo[:n-1]
	h.done = append(h.done, s)
	h.mu.Unlock()
	s.redo()
	h.Notify()
	return true
}

// CanUndo / CanRedo report whether Undo / Redo would do something.
func (h *History) CanUndo() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.done) > 0 }
func (h *History) CanRedo() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.redo) > 0 }

// UndoLabel / RedoLabel name the step Undo / Redo would act on ("").
func (h *History) UndoLabel() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.done); n > 0 {
		return h.done[n-1].label
	}
	return ""
}

func (h *History) RedoLabel() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.redo); n > 0 {
		return h.redo[n-1].label
	}
	return ""
}

// MarkClean records the current state as saved.
func (h *History) MarkClean() {
	h.mu.Lock()
	h.clean = len(h.done)
	h.mu.Unlock()
	h.Notify()
}

// Dirty reports whether the state differs from the last MarkClean (or from
// the start, if never marked).
func (h *History) Dirty() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clean != len(h.done)
}

// Clear forgets all steps (the current state stays as it is).
func (h *History) Clear() {
	h.mu.Lock()
	dirty := h.clean != len(h.done)
	h.done, h.redo, h.group = nil, nil, nil
	h.clean = 0
	if dirty {
		h.clean = -1
	}
	h.mu.Unlock()
	h.Notify()
}

// SetClock replaces the time source used for merging (tests).
func (h *History) SetClock(now func() time.Time) { h.mu.Lock(); h.now = now; h.mu.Unlock() }
