package widgets

import (
	"slices"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
)

// Draggable lets the user drag Data out of Child onto a DragTarget. While
// dragging, Feedback (default: Child, translucent) follows the pointer in
// the overlay. Needs an Overlay above it (material.App has one).
type Draggable struct {
	Data any
	// Feedback is drawn under the pointer while dragging.
	Feedback Widget
	// ChildWhileDragging replaces Child during the drag (nil = Child).
	ChildWhileDragging Widget
	OnDragStarted      func()
	// OnDragEnd reports whether a target accepted the data.
	OnDragEnd func(accepted bool)
	Child     Widget
}

// DragTarget receives data dropped by a Draggable.
type DragTarget struct {
	// OnWillAccept decides whether the target takes data (nil = yes).
	OnWillAccept func(data any) bool
	// OnAccept gets the dropped data and the drop point (local).
	OnAccept func(data any, local geom.Offset)
	// OnMove reports an accepted drag moving over the target (local).
	OnMove func(data any, local geom.Offset)
	// OnLeave reports a drag leaving the target without dropping.
	OnLeave func(data any)
	// Builder draws the target; candidate is the data hovering over it
	// that it would accept (nil when none).
	Builder func(ctx BuildContext, candidate any) Widget
}

// dragManager tracks the drag in progress and the mounted targets.
type dragManager struct {
	targets []*dragTargetState
	active  *dragSession
}

type dragSession struct {
	data   any
	pos    geom.Offset // pointer, window coordinates
	grab   geom.Offset // pointer position inside the dragged child
	over   *dragTargetState
	entry  *OverlayEntry
	source *draggableState
}

func (o *BuildOwner) drags() *dragManager {
	if o.drag == nil {
		o.drag = &dragManager{}
	}
	return o.drag
}

// IsDragging reports whether a Draggable is being dragged.
func (o *BuildOwner) IsDragging() bool { return o.drag != nil && o.drag.active != nil }

// targetAt returns the topmost target under pos that accepts data.
func (m *dragManager) targetAt(pos geom.Offset, data any) *dragTargetState {
	var best *dragTargetState
	bestDepth := -1
	for _, t := range m.targets {
		ro := t.ro()
		if ro == nil || ro.Base().Owner() == nil || !visibleRect(ro).Contains(pos) {
			continue
		}
		if w := WidgetOf[DragTarget](t); w.OnWillAccept != nil && !w.OnWillAccept(data) {
			continue
		}
		// Deeper wins; among equals the later mounted (painted on top).
		if d := ro.Base().Depth(); d >= bestDepth {
			best, bestDepth = t, d
		}
	}
	return best
}

// visibleRect returns ro's window rect cut by the clipping ancestors
// (scroll views, ClipRect), so a target scrolled out of view isn't hit.
func visibleRect(ro render.RenderObject) geom.Rect {
	r := geom.RectFrom(render.GlobalOrigin(ro), ro.Base().Size())
	for p := ro.Base().Parent(); p != nil; p = p.Base().Parent() {
		switch p.(type) {
		case *render.RenderViewport, *render.RenderViewport2D, *render.RenderClipRect:
			r = r.Intersect(geom.RectFrom(render.GlobalOrigin(p), p.Base().Size()))
		}
	}
	return r
}

func (m *dragManager) move(pos geom.Offset) {
	s := m.active
	if s == nil {
		return
	}
	s.pos = pos
	t := m.targetAt(pos, s.data)
	if t != s.over {
		if old := s.over; old != nil {
			old.setCandidate(nil)
			if cb := WidgetOf[DragTarget](old).OnLeave; cb != nil {
				cb(s.data)
			}
		}
		s.over = t
		if t != nil {
			t.setCandidate(s.data)
		}
	}
	if t != nil {
		if cb := WidgetOf[DragTarget](t).OnMove; cb != nil {
			cb(s.data, pos.Sub(render.GlobalOrigin(t.ro())))
		}
	}
	if s.entry != nil {
		s.entry.MarkNeedsBuild()
	}
}

