package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/menu"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// ctxApp mounts body in a material app and returns a context inside it.
func ctxApp(body w.Widget, wd, ht int) (*tester.Tester, *w.BuildContext) {
	var ctx w.BuildContext
	tt := tester.New(m.App{Home: m.Scaffold{Body: w.Builder{Builder: func(c w.BuildContext) w.Widget {
		ctx = c
		return body
	}}}}, wd, ht)
	return tt, &ctx
}

func TestMenuKeyboardAndSubmenus(t *testing.T) {
	var log []string
	tap := func(s string) func() { return func() { log = append(log, s) } }
	tt, ctx := ctxApp(w.SizedBox{}, 800, 600)
	m.ShowMenu(*ctx, geom.Rect{X: 20, Y: 20, W: 10, H: 10}, []m.MenuItem{
		{Label: "Cut", OnTap: tap("cut")},
		{Label: "Gone", Disabled: true},
		{Divider: true},
		{Label: "More", Submenu: []m.MenuItem{
			{Label: "Alpha", OnTap: tap("alpha")},
			{Label: "Beta", Checked: true, OnTap: tap("beta")},
		}},
	}, m.MenuOptions{})
	tt.Settle()
	// ↓ skips the disabled item and the divider.
	tt.Key(w.KeyDown) // Cut
	tt.Key(w.KeyDown) // More
	tt.Key(w.KeyRight)
	tt.Settle()
	if _, ok := tt.Find("Alpha"); !ok {
		t.Fatalf("submenu not opened: %v", tt.Texts())
	}
	tt.Key(w.KeyDown) // Beta (first item highlighted on open)
	tt.Key(w.KeyEnter)
	if strings.Join(log, ",") != "beta" {
		t.Fatalf("picked %v", log)
	}
	if _, ok := tt.Find("Cut"); ok {
		t.Fatal("whole menu should close after a pick")
	}

	// ← closes just the submenu; Escape the rest.
	m.ShowMenu(*ctx, geom.Rect{X: 20, Y: 20}, []m.MenuItem{{Label: "Top", Submenu: []m.MenuItem{{Label: "Inner", OnTap: tap("inner")}}}}, m.MenuOptions{})
	tt.Settle()
	tt.Key(w.KeyDown)
	tt.Key(w.KeyRight)
	tt.Settle()
	tt.Key(w.KeyLeft)
	if _, ok := tt.Find("Inner"); ok {
		t.Fatal("← should close the submenu")
	}
	if _, ok := tt.Find("Top"); !ok {
		t.Fatal("← closed the parent too")
	}
	tt.Key(w.KeyEscape)
	if _, ok := tt.Find("Top"); ok {
		t.Fatal("Escape didn't close the menu")
	}
	// Hovering an item with a submenu opens it; clicking a nested item runs it.
	m.ShowMenu(*ctx, geom.Rect{X: 20, Y: 20}, []m.MenuItem{{Label: "Top", Submenu: []m.MenuItem{{Label: "Inner", OnTap: tap("inner")}}}}, m.MenuOptions{})
	tt.Settle()
	r, _ := tt.Find("Top")
	tt.Hover(r.X+2, r.Y+2)
	tt.Settle()
	tt.TapText("Inner")
	if strings.Join(log, ",") != "beta,inner" {
		t.Fatalf("hover submenu: %v", log)
	}
}

func TestContextMenuAndCommands(t *testing.T) {
	host := commands.NewHost()
	var log []string
	host.Registry.Register(
		commands.Command{ID: "edit.copy", Title: "Copy", Keys: []string{"Ctrl+C"}, Run: func() { log = append(log, "copy") }},
		commands.Command{ID: "edit.paste", Title: "Paste", Run: func() {}, Enabled: func() bool { return false }},
	)
	region := m.ContextMenuRegion{Items: func() []menu.Item {
		return []menu.Item{{Command: "edit.copy"}, {Command: "edit.paste"}, {Command: "missing"}, menu.Sep, {Label: "Custom", Action: func() { log = append(log, "custom") }}}
	}, Child: w.SizedBox{Width: 200, Height: 100, Child: w.Text{Text: "right-click me"}}}
	tt := tester.New(m.App{Home: commands.Commands{Host: host, Child: w.Align{Alignment: geom.TopLeft, Child: region}}}, 600, 400)
	// A left click does nothing; a right click opens the menu at the pointer.
	tt.Tap(50, 50)
	if _, ok := tt.Find("Copy"); ok {
		t.Fatal("left click opened the context menu")
	}
	e := render.PointerEvent{Kind: render.PointerDown, ID: 1, Position: geom.Pt(50, 50), Button: render.ButtonSecondary}
	tt.Dispatch(e)
	e.Kind = render.PointerUp
	tt.Dispatch(e)
	tt.Settle()
	r, ok := tt.Find("Copy")
	if !ok || r.Y < 50 || r.X < 50 {
		t.Fatalf("menu at %v %v", r, ok)
	}
	if !contains(tt.Texts(), "Ctrl+C") {
		t.Fatalf("shortcut hint missing: %v", tt.Texts())
	}
	tt.TapText("Paste") // disabled
	tt.TapText("missing")
	if len(log) != 0 {
		t.Fatalf("disabled items ran: %v", log)
	}
	tt.TapText("Copy")
	if strings.Join(log, ",") != "copy" {
		t.Fatalf("command item: %v", log)
	}
}

