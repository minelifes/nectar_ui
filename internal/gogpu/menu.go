package gogpu

import "github.com/minelifes/nectar_ui/internal/gogpu/internal/platform"

// MenuRole maps to standard menu items.
type MenuRole int

const (
	RoleNone MenuRole = iota
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

// MenuItem represents a single item in a menu.
type MenuItem struct {
	Title     string
	Action    func()
	Role      MenuRole
	Disabled  bool
	Separator bool
	Submenu   *Menu

	// KeyEquivalent is the item's shortcut key, shown and handled by the
	// macOS menu bar: a character ("s", "/") or a key name ("F5", "Left",
	// "Enter", "Escape", ...), with KeyModifiers. Empty = none.
	KeyEquivalent string
	KeyModifiers  MenuModifiers
	// ShortcutText is the shortcut shown right-aligned in Windows menus
	// ("Ctrl+S"); display only (key handling stays with the app).
	ShortcutText string
	// Checked shows a check mark.
	Checked bool
}

// MenuModifiers are the modifier keys of a menu item's key equivalent.
type MenuModifiers uint8

const (
	MenuShift   MenuModifiers = MenuModifiers(platform.MenuModShift)
	MenuControl MenuModifiers = MenuModifiers(platform.MenuModControl)
	MenuAlt     MenuModifiers = MenuModifiers(platform.MenuModAlt)
	MenuCommand MenuModifiers = MenuModifiers(platform.MenuModCommand)
)

// Menu represents a top-level menu.
type Menu struct {
	Title string
	Items []MenuItem
}

// NewMenu creates an empty menu.
func NewMenu() *Menu {
	return &Menu{}
}

// NewMenuWithTitle creates an empty menu with the specified title.
func NewMenuWithTitle(title string) *Menu {
	return &Menu{Title: title}
}

// AddItem appends an item to the menu and returns the menu for chaining.
func (m *Menu) AddItem(item MenuItem) *Menu {
	m.Items = append(m.Items, item)
	return m
}

// NewSeparator returns a menu separator item.
func NewSeparator() MenuItem {
	return MenuItem{Separator: true}
}

// AddSeparator appends a separator to the menu and returns the menu for chaining.
func (m *Menu) AddSeparator() *Menu {
	return m.AddItem(MenuItem{Separator: true})
}

// SystemMenu identifies a standard macOS menu that can be extended.
type SystemMenu int

const (
	SystemMenuApplication SystemMenu = iota
	SystemMenuWindow
)

// SystemMenuHandle allows adding items to a system menu.
type SystemMenuHandle struct {
	manager platform.PlatMenuManager
	menu    platform.SystemMenu
	app     *App
}

// AddItem appends a new item to the system menu.
func (h *SystemMenuHandle) AddItem(item MenuItem) bool {
	if h.manager != nil {
		return h.manager.AddToSystemMenu(h.menu, []platform.MenuItem{{
			Title:     item.Title,
			Action:    item.Action,
			Role:      platform.MenuRole(item.Role),
			Disabled:  item.Disabled,
			Separator: item.Separator,
		}})
	}
	if h.app != nil && h.app.pendingSystemMenuItems != nil {
		h.app.pendingSystemMenuItems[SystemMenu(h.menu)] = append(
			h.app.pendingSystemMenuItems[SystemMenu(h.menu)],
			item,
		)
		return true
	}
	return false
}
