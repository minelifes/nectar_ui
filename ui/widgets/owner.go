package widgets

import (
	"slices"
	"sync"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/resources"
)

// BuildOwner tracks dirty elements and rebuilds them once per frame.
type BuildOwner struct {
	// OnScheduleFrame is called when the tree needs a new frame. It may be
	// called from any goroutine (via Post).
	OnScheduleFrame func()

	// Now returns the frame time used by animations (default time.Now);
	// tests can replace it to step animations deterministically.
	Now func() time.Time

	// Clipboard is used by text fields for copy/paste (set by the app).
	Clipboard Clipboard

	// Window is the native window hosting the tree (set by the app and
	// the tester); read it with WindowOf(ctx).
	Window Window

	// Resources are the app's files (set by the app and the tester); read
	// them with ResourcesOf(ctx).
	Resources *resources.Set

	dirty   []Element
	tickers map[*Ticker]struct{}
	focus   *FocusManager

	mu     sync.Mutex
	posted []func()
}

// Clipboard reads and writes the system clipboard.
type Clipboard interface {
	ReadText() (string, error)
	WriteText(string) error
}

// NewBuildOwner creates an owner.
func NewBuildOwner() *BuildOwner { return &BuildOwner{} }

func (o *BuildOwner) scheduleBuildFor(e Element) {
	o.dirty = append(o.dirty, e)
	o.requestFrame()
}

func (o *BuildOwner) requestFrame() {
	if o.OnScheduleFrame != nil {
		o.OnScheduleFrame()
	}
}

// Post queues fn to run on the UI goroutine at the start of the next frame.
// Safe to call from any goroutine.
func (o *BuildOwner) Post(fn func()) {
	o.mu.Lock()
	o.posted = append(o.posted, fn)
	o.mu.Unlock()
	o.requestFrame()
}

// FlushBuild runs posted callbacks and rebuilds dirty elements, parents
// before children (a parent rebuild usually rebuilds the child anyway).
func (o *BuildOwner) FlushBuild() {
	o.mu.Lock()
	posted := o.posted
	o.posted = nil
	o.mu.Unlock()
	for _, fn := range posted {
		fn()
	}
	o.tick()

	for len(o.dirty) > 0 {
		batch := o.dirty
		o.dirty = nil
		slices.SortStableFunc(batch, func(a, b Element) int { return a.base().depth - b.base().depth })
		for _, e := range batch {
			if b := e.base(); b.dirty && b.active {
				e.rebuild()
			}
		}
	}
}

// --- root ------------------------------------------------------------------

// rootWidget hosts the app widget inside a RenderView.
type rootWidget struct {
	child      Widget
	background geom.Color
}

func (w rootWidget) ChildWidget() Widget { return w.child }

func (w rootWidget) CreateRenderObject(BuildContext) render.RenderObject {
	return &render.RenderView{Background: w.background}
}

func (rootWidget) MarksOwnPaint() {}
func (w rootWidget) UpdateRenderObject(_ BuildContext, ro render.RenderObject) {
	v := ro.(*render.RenderView)
	if v.Background != w.background {
		v.Background = w.background
		render.MarkNeedsPaint(v)
	}
}

// Root is the top of the element tree.
type Root struct {
	element *singleChildElement
	owner   *BuildOwner
}

// Mount inflates app into a new element tree and attaches its render tree
// to pipeline.
func Mount(app Widget, background geom.Color, owner *BuildOwner, pipeline *render.PipelineOwner) *Root {
	el := &singleChildElement{}
	el.widget = rootWidget{child: app, background: background}
	el.owner = owner
	el.mount(nil)
	pipeline.SetRoot(el.ro)
	return &Root{element: el, owner: owner}
}

// SetApp replaces the app widget (e.g. hot reload of the whole tree).
func (r *Root) SetApp(app Widget, background geom.Color) {
	r.element.update(rootWidget{child: app, background: background})
}

// Reassemble rebuilds the whole tree, keeping every State (Flutter's
// reassemble), for when something the builds read changed behind the
// tree's back: resource files edited on disk, a reloaded config. States
// with a Reassemble() method get it called first, to drop what they
// cached (Image reloads its source). Call it on the UI goroutine.
func (r *Root) Reassemble() {
	var walk func(e Element)
	walk = func(e Element) {
		if se, ok := e.(*statefulElement); ok {
			if s, ok := se.state.(reassembler); ok {
				s.Reassemble()
			}
		}
		e.base().markNeedsBuild()
		e.visitChildren(walk)
	}
	walk(r.element)
}

// Context returns the root BuildContext.
func (r *Root) Context() BuildContext { return r.element }

// Unmount tears down the tree (calls Dispose on states).
func (r *Root) Unmount() { r.element.unmount() }
