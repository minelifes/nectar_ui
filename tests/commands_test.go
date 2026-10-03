package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/fuzzy"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/tester"
	"github.com/minelifes/nectar_ui/ui/undo"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// commandsApp: global commands, an "editor" scope around a text field, and
// a plain button outside it.
func commandsApp(host *commands.Host, log *[]string, ctrl *w.TextEditingController, ctxOut ...*w.BuildContext) w.Widget {
	run := func(s string) func() { return func() { *log = append(*log, s) } }
	host.Registry.Register(
		commands.Command{ID: "file.save", Title: "Save", Category: "File", Keys: []string{"Ctrl+S"}, Run: run("save")},
		commands.Command{ID: "file.saveAll", Title: "Save All", Category: "File", Keys: []string{"Ctrl+K Ctrl+S"}, Run: run("saveAll")},
		commands.Command{ID: "view.zen", Title: "Zen Mode", Category: "View", Keys: []string{"Ctrl+K Z"}, Run: run("zen")},
		commands.Command{ID: "edit.dup", Title: "Duplicate (global)", Keys: []string{"Ctrl+D"}, Run: run("dup-global")},
		commands.Command{ID: "never", Title: "Disabled", Keys: []string{"Ctrl+E"}, Run: run("never"), Enabled: func() bool { return false }},
	)
	return m.App{Home: m.Scaffold{Body: commands.Commands{Host: host, Child: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		for _, p := range ctxOut {
			*p = ctx
		}
		return w.Column{Children: []w.Widget{
			commands.Scope{Name: "editor", Commands: []commands.Command{
				{ID: "edit.dup", Title: "Duplicate Line", Category: "Editor", Keys: []string{"Ctrl+D"}, Run: run("dup-editor")},
			}, Child: w.SizedBox{Width: 300, Height: 40, Child: w.EditableText{Controller: ctrl, Style: text.Style{Size: 16}}}},
			m.TextButton{Label: "Outside", OnPressed: func() {}},
		}}
	}}}}}
}

func TestCommandShortcuts(t *testing.T) {
	host := commands.NewHost()
	var log []string
	ctrl := w.NewTextController("")
	tt := tester.New(commandsApp(host, &log, ctrl), 600, 400)

	// Nothing focused: global shortcuts still work.
	tt.Key(w.KeyS, w.ModControl)
	tt.Key(w.KeyD, w.ModControl)
	tt.Key(w.KeyE, w.ModControl) // disabled: nothing
	if strings.Join(log, ",") != "save,dup-global" {
		t.Fatalf("unfocused: %v", log)
	}
	// Focus in the editor scope: its Ctrl+D wins; globals still work;
	// the field keeps its own keys (Ctrl+A).
	log = nil
	tt.Tap(20, 20)
	tt.Type("abc")
	tt.Key(w.KeyD, w.ModControl)
	tt.Key(w.KeyS, w.ModControl)
	tt.Key(w.KeyA, w.ModControl)
	if strings.Join(log, ",") != "dup-editor,save" {
		t.Fatalf("in scope: %v", log)
	}
	if b, e := ctrl.Selection(); b != 0 || e != 3 {
		t.Fatalf("the field's own Ctrl+A was taken: %d..%d", b, e)
	}

	// A two-key sequence: Ctrl+K waits (and types nothing), Ctrl+S
	// finishes it; "Ctrl+K Z" doesn't type the z either.
	log = nil
	ctrl.SetText("")
	tt.Key(w.KeyK, w.ModControl)
	if p := host.Pending(); p == nil || p.String() != "Ctrl+K" {
		t.Fatalf("pending: %v", p)
	}
	tt.Key(w.KeyS, w.ModControl)
	tt.Key(w.KeyK, w.ModControl)
	tt.Key(w.KeyZ)
	tt.Type("z") // the platform's text for the Z key press
	if strings.Join(log, ",") != "saveAll,zen" || ctrl.Text() != "" || host.Pending() != nil {
		t.Fatalf("sequences: %v, text %q", log, ctrl.Text())
	}
	// A key that doesn't continue the sequence cancels it (and is eaten);
	// typing works again afterwards.
	tt.Key(w.KeyK, w.ModControl)
	tt.Key(w.KeyQ)
	tt.Type("q")
	tt.Key(w.KeyX)
	tt.Type("x")
	if ctrl.Text() != "x" || host.Pending() != nil {
		t.Fatalf("after a cancelled sequence: %q", ctrl.Text())
	}

	// The user rebinds Save: the old key stops, the new one works.
	log = nil
	host.Keymap.SetKeys("file.save", "F2")
	tt.Key(w.KeyS, w.ModControl)
	tt.Key(w.KeyF2)
	if strings.Join(log, ",") != "save" || host.ShortcutLabel("file.save") != "F2" {
		t.Fatalf("rebound: %v %q", log, host.ShortcutLabel("file.save"))
	}

	// Available: the scope's commands first, shadowing the global edit.dup.
	var ids []string
	for _, c := range host.Available() {
		ids = append(ids, c.ID+"="+c.Title)
	}
	if got := strings.Join(ids, ";"); !strings.HasPrefix(got, "edit.dup=Duplicate Line;") || strings.Contains(got, "(global)") {
		t.Fatalf("available in scope: %s", got)
	}
	// Outside the scope the global one is back.
	tt.TapText("Outside")
	if c, _ := host.Find("edit.dup"); c.Title != "Duplicate (global)" {
		t.Fatalf("outside the scope: %+v", c)
	}
}

