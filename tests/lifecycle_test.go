package tests

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// lifeLog records lifecycle events in order.
type lifeLog struct {
	mu  sync.Mutex
	got []string
}

func (l *lifeLog) add(s string) { l.mu.Lock(); l.got = append(l.got, s); l.mu.Unlock() }
func (l *lifeLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.got, " ")
}
func (l *lifeLog) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, g := range l.got {
		if g == s {
			n++
		}
	}
	return n
}

// countingSource is a Listenable that counts its subscribers.
type countingSource struct {
	mvvm.Notifier
	mu   sync.Mutex
	subs int
}

func (c *countingSource) Subscribe(fn func()) func() {
	c.mu.Lock()
	c.subs++
	c.mu.Unlock()
	cancel := c.Notifier.Subscribe(fn)
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.subs--
			c.mu.Unlock()
			cancel()
		})
	}
}

func (c *countingSource) Len() int { c.mu.Lock(); defer c.mu.Unlock(); return c.subs }

type closer struct {
	log  *lifeLog
	name string
}

func (c closer) Close() error { c.log.add("close:" + c.name); return nil }

type lifeChild struct {
	log    *lifeLog
	label  string
	source *countingSource
}

func (c lifeChild) CreateState() w.State { return &lifeChildState{} }

type lifeChildState struct {
	w.StateBase
	builds, heard int
	cancelled     context.Context
}

func (s *lifeChildState) InitState() {
	c := w.WidgetOf[lifeChild](s)
	c.log.add("init")
	s.Own(func() { c.log.add("release:func") })
	s.Own(closer{c.log, "file"})
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelled = ctx
	s.Own(cancel)
	t := time.NewTimer(time.Hour)
	s.Own(t)
	s.Listen(c.source, func() { s.SetState(func() { s.heard++ }) })
}

func (s *lifeChildState) OnDestroy() {
	c := w.WidgetOf[lifeChild](s)
	c.log.add("destroy")
	if s.Destroyed() {
		c.log.add("destroyed-too-early")
	}
}

func (s *lifeChildState) Build(ctx w.BuildContext) w.Widget {
	s.builds++
	return w.Text{Text: fmt.Sprintf("%s heard %d", w.WidgetOf[lifeChild](s).label, s.heard)}
}

// lifeHost shows the child until "remove" is tapped; "rebuild" rebuilds it
// with a new label (same state).
type lifeHost struct {
	log    *lifeLog
	source *countingSource
	child  **lifeChildState
}

func (h lifeHost) CreateState() w.State { return &lifeHostState{} }

type lifeHostState struct {
	w.StateBase
	shown bool
	n     int
}

func (s *lifeHostState) InitState() { s.shown = true }

func (s *lifeHostState) Build(ctx w.BuildContext) w.Widget {
	h := w.WidgetOf[lifeHost](s)
	kids := []w.Widget{
		w.GestureDetector{OnTap: func() { s.SetState(func() { s.n++ }) }, Child: w.Text{Text: "rebuild"}},
		w.GestureDetector{OnTap: func() { s.SetState(func() { s.shown = false }) }, Child: w.Text{Text: "remove"}},
	}
	if s.shown {
		kids = append(kids, w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
			return lifeChild{log: h.log, label: "child " + string(rune('a'+s.n%26)), source: h.source}
		}})
	}
	return w.Column{Children: kids}
}

func TestStateLifecycle(t *testing.T) {
	log := &lifeLog{}
	src := &countingSource{}
	tt := tester.New(lifeHost{log: log, source: src}, 400, 300)
	tt.Settle()
	for i := 0; i < 5; i++ {
		tt.TapText("rebuild")
		tt.Settle()
	}
	if _, ok := tt.Find("child f heard 0"); !ok {
		t.Fatalf("child not rebuilt: %v", tt.Texts())
	}
	if n := log.count("init"); n != 1 {
		t.Fatalf("InitState ran %d times over 6 builds: %s", n, log)
	}
	// Listen: changes from another goroutine run fn on the UI goroutine.
	done := make(chan struct{})
	go func() { src.Notify(); close(done) }()
	<-done
	tt.Settle()
	if _, ok := tt.Find("child f heard 1"); !ok || src.Len() != 1 {
		t.Fatalf("Listen: %v, %d subscribers (want 1)", tt.Texts(), src.Len())
	}

	tt.TapText("remove")
	tt.Settle()
	got := log.String()
	if !strings.HasPrefix(got, "init destroy") || !strings.HasSuffix(got, "close:file release:func") {
		t.Fatalf("lifecycle order: %s", got)
	}
	if log.count("destroy") != 1 || strings.Contains(got, "too-early") {
		t.Fatalf("OnDestroy: %s", got)
	}
	if src.Len() != 0 {
		t.Fatalf("Listen not cancelled: %d subscribers left", src.Len())
	}
	// Notifying after destroy doesn't reach the state.
	src.Notify()
	tt.Settle()
	tt.TapText("rebuild")
	tt.Settle()
	if log.count("init") != 1 || log.count("destroy") != 1 {
		t.Fatalf("hooks ran again after removal: %s", log)
	}
}

type ownState struct {
	w.StateBase
	log *lifeLog
}

type ownWidget struct{ s **ownState }

func (o ownWidget) CreateState() w.State          { s := &ownState{log: &lifeLog{}}; *o.s = s; return s }
func (s *ownState) Build(w.BuildContext) w.Widget { return w.SizedBox{} }

func TestOwnKindsAndLateOwn(t *testing.T) {
	var s *ownState
	show := true
	tt := tester.New(w.Builder{Builder: func(w.BuildContext) w.Widget {
		if show {
			return ownWidget{&s}
		}
		return w.SizedBox{}
	}}, 100, 100)
	tt.Settle()
	ctx, cancel := context.WithCancel(context.Background())
	tick := time.NewTicker(time.Hour)
	s.Own(cancel)
	s.Own(tick)
	s.Own(func() error { s.log.add("err-func"); return errors.New("ignored") })
	s.Own(nil)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Own of an int should panic")
			}
		}()
		s.Own(42)
	}()
	tt.Root.Reassemble() // a rebuild of everything doesn't destroy it
	tt.Settle()
	if s.Destroyed() || ctx.Err() != nil {
		t.Fatal("released before destroy")
	}
	show = false
	tt.Root.Reassemble()
	tt.Settle()
	if !s.Destroyed() || ctx.Err() == nil || s.log.count("err-func") != 1 {
		t.Fatalf("not released: destroyed %v ctx %v log %s", s.Destroyed(), ctx.Err(), s.log)
	}
	// Owning after destroy releases at once.
	s.Own(func() { s.log.add("late") })
	if s.log.count("late") != 1 {
		t.Fatal("late Own not released")
	}
}
