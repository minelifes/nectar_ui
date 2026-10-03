// Package tasks runs work in the background with progress, status text and
// cancellation, and reports back on the UI goroutine: indexing a project,
// building, downloading. A Manager is a Listenable (status bars and
// progress lists Listen to it); each Task is one too.
//
//	mgr := tasks.NewManager(app.Post) // or tasks.ManagerFor(ctx)
//	t := mgr.Run("Indexing", func(ctx context.Context, p *tasks.Progress) error {
//	    for i, f := range files {
//	        if ctx.Err() != nil {
//	            return ctx.Err()
//	        }
//	        p.Report(float32(i)/float32(len(files)), f)
//	        index(f)
//	    }
//	    return nil
//	})
//	t.Then(func(err error) { ... }) // on the UI goroutine
package tasks

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// State is where a task is in its life.
type State uint8

const (
	Running State = iota
	Done
	Failed
	Cancelled
)

func (s State) String() string {
	return [...]string{"running", "done", "failed", "cancelled"}[s]
}

// Task is one piece of background work.
type Task struct {
	mvvm.Notifier
	id      int
	title   string
	mgr     *Manager
	cancel  context.CancelFunc
	started time.Time
	done    chan struct{}

	mu       sync.Mutex
	state    State
	progress float32 // < 0: unknown (indeterminate)
	status   string
	err      error
	then     []func(error)
}

// ID is unique within the manager.
func (t *Task) ID() int { return t.id }

// Title names the task ("Indexing").
func (t *Task) Title() string { return t.title }

// Started returns when the task started.
func (t *Task) Started() time.Time { return t.started }

// State returns the task's state.
func (t *Task) State() State { t.mu.Lock(); defer t.mu.Unlock(); return t.state }

// Progress returns how far it is, 0..1, or -1 when unknown.
func (t *Task) Progress() float32 { t.mu.Lock(); defer t.mu.Unlock(); return t.progress }

// Status returns the latest status text ("parsing main.go").
func (t *Task) Status() string { t.mu.Lock(); defer t.mu.Unlock(); return t.status }

// Err returns the task's error once it failed or was cancelled.
func (t *Task) Err() error { t.mu.Lock(); defer t.mu.Unlock(); return t.err }

// Cancel asks the task to stop (its context is cancelled).
func (t *Task) Cancel() { t.cancel() }

// Done is closed when the task ends.
func (t *Task) Done() <-chan struct{} { return t.done }

// Wait blocks until the task ends and returns its error. Don't call it on
// the UI goroutine; use Then.
func (t *Task) Wait() error { <-t.done; return t.Err() }

// Then runs fn with the task's error (nil on success) on the UI goroutine
// when it ends (right away, posted, if it already has).
func (t *Task) Then(fn func(err error)) *Task {
	t.mu.Lock()
	if t.state == Running {
		t.then = append(t.then, fn)
		t.mu.Unlock()
		return t
	}
	err := t.err
	t.mu.Unlock()
	t.mgr.post(func() { fn(err) })
	return t
}

// Progress is how a task's function reports on itself.
type Progress struct{ t *Task }

// Report sets the fraction done (0..1; < 0 = unknown) and a status text.
// Cheap enough to call often: subscribers are told at most once per frame.
func (p *Progress) Report(fraction float32, status string) {
	t := p.t
	t.mu.Lock()
	if fraction > 1 {
		fraction = 1
	}
	changed := t.progress != fraction || t.status != status
	t.progress, t.status = fraction, status
	t.mu.Unlock()
	if changed {
		t.Notify()
		t.mgr.Notify()
	}
}

// Manager runs tasks and lists the running ones.
type Manager struct {
	mvvm.Notifier
	post func(func())

	mu      sync.Mutex
	running []*Task
	next    int
	// Keep, when > 0, keeps that many finished tasks in Recent.
	Keep   int
	recent []*Task
}

// NewManager makes a manager whose callbacks (Then) run through post: pass
// the app's Post (or BuildOwner.Post) so they run on the UI goroutine.
// nil runs them on the task's goroutine.
func NewManager(post func(func())) *Manager {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	return &Manager{post: post, Keep: 20}
}

// Run starts fn on a new goroutine as a task called title.
func (m *Manager) Run(title string, fn func(ctx context.Context, p *Progress) error) *Task {
	return m.RunContext(context.Background(), title, fn)
}

// RunContext is Run with a parent context (cancelling it cancels the task).
func (m *Manager) RunContext(parent context.Context, title string, fn func(ctx context.Context, p *Progress) error) *Task {
	ctx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	m.next++
	t := &Task{id: m.next, title: title, mgr: m, cancel: cancel, started: time.Now(), done: make(chan struct{}), progress: -1}
	m.running = append(m.running, t)
	m.mu.Unlock()
	m.Notify()
	go func() {
		err := run(ctx, fn, &Progress{t})
		cancel()
		state := Done
		switch {
		case err != nil && (errors.Is(err, context.Canceled) || ctx.Err() != nil && errors.Is(err, ctx.Err())):
			state = Cancelled
		case err != nil:
			state = Failed
		}
		t.mu.Lock()
		t.state, t.err = state, err
		if state == Done {
			t.progress = 1
		}
		then := t.then
		t.then = nil
		t.mu.Unlock()
		m.mu.Lock()
		m.running = slices.DeleteFunc(m.running, func(x *Task) bool { return x == t })
		if m.Keep > 0 {
			m.recent = append(m.recent, t)
			if len(m.recent) > m.Keep {
				m.recent = m.recent[len(m.recent)-m.Keep:]
			}
		}
		m.mu.Unlock()
		close(t.done)
		t.Notify()
		m.Notify()
		for _, fn := range then {
			fn := fn
			m.post(func() { fn(err) })
		}
	}()
	return t
}

// run calls fn, turning a panic into an error so one bad task can't take
// the app down.
func run(ctx context.Context, fn func(context.Context, *Progress) error, p *Progress) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r}
		}
	}()
	return fn(ctx, p)
}

// PanicError is the error of a task whose function panicked.
type PanicError struct{ Value any }

func (e *PanicError) Error() string { return "tasks: task panicked: " + toString(e.Value) }

func toString(v any) string {
	switch x := v.(type) {
	case error:
		return x.Error()
	case string:
		return x
	}
	return "(non-string panic value)"
}

// Running returns the running tasks, oldest first.
func (m *Manager) Running() []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.running)
}

// Recent returns finished tasks, oldest first (up to Keep).
func (m *Manager) Recent() []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.recent)
}

// CancelAll cancels every running task.
func (m *Manager) CancelAll() {
	for _, t := range m.Running() {
		t.Cancel()
	}
}

// Overall returns the combined progress of the running tasks (-1 if any is
// indeterminate, 1 when none run) and how many there are.
func (m *Manager) Overall() (float32, int) {
	ts := m.Running()
	if len(ts) == 0 {
		return 1, 0
	}
	var sum float32
	for _, t := range ts {
		p := t.Progress()
		if p < 0 {
			return -1, len(ts)
		}
		sum += p
	}
	return sum / float32(len(ts)), len(ts)
}

// ManagerFor returns the task manager of the widget tree at ctx, created on
// first use with the tree's Post (one per window).
func ManagerFor(ctx widgets.BuildContext) *Manager {
	o := ctx.Owner()
	managersMu.Lock()
	defer managersMu.Unlock()
	if m, ok := managers[o]; ok {
		return m
	}
	m := NewManager(o.Post)
	managers[o] = m
	return m
}

var (
	managersMu sync.Mutex
	managers   = map[*widgets.BuildOwner]*Manager{}
)
