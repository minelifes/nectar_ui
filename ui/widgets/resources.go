package widgets

import (
	"io/fs"

	"github.com/minelifes/nectar_ui/ui/resources"
)

// ResourcesOf returns the resources visible at ctx: the nearest Resources
// widget's, else the app's (ui.Config.WithResources), else the global Set.
// Each layer sees the files of the ones above it.
//
//	data, err := widgets.ResourcesOf(ctx).ReadFile("charts/bar.json")
func ResourcesOf(ctx BuildContext) *resources.Set {
	if set, ok := Of[*resources.Set](ctx); ok && set != nil {
		return set
	}
	if o := ctx.Owner(); o != nil && o.Resources != nil {
		return o.Resources
	}
	return resources.Global()
}

// Resources mounts FS under Prefix for its subtree only, on top of the
// resources already visible there. Nest them for several mounts. The mount
// lives as long as the widget: it's added when the widget is first built
// and removed when it leaves the tree.
//
//	widgets.Resources{Prefix: "charts", FS: chartFiles, Child: ChartsPage{}}
//
// AssetImage and ResourcesOf below it see these files. (Images already
// decoded stay cached if you swap FS for different content under the same
// names; evict them with Images.Evict.)
type Resources struct {
	Prefix string
	FS     fs.FS
	Child  Widget
}

func (Resources) CreateState() State { return &resourcesState{} }

type resourcesState struct {
	StateBase
	set     *resources.Set
	unmount func()
}

func (s *resourcesState) InitState() {
	s.set = resources.NewSet(ResourcesOf(s.Context()))
	s.mount()
}

func (s *resourcesState) mount() {
	if s.unmount != nil {
		s.unmount()
		s.unmount = nil
	}
	if w := WidgetOf[Resources](s); w.FS != nil {
		s.unmount = s.set.Mount(w.Prefix, w.FS)
	}
}

// DidUpdateWidget remounts (fs.FS values can't always be compared, and a
// remount is cheap).
func (s *resourcesState) DidUpdateWidget(Widget) { s.mount() }

func (s *resourcesState) Dispose() {
	if s.unmount != nil {
		s.unmount()
		s.unmount = nil
	}
}

func (s *resourcesState) Build(BuildContext) Widget {
	return Provider[*resources.Set]{Value: s.set, Child: WidgetOf[Resources](s).Child}
}
