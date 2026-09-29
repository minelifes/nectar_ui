// Package hotreload supports `nectar dev`, which rebuilds and restarts the
// app whenever its Go code changes. Go can't swap code into a running
// process, so a "reload" is a restart that carries state across:
//
//   - the window keeps its size (ui.App does this),
//   - values registered with Keep (or mvvm's Property.Keep / List.Keep)
//     are saved before the old process exits and restored when the new
//     one registers the same key,
//   - resources mounted with ui.Config.WithDevResources are read from disk
//     and reload live, without a restart.
//
// Outside `nectar dev` (Enabled() == false) everything here is a no-op, so
// the calls can stay in release builds.
package hotreload

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// Environment variables `nectar dev` sets for the app it runs.
const (
	// EnvDev is "1" when the app runs under `nectar dev`.
	EnvDev = "NECTAR_DEV"
	// EnvState names the file state is saved to between restarts.
	EnvState = "NECTAR_DEV_STATE"
	// EnvControl is "stdin" when `nectar dev` sends commands on stdin
	// ("reload": save state and quit).
	EnvControl = "NECTAR_DEV_CONTROL"
)

// Enabled reports whether the app runs under `nectar dev`.
func Enabled() bool { return os.Getenv(EnvDev) == "1" }

type entry struct {
	save func() ([]byte, error)
}

var reg struct {
	mu      sync.Mutex
	loaded  bool
	saved   map[string]json.RawMessage // from the previous run, not yet restored
	entries map[string]*entry
}

func loadLocked() {
	if reg.loaded {
		return
	}
	reg.loaded = true
	reg.entries = map[string]*entry{}
	path := os.Getenv(EnvState)
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	if err := json.Unmarshal(data, &reg.saved); err != nil {
		slog.Warn("hotreload: ignoring unreadable state", "file", path, "err", err)
	}
}

// Register makes a value survive restarts under key. If the previous run
// saved key, restore is called right away with that data (once per run:
// a later Register of the same key starts fresh). save is called before
// the process exits for a reload, on the UI goroutine. A later Register
// of the same key replaces this one; unregister removes it.
func Register(key string, save func() ([]byte, error), restore func([]byte) error) (unregister func()) {
	if !Enabled() {
		return func() {}
	}
	reg.mu.Lock()
	loadLocked()
	data, ok := reg.saved[key]
	delete(reg.saved, key)
	e := &entry{save: save}
	reg.entries[key] = e
	reg.mu.Unlock()
	if ok && restore != nil {
		if err := restore(data); err != nil {
			slog.Warn("hotreload: could not restore", "key", key, "err", err)
		}
	}
	return func() {
		reg.mu.Lock()
		defer reg.mu.Unlock()
		if reg.entries[key] == e {
			delete(reg.entries, key)
		}
	}
}

// Keep makes *ptr survive restarts under key: it's restored now (if the
// previous run saved it) and saved before a reload, as JSON. Call it where
// the value is created, e.g. in a State's InitState:
//
//	func (s *pageState) InitState() { hotreload.Keep("page.tab", &s.tab) }
//
// ptr must only be written on the UI goroutine (or be otherwise safe to
// read there).
func Keep[T any](key string, ptr *T) (unregister func()) {
	return Register(key,
		func() ([]byte, error) { return json.Marshal(*ptr) },
		func(data []byte) error { return json.Unmarshal(data, ptr) })
}

// Save writes every registered value to the state file. ui.App calls it
// before quitting for a reload; call it on the UI goroutine.
func Save() error {
	path := os.Getenv(EnvState)
	if !Enabled() || path == "" {
		return nil
	}
	reg.mu.Lock()
	loadLocked()
	out := make(map[string]json.RawMessage, len(reg.entries))
	entries := make(map[string]*entry, len(reg.entries))
	for k, e := range reg.entries {
		entries[k] = e
	}
	reg.mu.Unlock()
	for k, e := range entries {
		data, err := e.save()
		if err != nil {
			slog.Warn("hotreload: could not save", "key", k, "err", err)
			continue
		}
		out[k] = data
	}
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("hotreload: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("hotreload: %w", err)
	}
	return os.Rename(tmp, path)
}

// reset forgets all registrations and the loaded state (tests).
func reset() {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.loaded, reg.saved, reg.entries = false, nil, nil
}
