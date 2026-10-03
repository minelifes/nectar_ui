package ui

import (
	"runtime"
	"strings"

	"github.com/minelifes/nectar_ui/internal/gogpu"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/menu"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// --- File dialogs ------------------------------------------------------

func toGPUDialog(o widgets.FileDialogOptions) gogpu.FileDialogOptions {
	g := gogpu.FileDialogOptions{Title: o.Title, Directory: o.Directory, Multiple: o.Multiple,
		InitialDirectory: o.InitialDirectory, DefaultFilename: o.DefaultFilename}
	for _, f := range o.Filters {
		g.Filters = append(g.Filters, gogpu.FileTypeFilter{Name: f.Name, Extensions: f.Extensions})
	}
	return g
}

// OpenFileDialog implements widgets.FileDialogs: the dialog runs on the
// main thread (it's modal), the result is posted to the UI goroutine.
func (n *nativeWindow) OpenFileDialog(opt widgets.FileDialogOptions, done func([]string, error)) {
	n.do(func(a *gogpu.App) {
		paths, err := a.ShowOpenFileDialog(toGPUDialog(opt))
		n.post(func() { done(paths, err) })
	})
}

// SaveFileDialog implements widgets.FileDialogs.
func (n *nativeWindow) SaveFileDialog(opt widgets.FileDialogOptions, done func(string, error)) {
	n.do(func(a *gogpu.App) {
		path, err := a.ShowSaveFileDialog(toGPUDialog(opt))
		n.post(func() { done(path, err) })
	})
}

// --- Native menu bar ---------------------------------------------------------

// HasNativeMenuBar reports whether SetNativeMenu shows a system menu bar:
// the macOS menu bar, or the Win32 menu bar of the window on Windows.
// Elsewhere, put a material.MenuBar in the window.
func HasNativeMenuBar() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

// SetNativeMenu sets the system menu bar (macOS, Windows; a no-op
// elsewhere) from the same menus a material.MenuBar takes. Items naming
// commands are resolved against host (may be nil) when the menu is set:
// labels, enabled states and shortcuts (on macOS single-chord shortcuts
// become the items' key equivalents; Windows shows them). Picking an item
// runs its command where the focus is then. Call SetNativeMenu again to
// refresh labels and enabled states. Actions run on the UI goroutine. Call
// it before or after Run.
func (a *App) SetNativeMenu(menus []menu.Menu, host *commands.Host) {
	a.nativeMenus, a.nativeHost = menus, host
	if a.window != nil {
		a.window.do(func(g *gogpu.App) { a.applyNativeMenu(g) })
	}
}

func (a *App) applyNativeMenu(g *gogpu.App) {
	if a.nativeMenus == nil {
		return
	}
	var snap *commands.Snapshot
	if a.nativeHost != nil {
		snap = a.nativeHost.Snapshot()
	}
	root := gogpu.NewMenu()
	for _, m := range a.nativeMenus {
		root.AddItem(gogpu.MenuItem{Title: m.Title, Submenu: &gogpu.Menu{Title: m.Title, Items: a.nativeItems(menu.Resolve(m.Items, snap))}})
	}
	g.SetMenu(root)
}

func (a *App) nativeItems(items []menu.Item) []gogpu.MenuItem {
	out := make([]gogpu.MenuItem, 0, len(items))
	for _, it := range items {
		if it.Separator {
			out = append(out, gogpu.NewSeparator())
			continue
		}
		gi := gogpu.MenuItem{Title: it.Label, Disabled: it.Disabled, Role: gogpu.MenuRole(it.Role),
			Checked: it.Checked, ShortcutText: it.Shortcut}
		if len(it.Keys) == 1 {
			gi.KeyEquivalent, gi.KeyModifiers = nativeKey(it.Keys[0])
		}
		act := it.Action
		if id, host := it.Command, a.nativeHost; id != "" && host != nil {
			// Run the command available where the focus is when picked.
			act = func() { host.Execute(id) }
		}
		if act != nil {
			// Native menus call back on the main thread; run on the UI's.
			gi.Action = func() { a.Post(act) }
		}
		if len(it.Submenu) > 0 {
			gi.Submenu = &gogpu.Menu{Title: it.Label, Items: a.nativeItems(it.Submenu)}
		}
		out = append(out, gi)
	}
	return out
}

// nativeKey converts a chord to a menu key equivalent.
func nativeKey(c commands.Chord) (string, gogpu.MenuModifiers) {
	var m gogpu.MenuModifiers
	if c.Mods&widgets.ModShift != 0 {
		m |= gogpu.MenuShift
	}
	if c.Mods&widgets.ModControl != 0 {
		m |= gogpu.MenuControl
	}
	if c.Mods&widgets.ModAlt != 0 {
		m |= gogpu.MenuAlt
	}
	if c.Mods&widgets.ModSuper != 0 {
		m |= gogpu.MenuCommand
	}
	spec := c.Spec() // "Ctrl+Shift+P": the key is the last part
	if i := strings.LastIndexByte(spec, '+'); i >= 0 && i < len(spec)-1 {
		spec = spec[i+1:]
	}
	return spec, m
}
