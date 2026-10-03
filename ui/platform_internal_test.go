package ui

import (
	"testing"

	"github.com/gogpu/gogpu"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/menu"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

func TestNativeMenuItems(t *testing.T) {
	a := &App{buildOwner: widgets.NewBuildOwner()}
	host := commands.NewHost()
	saved := false
	host.Registry.Register(commands.Command{ID: "file.save", Title: "Save", Run: func() { saved = true }})
	items := a.nativeItems(menu.Resolve([]menu.Item{
		{Command: "file.save"},
		{Command: "nope"},
		menu.Sep,
		{Label: "Wrap", Checked: true, Action: func() {}},
		{Label: "Recent", Submenu: []menu.Item{{Label: "a.txt", Action: func() {}}}},
		{Label: "Quit", Role: menu.RoleQuit},
	}, host.Snapshot()))
	if len(items) != 6 {
		t.Fatalf("%d items", len(items))
	}
	if items[0].Title != "Save" || items[0].Disabled || items[0].Action == nil {
		t.Fatalf("command item: %+v", items[0])
	}
	if !items[1].Disabled || items[1].Title != "nope" {
		t.Fatalf("missing command: %+v", items[1])
	}
	if !items[2].Separator || items[3].Title != "✓ Wrap" || items[4].Submenu == nil || len(items[4].Submenu.Items) != 1 {
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
