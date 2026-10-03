// Package menu describes menus as data: the same items drive the in-app
// menu bar and context menus (package material) and the native macOS menu
// bar (ui.App.SetNativeMenu). Items can point at commands (package
// commands), which supply their label, shortcut, enabled state and action.
package menu

import "github.com/minelifes/nectar_ui/ui/commands"

// Role marks a standard item the OS provides on macOS (Quit, Hide, ...);
// such items ignore Action. Elsewhere they're ordinary items.
type Role uint8

const (
	RoleNone Role = iota
	RoleAbout
	RolePreferences
	RoleServices
	RoleHide
	RoleHideOthers
	RoleShowAll
	RoleQuit
	RoleClose
	RoleMinimize
	RoleZoom
	RoleFullScreen
	RoleBringAllToFront
)

// Item is one entry of a menu.
type Item struct {
	Label string
	// Command is a command ID: Label, Shortcut, Disabled and Action are
	// taken from it where not set here (see Resolve).
	Command string
	// Shortcut is the key hint shown on the right ("Ctrl+S").
	Shortcut string
	// Keys is the shortcut as keys (filled by Resolve from the command):
	// native menus show and handle it.
	Keys      commands.Sequence
	Action    func()
	Disabled  bool
	Checked   bool // shows a check mark
	Separator bool // a divider line instead of an item
	Submenu   []Item
	Role      Role
}

// Menu is a titled list of items (a menu bar entry).
type Menu struct {
	Title string
	Items []Item
}

// Sep is a separator item.
var Sep = Item{Separator: true}

// Resolve fills in items that name a command from the commands available
// in snap (a Host.Snapshot taken before the menu opened, so commands of
// the focused part of the UI apply): label, shortcut, enabled state, and
// an action that runs the command with the focus back where it was. Items
// whose command isn't available are disabled.
func Resolve(items []Item, snap *commands.Snapshot) []Item {
	out := make([]Item, len(items))
	for i, it := range items {
		if len(it.Submenu) > 0 {
			it.Submenu = Resolve(it.Submenu, snap)
		}
		if it.Command != "" {
			c, ok := commands.Command{}, false
			if snap != nil {
				c, ok = snap.Find(it.Command)
			}
			if it.Label == "" {
				it.Label = c.Title
				if it.Label == "" {
					it.Label = it.Command
				}
			}
			if it.Shortcut == "" && ok {
				it.Shortcut = snap.ShortcutLabel(it.Command)
			}
			if it.Keys == nil && ok {
				it.Keys = snap.Keys(it.Command)
			}
			if !ok || !c.IsEnabled() {
				it.Disabled = true
			}
			if it.Action == nil && ok {
				id := it.Command
				it.Action = func() { snap.Run(id) }
			}
		}
		out[i] = it
	}
	return out
}