func TestMenuBar(t *testing.T) {
	var log []string
	menus := []menu.Menu{
		{Title: "File", Items: []menu.Item{{Label: "New", Action: func() { log = append(log, "new") }}, {Label: "Recent", Submenu: []menu.Item{{Label: "a.txt", Action: func() { log = append(log, "a") }}}}}},
		{Title: "Edit", Items: []menu.Item{{Label: "Undo", Shortcut: "Ctrl+Z", Action: func() { log = append(log, "undo") }}}},
		{Title: "View", Items: []menu.Item{{Label: "Zen", Checked: true, Action: func() {}}}},
	}
	tt := tester.New(m.App{Home: w.Column{Cross: w.CrossStretch, Children: []w.Widget{m.MenuBar{Menus: menus}, w.Expanded{Child: w.SizedBox{}}}}}, 800, 600)
	tt.TapText("File")
	tt.Settle()
	if _, ok := tt.Find("New"); !ok {
		t.Fatal("File menu didn't open")
	}
	// Hovering another title switches menus.
	r, _ := tt.Find("Edit")
	tt.Hover(r.X+2, r.Y+2)
	tt.Settle()
	if _, ok := tt.Find("New"); ok {
		t.Fatal("File menu still open")
	}
	if _, ok := tt.Find("Undo"); !ok {
		t.Fatal("hover didn't open Edit")
	}
	// → moves to View with the keyboard, ← back to Edit.
	tt.Key(w.KeyRight)
	tt.Settle()
	if _, ok := tt.Find("Zen"); !ok {
		t.Fatal("→ didn't move to View")
	}
	tt.Key(w.KeyLeft)
	tt.Settle()
	tt.Key(w.KeyEnter) // Undo is highlighted (opened by keyboard)
	if strings.Join(log, ",") != "undo" {
		t.Fatalf("picked %v", log)
	}
	// Tapping the open title closes its menu.
	tt.TapText("File")
	tt.Settle()
	tt.TapText("File")
	tt.Settle()
	if _, ok := tt.Find("New"); ok {
		t.Fatal("second tap on the title should close the menu")
	}
}

func TestToasts(t *testing.T) {
	tt, ctx := ctxApp(w.SizedBox{}, 800, 600)
	var closed []string
	a := m.ShowToast(*ctx, m.Toast{Title: "Saved", Message: "main.go", Kind: m.ToastSuccess, OnClose: func() { closed = append(closed, "a") }})
	b := m.ShowToast(*ctx, m.Toast{Message: "Indexing…", Busy: true, OnClose: func() { closed = append(closed, "b") }})
	tt.Pump()
	ra, _ := tt.Find("Saved")
	rb, _ := tt.Find("Indexing…")
	if ra.Y >= rb.Y || ra.X < 400 {
		t.Fatalf("stack order/position: %v %v", ra, rb)
	}
	// Hovering keeps a toast; leaving lets it time out (busy ones stay).
	tt.Hover(ra.X+2, ra.Y+2)
	tt.Advance(6 * time.Second)
	if _, ok := tt.Find("Saved"); !ok {
		t.Fatal("hovered toast timed out")
	}
	tt.Hover(10, 10)
	tt.Advance(6 * time.Second)
	if _, ok := tt.Find("Saved"); ok || strings.Join(closed, ",") != "a" {
		t.Fatalf("toast didn't time out: %v", closed)
	}
	if _, ok := tt.Find("Indexing…"); !ok {
		t.Fatal("busy toast must stay")
	}
	b.Update(m.Toast{Message: "Indexed 120 files", Kind: m.ToastInfo, Duration: -1, OnClose: func() { closed = append(closed, "b") }})
	tt.Advance(10 * time.Second)
	if _, ok := tt.Find("Indexed 120 files"); !ok {
		t.Fatal("updated toast with Duration < 0 must stay")
	}
	b.Close()
	tt.Pump()
	if _, ok := tt.Find("Indexed 120 files"); ok || strings.Join(closed, ",") != "a,b" {
		t.Fatalf("close: %v", closed)
	}
	_ = a
	if tt.Build.HasActiveTickers() {
		t.Fatal("the toaster keeps ticking with no toasts")
	}
	// An action runs and closes the toast.
	ran := false
	m.ShowToast(*ctx, m.Toast{Message: "Build failed", Kind: m.ToastError, Actions: []m.ToastAction{{Label: "Show", OnPressed: func() { ran = true }}}})
	tt.Pump()
	tt.TapText("Show")
	if _, ok := tt.Find("Build failed"); !ran || ok {
		t.Fatalf("action: ran=%v still shown=%v", ran, ok)
	}
}

