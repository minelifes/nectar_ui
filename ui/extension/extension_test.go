package extension_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/extension"
)

// statusItems is an extension point an app might declare.
var statusItems = extension.NewPoint[string]("statusBar.items")

func TestActivationAndCleanup(t *testing.T) {
	h := extension.NewHost()
	reg := commands.NewRegistry()
	h.UseCommands(reg)
	var log []string
	mk := func(id string, act []string, req ...string) {
		h.Register(extension.Manifest{ID: id, Activation: act, Requires: req}, func() extension.Extension {
			return extension.ExtensionFunc(func(ctx *extension.Context) error {
				log = append(log, "activate "+id)
				extension.Contribute(ctx, statusItems, id+" item")
				ctx.Track(func() { log = append(log, "cleanup "+id) })
				return extension.RegisterCommand(ctx, id+".run", func() { log = append(log, "run "+id) })
			})
		})
	}
	mk("base", nil)
	mk("git", []string{"onView:git"}, "base")
	mk("startup", []string{"onStartup"})
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(log, ",") != "activate startup" || len(statusItems.Values()) != 1 {
		t.Fatalf("startup: %v %v", log, statusItems.Values())
	}
	// An app event activates git, and base first (it's required).
	h.Fire("onView:git")
	if strings.Join(log, ",") != "activate startup,activate base,activate git" {
		t.Fatalf("event: %v", log)
	}
	got := statusItems.Contributions()
	if len(got) != 3 || got[2].Extension != "git" || got[2].Value != "git item" {
		t.Fatalf("point: %+v", got)
	}
	// Undeclared commands registered at activation join the registry.
	c, ok := reg.Get("git.run")
	if !ok {
		t.Fatal("runtime command missing")
	}
	c.Run()
	// Deactivation undoes contributions, commands and tracked cleanups.
	h.Deactivate("git")
	if _, ok := reg.Get("git.run"); ok || len(statusItems.Values()) != 2 || log[len(log)-1] != "cleanup git" {
		t.Fatalf("deactivate: %v %v", log, statusItems.Values())
	}
	// Extensions registered after Start with onStartup activate at once.
	mk("late", []string{"*"})
	if info, _ := h.Get("late"); info.State != extension.Active {
		t.Fatalf("late: %v", info.State)
	}
	h.Stop()
	for _, i := range h.Extensions() {
		if i.State != extension.Registered {
			t.Fatalf("%s still %v", i.Manifest.ID, i.State)
		}
	}
	if len(statusItems.Values()) != 0 {
		t.Fatalf("leftover contributions: %v", statusItems.Values())
	}
}

func TestDeclaredCommandsActivateLazily(t *testing.T) {
	h := extension.NewHost()
	reg := commands.NewRegistry()
	h.UseCommands(reg)
	activations, runs := 0, 0
	specs, _ := json.Marshal([]extension.CommandSpec{{ID: "fmt.doc", Title: "Format Document", Category: "Format", Keys: []string{"Shift+Alt+F"}}})
	m := extension.Manifest{ID: "fmt", Activation: []string{"onCommand:fmt.doc"}, Contributes: map[string]json.RawMessage{"commands": specs}}
	h.Register(m, func() extension.Extension {
		return extension.ExtensionFunc(func(ctx *extension.Context) error {
			activations++
			return extension.RegisterCommand(ctx, "fmt.doc", func() { runs++ })
		})
	})
	c, ok := reg.Get("fmt.doc")
	if !ok || c.Label() != "Format: Format Document" || len(c.Keys) != 1 || activations != 0 {
		t.Fatalf("declared: %+v", c)
	}
	c.Run()
	c.Run()
	if activations != 1 || runs != 2 {
		t.Fatalf("activations %d runs %d", activations, runs)
	}
	// Unregistering removes the declared command too.
	h.Unregister("fmt")
	if _, ok := reg.Get("fmt.doc"); ok {
		t.Fatal("declared command left after unregister")
	}
}

func TestActivationFailures(t *testing.T) {
	h := extension.NewHost()
	var reported []string
	h.OnError = func(id string, err error) { reported = append(reported, id) }
	h.Register(extension.Manifest{ID: "err"}, func() extension.Extension {
		return extension.ExtensionFunc(func(ctx *extension.Context) error {
			ctx.Track(func() { reported = append(reported, "cleaned") })
			return errors.New("nope")
		})
	})
	h.Register(extension.Manifest{ID: "panics"}, func() extension.Extension {
		return extension.ExtensionFunc(func(*extension.Context) error { panic("boom") })
	})
	h.Register(extension.Manifest{ID: "a", Requires: []string{"b"}}, func() extension.Extension {
		return extension.ExtensionFunc(func(*extension.Context) error { return nil })
	})
	h.Register(extension.Manifest{ID: "b", Requires: []string{"a"}}, func() extension.Extension {
		return extension.ExtensionFunc(func(*extension.Context) error { return nil })
	})
	if err := h.Activate("err"); err == nil {
		t.Fatal("error swallowed")
	}
	if err := h.Activate("panics"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("panic: %v", err)
	}
	if err := h.Activate("a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	if info, _ := h.Get("panics"); info.State != extension.Failed || info.Err == nil {
		t.Fatalf("state %+v", info)
	}
	if strings.Join(reported, ",") != "cleaned,err,panics,b,a" {
		t.Fatalf("reported %v", reported)
	}
	if err := h.Register(extension.Manifest{ID: "err"}, nil); err == nil {
		t.Fatal("duplicate id accepted")
	}
}

func TestLoadDir(t *testing.T) {
	h := extension.NewHost()
	fsys := fstest.MapFS{
		"one/extension.json":    {Data: []byte(`{"id": "one", "name": "One", "main": "one.bin"}`)},
		"one/one.bin":           {Data: []byte("code-1")},
		"broken/extension.json": {Data: []byte(`{`)},
		"notext/readme.md":      {Data: []byte("x")},
	}
	var loaded string
	err := h.LoadDir(fsys, func(m extension.Manifest, folder fs.FS) (extension.Extension, error) {
		b, err := fs.ReadFile(folder, m.Main)
		loaded = string(b)
		return extension.ExtensionFunc(func(*extension.Context) error { return err }), nil
	})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("broken manifest not reported: %v", err)
	}
	if exts := h.Extensions(); len(exts) != 1 || exts[0].Manifest.Name != "One" {
		t.Fatalf("extensions: %+v", exts)
	}
	if loaded != "" {
		t.Fatal("loaded before activation")
	}
	if err := h.Activate("one"); err != nil || loaded != "code-1" {
		t.Fatalf("activate: %v %q", err, loaded)
	}
}