func TestCommandPalette(t *testing.T) {
	host := commands.NewHost()
	var log []string
	ctrl := w.NewTextController("")
	var ctx w.BuildContext
	app := commandsApp(host, &log, ctrl, &ctx)
	host.Registry.Register(commands.Command{ID: "palette", Title: "Show All Commands", Keys: []string{"Ctrl+Shift+P"}, Hidden: true,
		Run: func() { m.ShowCommandPalette(ctx) }})
	tt := tester.New(app, 800, 600)

	tt.Tap(20, 20) // focus the editor
	tt.Key(w.KeyP, w.ModControl|w.ModShift)
	texts := strings.Join(tt.Texts(), "|")
	for _, want := range []string{"File: Save", "Ctrl+S", "Editor: Duplicate Line", "View: Zen Mode"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("palette lacks %q: %s", want, texts)
		}
	}
	if strings.Contains(texts, "Show All Commands") || strings.Contains(texts, "Disabled") || strings.Contains(texts, "(global)") {
		t.Fatalf("palette lists hidden, disabled or shadowed commands: %s", texts)
	}
	// Type to filter, ↓ to the second match, Enter runs it with the focus
	// back in the editor.
	tt.Type("fsav")
	texts = strings.Join(tt.Texts(), "|")
	if strings.Contains(texts, "Zen Mode") {
		t.Fatalf("filter kept Zen Mode: %s", texts)
	}
	tt.Key(w.KeyDown)
	tt.Key(w.KeyEnter)
	if strings.Join(log, ",") != "saveAll" {
		t.Fatalf("picked: %v", log)
	}
	if strings.Contains(strings.Join(tt.Texts(), "|"), "File: Save") {
		t.Fatal("palette still open")
	}
	tt.Type("q")
	if ctrl.Text() != "q" {
		t.Fatalf("focus not restored to the editor: %q", ctrl.Text())
	}
	// Reopened: the last command is listed first; Escape closes.
	tt.Key(w.KeyP, w.ModControl|w.ModShift)
	tt.Settle()
	if r, ok := tt.Find("File: Save All"); !ok || r.Y > 120 {
		t.Fatalf("recent command not first: %v %v", r, ok)
	}
	tt.Key(w.KeyEscape)
	if _, ok := tt.Find("File: Save All"); ok {
		t.Fatal("Escape didn't close the palette")
	}
}