func (m *dragManager) end(drop bool) {
	s := m.active
	if s == nil {
		return
	}
	m.active = nil
	if s.entry != nil {
		s.entry.Remove()
	}
	accepted := false
	if t := s.over; t != nil {
		t.setCandidate(nil)
		w := WidgetOf[DragTarget](t)
		if drop && t.Mounted() {
			accepted = true
			if w.OnAccept != nil {
				w.OnAccept(s.data, s.pos.Sub(render.GlobalOrigin(t.ro())))
			}
		} else if w.OnLeave != nil {
			w.OnLeave(s.data)
		}
	}
	if src := s.source; src != nil && src.Mounted() {
		src.SetState(func() { src.dragging = false })
		if cb := WidgetOf[Draggable](src).OnDragEnd; cb != nil {
			cb(accepted)
		}
	}
}

// ---------------------------------------------------------------------------

func (Draggable) CreateState() State { return &draggableState{} }

type draggableState struct {
	StateBase
	dragging bool
}

func (s *draggableState) Dispose() {
	if m := s.Context().Owner().drag; m != nil && m.active != nil && m.active.source == s {
		m.end(false)
	}
}

func (s *draggableState) start(d DragDetails) {
	w := WidgetOf[Draggable](s)
	owner := s.Context().Owner()
	m := owner.drags()
	if m.active != nil {
		m.end(false)
	}
	ro := s.Context().RenderObject()
	var size geom.Size
	var origin geom.Offset
	if ro != nil {
		size, origin = ro.Base().Size(), render.GlobalOrigin(ro)
	}
	start := d.Global.Sub(d.Total) // where the pointer went down
	sess := &dragSession{data: w.Data, pos: d.Global, grab: start.Sub(origin), source: s}
	m.active = sess
	feedback := w.Feedback
	if feedback == nil {
		feedback = Opacity{Opacity: 0.7, Child: SizedBox{Width: size.W, Height: size.H, Child: w.Child}}
	}
	if ov := OverlayOf(s.Context()); ov != nil {
		sess.entry = &OverlayEntry{Builder: func(BuildContext) Widget {
			p := sess.pos.Sub(sess.grab)
			return Stack{Expand: true, Children: []Widget{
				Positioned{Left: At(p.X), Top: At(p.Y), Child: IgnorePointer{Ignoring: true, Child: feedback}},
			}}
		}}
		ov.Insert(sess.entry)
	}
	s.SetState(func() { s.dragging = true })
	if w.OnDragStarted != nil {
		w.OnDragStarted()
	}
	m.move(d.Global)
}

func (s *draggableState) Build(BuildContext) Widget {
	w := WidgetOf[Draggable](s)
	child := w.Child
	if s.dragging && w.ChildWhileDragging != nil {
		child = w.ChildWhileDragging
	}
	m := func() *dragManager { return s.Context().Owner().drags() }
	return GestureDetector{
		OnPanStart:  s.start,
		OnPanUpdate: func(d DragDetails) { m().move(d.Global) },
		OnPanEnd:    func(DragDetails) { m().end(true) },
		Child:       child,
	}
}

// ---------------------------------------------------------------------------

func (DragTarget) CreateState() State { return &dragTargetState{} }

type dragTargetState struct {
	StateBase
	candidate any
	hovering  bool
}

func (s *dragTargetState) ro() render.RenderObject { return s.Context().RenderObject() }

func (s *dragTargetState) InitState() {
	m := s.Context().Owner().drags()
	m.targets = append(m.targets, s)
}

func (s *dragTargetState) Dispose() {
	m := s.Context().Owner().drags()
	m.targets = slices.DeleteFunc(m.targets, func(t *dragTargetState) bool { return t == s })
	if m.active != nil && m.active.over == s {
		m.active.over = nil
	}
}

func (s *dragTargetState) setCandidate(data any) {
	if s.Mounted() {
		s.SetState(func() { s.candidate, s.hovering = data, data != nil })
	}
}

func (s *dragTargetState) Build(ctx BuildContext) Widget {
	w := WidgetOf[DragTarget](s)
	var cand any
	if s.hovering {
		cand = s.candidate
	}
	if w.Builder == nil {
		return SizedBox{}
	}
	return w.Builder(ctx, cand)
}

// ---------------------------------------------------------------------------
// Files dropped from the OS

