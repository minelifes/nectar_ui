package tests

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// counts how often each named part of a test tree was built.
var mvBuilds = map[string]int{}

type counterVM struct {
	mvvm.ViewModel
	Count    *mvvm.Property[int]
	Name     *mvvm.Property[string]
	disposed bool
}

func newCounterVM() *counterVM {
	return &counterVM{Count: mvvm.NewProperty(0), Name: mvvm.NewProperty("a")}
}

func (vm *counterVM) Dispose() { vm.disposed = true; vm.ViewModel.Dispose() }

// counterPage reads the view model and binds two parts of it.
type counterPage struct{}

func (counterPage) Build(ctx w.BuildContext) w.Widget {
	mvBuilds["page"]++
	vm := mvvm.Use[*counterVM](ctx)
	return w.Column{Children: []w.Widget{
		mvvm.Bind(vm.Count, func(_ w.BuildContext, n int) w.Widget {
			mvBuilds["count"]++
			return w.Text{Text: fmt.Sprintf("count %d", n)}
		}),
		mvvm.Bind(vm.Name, func(_ w.BuildContext, s string) w.Widget {
			mvBuilds["name"]++
			return w.Text{Text: "name " + s}
		}),
		static{name: "static"},
	}}
}

// static counts its builds and never changes.
type static struct{ name string }

func (s static) Build(w.BuildContext) w.Widget {
	mvBuilds[s.name]++
	return w.Text{Text: s.name}
}

func mountCounter(t *testing.T) (*tester.Tester, *counterVM) {
	t.Helper()
	clear(mvBuilds)
	var vm *counterVM
	tt := tester.New(mvvm.Provide[*counterVM]{
		Create: func() *counterVM { vm = newCounterVM(); return vm },
		Child:  counterPage{},
	}, 300, 200)
	return tt, vm
}

func TestBindRebuildsOnlyTheBoundPart(t *testing.T) {
	tt, vm := mountCounter(t)
	if got := tt.Texts(); !slices.Equal(got, []string{"count 0", "name a", "static"}) {
		t.Fatalf("texts = %v", got)
	}
	vm.Count.Set(5)
	tt.Pump()
	if got := tt.Texts(); got[0] != "count 5" {
		t.Fatalf("texts = %v", got)
	}
	want := map[string]int{"page": 1, "count": 2, "name": 1, "static": 1}
	if !mapsEqual(mvBuilds, want) {
		t.Fatalf("builds = %v, want %v", mvBuilds, want)
	}

	// Setting the same value doesn't notify.
	vm.Count.Set(5)
	tt.Pump()
	if mvBuilds["count"] != 2 {
		t.Fatalf("unchanged value rebuilt: %v", mvBuilds)
	}
}

func TestBindCoalescesChangesFromOtherGoroutines(t *testing.T) {
	tt, vm := mountCounter(t)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm.Count.Update(func(n int) int { return n + 1 })
		}()
	}
	wg.Wait()
	tt.Pump()
	if got := tt.Texts()[0]; got != "count 100" {
		t.Fatalf("got %q", got)
	}
	if mvBuilds["count"] != 2 || mvBuilds["page"] != 1 {
		t.Fatalf("100 changes should cause one rebuild of the binding only: %v", mvBuilds)
	}
}

func TestProvideDisposesViewModelAndSubscriptions(t *testing.T) {
	tt, vm := mountCounter(t)
	if !vm.Count.HasListeners() {
		t.Fatal("binding did not subscribe")
	}
	tt.Root.SetApp(w.Text{Text: "gone"}, geom.Transparent)
	tt.Pump()
	if !vm.disposed {
		t.Fatal("view model not disposed when Provide left the tree")
	}
	if vm.Count.HasListeners() || vm.Name.HasListeners() {
		t.Fatal("bindings kept their subscriptions after unmount")
	}
}

// switcher watches a or b depending on useA.
type switcher struct {
	useA *mvvm.Property[bool]
	a, b *mvvm.Property[int]
}

func (s switcher) Build(ctx w.BuildContext) w.Widget {
	mvBuilds["switcher"]++
	p := s.b
	if mvvm.Watch(ctx, s.useA) {
		p = s.a
	}
	return w.Text{Text: fmt.Sprint(mvvm.Watch(ctx, p))}
}

func TestWatchDropsSubscriptionsABuildNoLongerMakes(t *testing.T) {
	clear(mvBuilds)
	sw := switcher{useA: mvvm.NewProperty(true), a: mvvm.NewProperty(1), b: mvvm.NewProperty(2)}
	tt := tester.New(sw, 100, 100)
	if !sw.a.HasListeners() || sw.b.HasListeners() {
		t.Fatal("should watch a only")
	}
	sw.useA.Set(false)
	tt.Pump()
	if sw.a.HasListeners() || !sw.b.HasListeners() {
		t.Fatal("after switching, should watch b only")
	}
	n := mvBuilds["switcher"]
	sw.a.Set(10)
	tt.Pump()
	if mvBuilds["switcher"] != n {
		t.Fatal("a change of an unwatched property rebuilt")
	}
	sw.b.Set(20)
	tt.Pump()
	if got := tt.Texts(); got[0] != "20" {
		t.Fatalf("texts = %v", got)
	}
}