func TestHoverCard(t *testing.T) {
	more := false
	tt := tester.New(m.App{Home: w.Align{Alignment: geom.TopLeft, Child: m.HoverCard{Title: "fmt.Println", Text: "Println formats using the default formats.",
		Actions: []m.ToastAction{{Label: "Docs", OnPressed: func() { more = true }}},
		Child:   w.SizedBox{Width: 100, Height: 30, Child: w.Text{Text: "Println"}}}}}, 600, 400)
	tt.Hover(10, 10)
	tt.Advance(200 * time.Millisecond)
	if _, ok := tt.Find("fmt.Println"); ok {
		t.Fatal("card shown before the wait")
	}
	tt.Advance(400 * time.Millisecond)
	r, ok := tt.Find("fmt.Println")
	if !ok || r.Y < 30 {
		t.Fatalf("card not shown below: %v %v", r, ok)
	}
	// Moving onto the card keeps it; its action works.
	tt.Hover(r.X+2, r.Y+2)
	tt.Advance(time.Second)
	if _, ok := tt.Find("fmt.Println"); !ok {
		t.Fatal("card hid while hovered")
	}
	tt.TapText("Docs")
	if !more {
		t.Fatal("card action didn't run")
	}
	// Leaving both hides it after a short grace.
	tt.Hover(500, 350)
	tt.Advance(100 * time.Millisecond)
	if _, ok := tt.Find("fmt.Println"); !ok {
		t.Fatal("card hid without grace time")
	}
	tt.Advance(300 * time.Millisecond)
	if _, ok := tt.Find("fmt.Println"); ok {
		t.Fatal("card stayed after leaving")
	}
	// A quick pass over the child shows nothing.
	tt.Hover(10, 10)
	tt.Advance(100 * time.Millisecond)
	tt.Hover(500, 350)
	tt.Advance(time.Second)
	if _, ok := tt.Find("fmt.Println"); ok || tt.Build.HasActiveTickers() {
		t.Fatal("card after a quick pass")
	}
}

func TestFileDialogs(t *testing.T) {
	var got []string
	var saved string
	tt, ctx := ctxApp(w.SizedBox{}, 400, 300)
	tt.Window.OpenResult = []string{"/tmp/a.go", "/tmp/b.go"}
	tt.Window.SaveResult = "/tmp/out.txt"
	w.ShowOpenDialog(*ctx, w.FileDialogOptions{Title: "Open", Multiple: true, Filters: []w.FileFilter{{Name: "Go", Extensions: []string{"go"}}}},
		func(p []string, err error) { got = p })
	w.ShowSaveDialog(*ctx, w.FileDialogOptions{DefaultFilename: "out.txt"}, func(p string, err error) { saved = p })
	tt.Pump()
	if strings.Join(got, ",") != "/tmp/a.go,/tmp/b.go" || saved != "/tmp/out.txt" {
		t.Fatalf("dialogs: %v %q", got, saved)
	}
	if len(tt.Window.Dialogs) != 2 || tt.Window.Dialogs[0].Title != "Open" || !tt.Window.Dialogs[0].Multiple {
		t.Fatalf("options: %+v", tt.Window.Dialogs)
	}
}