// FileDropTarget receives files dragged from the system file manager and
// dropped on it. While files are dragged over it, OnEnter, OnMove and
// OnLeave report it (to highlight the drop zone); the innermost target
// under the pointer gets them. OnEnter's paths can be empty: on X11 the
// file list only arrives with the drop.
type FileDropTarget struct {
	OnDrop  func(paths []string, local geom.Offset)
	OnEnter func(paths []string, local geom.Offset)
	OnMove  func(local geom.Offset)
	OnLeave func()
	Child   Widget
}

// fileDragState is the OS file drag in progress.
type fileDragState struct {
	paths []string
	over  *fileDropState
}

func (FileDropTarget) CreateState() State { return &fileDropState{} }

type fileDropState struct{ StateBase }

func (s *fileDropState) InitState() {
	o := s.Context().Owner()
	o.fileTargets = append(o.fileTargets, s)
}

func (s *fileDropState) Dispose() {
	o := s.Context().Owner()
	o.fileTargets = slices.DeleteFunc(o.fileTargets, func(t *fileDropState) bool { return t == s })
	if o.fileDrag.over == s {
		o.fileDrag.over = nil
	}
}

// fileTargetAt returns the innermost FileDropTarget under pos.
func (o *BuildOwner) fileTargetAt(pos geom.Offset) *fileDropState {
	var best *fileDropState
	bestDepth := -1
	for _, t := range o.fileTargets {
		ro := t.Context().RenderObject()
		if ro == nil || ro.Base().Owner() == nil || !visibleRect(ro).Contains(pos) {
			continue
		}
		if d := ro.Base().Depth(); d >= bestDepth {
			best, bestDepth = t, d
		}
	}
	return best
}

func localTo(t *fileDropState, pos geom.Offset) geom.Offset {
	return pos.Sub(render.GlobalOrigin(t.Context().RenderObject()))
}

// FileDragEnter reports files from the OS entering the window at pos (the
// app calls it; tests can too). Some platforms (X11) report neither the
// files nor the position on entry: a pathless enter at the origin waits
// for the first move.
func (o *BuildOwner) FileDragEnter(paths []string, pos geom.Offset) {
	o.fileDrag.paths = paths
	o.fileDrag.over = nil
	if len(paths) == 0 && pos == (geom.Offset{}) {
		return
	}
	o.FileDragMove(pos)
}

// FileDragMove reports the dragged files moving to pos.
func (o *BuildOwner) FileDragMove(pos geom.Offset) {
	t := o.fileTargetAt(pos)
	fd := &o.fileDrag
	if t != fd.over {
		if old := fd.over; old != nil {
			if cb := WidgetOf[FileDropTarget](old).OnLeave; cb != nil {
				cb()
			}
		}
		fd.over = t
		if t != nil {
			if cb := WidgetOf[FileDropTarget](t).OnEnter; cb != nil {
				cb(fd.paths, localTo(t, pos))
			}
		}
		return
	}
	if t != nil {
		if cb := WidgetOf[FileDropTarget](t).OnMove; cb != nil {
			cb(localTo(t, pos))
		}
	}
}

// FileDragLeave reports the dragged files leaving the window.
func (o *BuildOwner) FileDragLeave() {
	if old := o.fileDrag.over; old != nil {
		if cb := WidgetOf[FileDropTarget](old).OnLeave; cb != nil {
			cb()
		}
	}
	o.fileDrag = fileDragState{}
}

func (s *fileDropState) Build(BuildContext) Widget { return WidgetOf[FileDropTarget](s).Child }

// DropFiles delivers files dropped from the OS at pos (window coordinates,
// logical pixels) to the innermost FileDropTarget there. Returns whether
// one took them. The app calls it; tests can too.
func (o *BuildOwner) DropFiles(paths []string, pos geom.Offset) bool {
	best := o.fileTargetAt(pos)
	// A target the drag was over other than the one dropped on loses it.
	if old := o.fileDrag.over; old != nil && old != best {
		if cb := WidgetOf[FileDropTarget](old).OnLeave; cb != nil {
			cb()
		}
	}
	o.fileDrag = fileDragState{}
	if best == nil {
		return false
	}
	if cb := WidgetOf[FileDropTarget](best).OnDrop; cb != nil {
		cb(paths, localTo(best, pos))
	}
	return true
}