func TestQuickPickMouseAndSearch(t *testing.T) {
	var picked string
	files := []string{"main.go", "ui/widgets/scroll.go", "ui/material/theme.go", "README.md"}
	var ctx w.BuildContext
	tt := tester.New(m.App{Home: m.Scaffold{Body: w.Builder{Builder: func(c w.BuildContext) w.Widget {
		ctx = c
		return w.SizedBox{}
	}}}}, 800, 600)
	m.ShowQuickPick(ctx, m.QuickPick{Placeholder: "Go to file",
		Search: func(q string) []m.QuickPickItem {
			var out []m.QuickPickItem
			for _, mt := range fuzzy.Filter(q, files) {
				out = append(out, m.QuickPickItem{Label: files[mt.Index], Value: files[mt.Index]})
			}
			return out
		},
		OnPick: func(it m.QuickPickItem) { picked = it.Value.(string) }})
	tt.Pump()
	tt.Settle() // the panel fades in
	if _, ok := tt.Find("Go to file"); !ok {
		t.Fatal("placeholder not shown")
	}
	tt.Type("thm")
	if texts := tt.Texts(); !contains(texts, "ui/material/theme.go") || contains(texts, "README.md") {
		t.Fatalf("search results: %v", texts)
	}
	tt.TapText("ui/material/theme.go")
	if picked != "ui/material/theme.go" {
		t.Fatalf("picked %q", picked)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestFuzzyRanking(t *testing.T) {
	items := []string{"View: Toggle Zen Mode", "File: Save", "File: Save All", "Format Document", "fsav"}
	ms := fuzzy.Filter("fsav", items)
	if len(ms) < 3 || items[ms[0].Index] != "fsav" {
		t.Fatalf("exact match should win: %+v", ms)
	}
	if _, _, ok := fuzzy.Score("zx", "File: Save"); ok {
		t.Fatal("non-subsequence matched")
	}
	sc1, pos, _ := fuzzy.Score("gtf", "Go To File")
	sc2, _, _ := fuzzy.Score("gtf", "getting toffee")
	if sc1 <= sc2 || len(pos) != 3 || pos[1] != 3 || pos[2] != 6 {
		t.Fatalf("word starts should win: %d vs %d, %v", sc1, sc2, pos)
	}
	if r := fuzzy.Ranges("File: Save", []int{0, 1, 6}); len(r) != 2 || r[0] != [2]int{0, 2} {
		t.Fatalf("ranges: %v", r)
	}
	// Long items stay fast (no quadratic blowup).
	long := strings.Repeat("abcdefghij", 200) + "xyz"
	start := time.Now()
	for i := 0; i < 200; i++ {
		fuzzy.Score("axyz", long)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("slow: %v", d)
	}
}

func TestUndoHistory(t *testing.T) {
	h := undo.New(3)
	clock := time.Unix(0, 0)
	h.SetClock(func() time.Time { return clock })
	doc := ""
	typeText := func(s string) {
		before := doc
		h.Do(undo.Action{Label: "Typing", MergeKey: "type", Do: func() { doc = before + s }, Undo: func() { doc = before }})
	}
	typeText("a")
	clock = clock.Add(100 * time.Millisecond)
	typeText("b") // merges with "a"
	clock = clock.Add(2 * time.Second)
	typeText("c") // too late to merge
	if doc != "abc" || h.UndoLabel() != "Typing" {
		t.Fatal(doc)
	}
	h.Undo()
	if doc != "ab" {
		t.Fatalf("undo 1: %q", doc)
	}
	h.Undo()
	if doc != "" || h.CanUndo() {
		t.Fatalf("undo merged step: %q", doc)
	}
	h.Redo()
	h.Redo()
	if doc != "abc" || h.CanRedo() {
		t.Fatalf("redo: %q", doc)
	}
	// Groups are one step; a new action drops the redo stack.
	h.MarkClean()
	h.Group("Replace", func() {
		typeText("1")
		clock = clock.Add(5 * time.Second)
		typeText("2")
	})
	if !h.Dirty() || h.UndoLabel() != "Replace" {
		t.Fatal("group")
	}
	h.Undo()
	if doc != "abc" || h.Dirty() {
		t.Fatalf("group undo: %q dirty=%v", doc, h.Dirty())
	}
	// After a save, typing starts a new step instead of merging into the
	// saved one.
	clock = clock.Add(10 * time.Millisecond)
	typeText("d")
	h.Undo()
	if doc != "abc" {
		t.Fatalf("merged across a save: %q", doc)
	}
	// The limit drops the oldest steps.
	for _, s := range []string{"1", "2", "3", "4", "5"} {
		clock = clock.Add(5 * time.Second)
		typeText(s)
	}
	n := 0
	for h.Undo() {
		n++
	}
	if n != 3 {
		t.Fatalf("limit: undid %d steps", n)
	}
	_ = geom.Offset{}
}
