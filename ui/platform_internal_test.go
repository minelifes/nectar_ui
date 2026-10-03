package ui

import (
	"testing"

	"github.com/minelifes/nectar_ui/internal/gogpu"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/menu"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

func TestNativeMenuItems(t *testing.T) {
	a := &App{buildOwner: widgets.NewBuildOwner()}
	host := commands.NewHost()
	saved := false
	host.Registry.Register(commands.Command{ID: "file.save", Title: "Save", Keys: []string{"Ctrl+Shift+S"}, Run: func() { saved = true }})
	host.Registry.Register(commands.Command{ID: "file.saveAll", Title: "Save All", Keys: []string{"Ctrl+K S"}, Run: func() {}})
	items := a.nativeItems(menu.Resolve([]menu.Item{
		{Command: "file.save"},
		{Command: "nope"},
		{Command: "file.saveAll"},
		menu.Sep,
		{Label: "Wrap", Checked: true, Action: func() {}},
		{Label: "Recent", Submenu: []menu.Item{{Label: "a.txt", Action: func() {}}}},
		{Label: "Quit", Role: menu.RoleQuit},
	}, host.Snapshot()))
	if len(items) != 7 {
		t.Fatalf("%d items", len(items))
	}
	if items[0].Title != "Save" || items[0].Disabled || items[0].Action == nil {
		t.Fatalf("command item: %+v", items[0])
	}
	if items[0].KeyEquivalent != "S" || items[0].KeyModifiers != gogpu.MenuControl|gogpu.MenuShift || items[0].ShortcutText == "" {
		t.Fatalf("shortcut: %q %v %q", items[0].KeyEquivalent, items[0].KeyModifiers, items[0].ShortcutText)
	}
	// A two-chord sequence can't be a key equivalent; it's still shown.
	if items[2].KeyEquivalent != "" || items[2].ShortcutText == "" {
		t.Fatalf("sequence: %+v", items[2])
	}
	items = append(items[:2], items[3:]...)
	if !items[1].Disabled || items[1].Title != "nope" {
		t.Fatalf("missing command: %+v", items[1])
	}
	if !items[2].Separator || items[3].Title != "Wrap" || !items[3].Checked || items[4].Submenu == nil || len(items[4].Submenu.Items) != 1 {
		t.Fatalf("items: %+v", items)
	}
	if items[5].Role != gogpu.RoleQuit {
		t.Fatalf("role: %+v", items[5])
	}
	// Picking runs the command on the UI goroutine (posted).
	items[0].Action()
	if saved {
		t.Fatal("ran on the menu's thread")
	}
	a.buildOwner.FlushBuild()
	if !saved {
		t.Fatal("posted command didn't run")
	}
}
