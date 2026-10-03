package wasm_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/minelifes/nectar_ui/ui/commands"
	"github.com/minelifes/nectar_ui/ui/extension"
	"github.com/minelifes/nectar_ui/ui/extension/wasm"
	"github.com/minelifes/nectar_ui/ui/settings"
)

var (
	buildOnce sync.Once
	guest     []byte
	buildErr  error
)

// guestWasm builds testdata/guest for wasip1 once.
func guestWasm(t *testing.T) []byte {
	t.Helper()
	buildOnce.Do(func() {
		out := filepath.Join(os.TempDir(), "nectar-wasm-guest-test.wasm")
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, ".")
		cmd.Dir = filepath.Join("testdata", "guest")
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr, guest = err, b
			return
		}
		guest, buildErr = os.ReadFile(out)
	})
	if buildErr != nil {
		t.Skipf("can't build the wasm test plugin: %v\n%s", buildErr, guest)
	}
	return guest
}

type recorder struct {
	mu          sync.Mutex
	logs, notes []string
}

func (r *recorder) opts(store *settings.Store) wasm.Options {
	return wasm.Options{
		Log:         func(_, line string) { r.mu.Lock(); r.logs = append(r.logs, line); r.mu.Unlock() },
		Notify:      func(_ string, _ int, msg string) { r.mu.Lock(); r.notes = append(r.notes, msg); r.mu.Unlock() },
		Storage:     store,
		CallTimeout: time.Second,
	}
}

func (r *recorder) all() (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.logs, "|"), strings.Join(r.notes, "|")
}

func manifest(perms ...string) extension.Manifest {
	cmds, _ := json.Marshal([]extension.CommandSpec{{ID: "hello.say", Title: "Say Hello", Category: "Hello"}, {ID: "loop.forever", Title: "Loop"}})
	return extension.Manifest{ID: "test.hello", Name: "Hello", Version: "1.0.0", Main: "plugin.wasm",
		Activation:  []string{"onCommand:hello.say"},
		Permissions: perms,
		Contributes: map[string]json.RawMessage{"commands": cmds},
	}
}

func TestWasmPlugin(t *testing.T) {
	code := guestWasm(t)
	var rec recorder
	store := settings.Memory()
	h := extension.NewHost()
	reg := commands.NewRegistry()
	h.UseCommands(reg)
	pings := 0
	reg.Register(commands.Command{ID: "app.ping", Title: "Ping", Run: func() { pings++ }})

	// Loaded from a folder of extensions, as an app would.
	m := manifest(wasm.PermCommands, wasm.PermNotifications, wasm.PermStorage)
	mj, _ := json.Marshal(m)
	plugins := fstest.MapFS{"hello/extension.json": {Data: mj}, "hello/plugin.wasm": {Data: code}}
	if err := h.LoadDir(plugins, wasm.Loader(rec.opts(store))); err != nil {
		t.Fatal(err)
	}
	// The command is in the palette before any plugin code runs.
	c, ok := reg.Get("hello.say")
	if !ok || c.Label() != "Hello: Say Hello" {
		t.Fatalf("declared command: %+v %v", c, ok)
	}
	if info, _ := h.Get("test.hello"); info.State != extension.Registered {
		t.Fatalf("activated too early: %v", info.State)
	}
	// Running it activates the plugin, which counts in its storage,
	// notifies and calls back into the app.
	c.Run()
	c.Run()
	if info, _ := h.Get("test.hello"); info.State != extension.Active {
		t.Fatalf("state %v %v", info.State, info.Err)
	}
	logs, notes := rec.all()
	if notes != "said hello 1 times|said hello 2 times" || pings != 2 {
		t.Fatalf("notes %q, pings %d, logs %q", notes, pings, logs)
	}
	if !strings.Contains(logs, "activating") || !strings.Contains(logs, "registered: 0 0") || !strings.Contains(logs, "storage: 0") {
		t.Fatalf("logs: %q", logs)
	}
	if settings.Get(store, "ext.test.hello.count", "") != "2" {
		t.Fatalf("storage: %v", store.Keys())
	}
	// A plugin stuck in a loop is stopped by the call timeout.
	start := time.Now()
	loop, _ := reg.Get("loop.forever")
	loop.Run()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("loop took %v", d)
	}
	if logs, _ := rec.all(); !strings.Contains(logs, "timed out") {
		t.Fatalf("no timeout logged: %q", logs)
	}
	// Deactivating removes what it registered at run time.
	if err := h.Deactivate("test.hello"); err != nil {
		t.Fatal(err)
	}
}

func TestWasmPermissions(t *testing.T) {
	code := guestWasm(t)
	var rec recorder
	store := settings.Memory()
	h := extension.NewHost()
	reg := commands.NewRegistry()
	h.UseCommands(reg)
	// No permissions at all: registration, notifications and storage are
	// refused; logging still works.
	m := manifest()
	if err := h.RegisterLoader(m, func() (extension.Extension, error) { return wasm.Load(m, code, rec.opts(store)), nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.Activate("test.hello"); err != nil {
		t.Fatal(err)
	}
	logs, _ := rec.all()
	if !strings.Contains(logs, "registered: -1 -1") || !strings.Contains(logs, "storage: -1") {
		t.Fatalf("permissions not enforced: %q", logs)
	}
	if len(store.Keys()) != 0 {
		t.Fatalf("stored without permission: %v", store.Keys())
	}
	h.Stop()
	if logs, _ := rec.all(); !strings.HasSuffix(logs, "bye") {
		t.Fatalf("deactivate not called: %q", logs)
	}
}

func TestWasmBadModules(t *testing.T) {
	h := extension.NewHost()
	m := extension.Manifest{ID: "bad"}
	h.RegisterLoader(m, func() (extension.Extension, error) { return wasm.Load(m, []byte("not wasm"), wasm.Options{}), nil })
	if err := h.Activate("bad"); err == nil || !strings.Contains(err.Error(), "compile") {
		t.Fatalf("garbage accepted: %v", err)
	}
	if info, _ := h.Get("bad"); info.State != extension.Failed {
		t.Fatalf("state %v", info.State)
	}
}