type cartVM struct {
	mvvm.ViewModel
	Items []string
	Note  string
}

func TestSelectRebuildsOnlyWhenTheSelectionChanges(t *testing.T) {
	clear(mvBuilds)
	vm := &cartVM{}
	tt := tester.New(mvvm.Select(vm, func(vm *cartVM) int { return len(vm.Items) },
		func(_ w.BuildContext, n int) w.Widget {
			mvBuilds["badge"]++
			return w.Text{Text: fmt.Sprint(n)}
		}), 100, 100)
	vm.Note = "changed"
	vm.Notify()
	tt.Pump()
	if mvBuilds["badge"] != 1 {
		t.Fatal("rebuilt although the selected value didn't change")
	}
	vm.Items = append(vm.Items, "x")
	vm.Notify()
	tt.Pump()
	if mvBuilds["badge"] != 2 || tt.Texts()[0] != "1" {
		t.Fatalf("builds %v texts %v", mvBuilds, tt.Texts())
	}
}

func TestObserverAndList(t *testing.T) {
	clear(mvBuilds)
	list := mvvm.NewList("a", "b")
	tt := tester.New(mvvm.Observer{Sources: []mvvm.Listenable{list}, Builder: func(w.BuildContext) w.Widget {
		var kids []w.Widget
		for _, s := range list.Get() {
			kids = append(kids, w.KeyedSubtree{ID: s, Child: static{name: s}})
		}
		return w.Column{Children: kids}
	}}, 100, 100)

	snapshot := list.Get()
	list.Append("c")
	list.RemoveAt(0)
	tt.Pump()
	if got := tt.Texts(); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("texts = %v", got)
	}
	if !slices.Equal(snapshot, []string{"a", "b"}) {
		t.Fatalf("list changed an earlier snapshot: %v", snapshot)
	}
	// b kept its element and wasn't rebuilt: its widget didn't change.
	if mvBuilds["b"] != 1 || mvBuilds["c"] != 1 {
		t.Fatalf("builds = %v", mvBuilds)
	}
}

func TestComputedNotifiesOnlyOnChange(t *testing.T) {
	price, qty := mvvm.NewProperty(2), mvvm.NewProperty(3)
	total := mvvm.NewComputed(func() int { return price.Get() * qty.Get() }, price, qty)
	even := mvvm.NewComputed(func() bool { return total.Get()%2 == 0 }, total)
	defer total.Dispose()
	defer even.Dispose()
	notified := 0
	even.Subscribe(func() { notified++ })
	price.Set(4) // 12: still even
	if total.Get() != 12 || notified != 0 {
		t.Fatalf("total %d notified %d", total.Get(), notified)
	}
	price.Set(5) // 15: odd
	if !(total.Get() == 15 && !even.Get() && notified == 1) {
		t.Fatalf("total %d even %v notified %d", total.Get(), even.Get(), notified)
	}
}

// --- render tree: a rebuild touches only what changed ----------------------

// rebuilder rebuilds on demand with a fresh closure but the same children.
type rebuilder struct{ bump *func() }

func (rebuilder) CreateState() w.State { return &rebuilderState{} }

type rebuilderState struct {
	w.StateBase
	n int
}

func (s *rebuilderState) Build(w.BuildContext) w.Widget {
	*w.WidgetOf[rebuilder](s).bump = func() { s.SetState(func() { s.n++ }) }
	return w.RepaintBoundary{Child: w.Column{Children: []w.Widget{
		w.Row{Children: []w.Widget{static{name: "left"}, static{name: "right"}}},
		// A new closure every build: only the detector itself is updated.
		w.GestureDetector{OnTap: func() { _ = s.n }, Child: static{name: "tap"}},
	}}}
}

func TestParentRebuildSkipsUnchangedChildren(t *testing.T) {
	clear(mvBuilds)
	var bump func()
	tt := tester.New(rebuilder{bump: &bump}, 200, 100)
	b := boundaryOf(tt)
	_, missesBefore := b.Stats()
	for range 3 {
		bump()
		tt.Pump()
	}
	want := map[string]int{"left": 1, "right": 1, "tap": 1}
	if !mapsEqual(mvBuilds, want) {
		t.Fatalf("builds = %v, want %v (equal children must be skipped)", mvBuilds, want)
	}
	if _, misses := b.Stats(); misses != missesBefore {
		t.Fatalf("repaint boundary repainted %d times for rebuilds that change nothing visible", misses-missesBefore)
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Watch works inside a LayoutBuilder, whose builder runs during layout.
func TestWatchInsideLayoutBuilder(t *testing.T) {
	p := mvvm.NewProperty("a")
	tt := tester.New(w.LayoutBuilder{Builder: func(ctx w.BuildContext, _ geom.Constraints) w.Widget {
		return w.Text{Text: mvvm.Watch(ctx, p)}
	}}, 100, 100)
	p.Set("b")
	tt.Pump()
	if got := tt.Texts(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("texts = %v", got)
	}
}
