// Package settings keeps an app's preferences in a JSON file in the user's
// config folder (~/.config/<app>/settings.json on Linux, ~/Library/
// Application Support/<app>/ on macOS, %AppData%\<app>\ on Windows):
// typed values, saved on every change, observable from widgets.
//
//	store, _ := settings.Open("myeditor")
//	fontSize := settings.Define(store, "editor.fontSize", 14)
//	fontSize.Set(16)                    // written to disk
//	mvvm.Bind(fontSize, func(ctx w.BuildContext, n int) w.Widget { ... })
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"

	"github.com/minelifes/nectar_ui/ui/mvvm"
)

// Store is a set of named settings backed by one JSON file. It's safe for
// concurrent use and notifies subscribers (Listenable) on every change.
type Store struct {
	mvvm.Notifier
	path   string
	mu     sync.Mutex
	values map[string]json.RawMessage
	// OnError receives save errors (default: ignored; the value stays in
	// memory).
	OnError func(error)
}

// Dir returns the folder Open uses for app: the OS's user config folder
// joined with app.
func Dir(app string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, app), nil
}

// Open loads the settings of app from its config folder (an empty store
// if there's no file yet).
func Open(app string) (*Store, error) {
	dir, err := Dir(app)
	if err != nil {
		return nil, err
	}
	return OpenFile(filepath.Join(dir, "settings.json"))
}

// OpenFile loads settings from path (an empty store if it doesn't exist).
func OpenFile(path string) (*Store, error) {
	s := &Store{path: path, values: map[string]json.RawMessage{}}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// Memory returns a store that is never written to disk (tests, defaults).
func Memory() *Store { return &Store{values: map[string]json.RawMessage{}} }

// Path returns the settings file ("" for a memory store).
func (s *Store) Path() string { return s.path }

// Reload rereads the file (after another process changed it) and notifies.
func (s *Store) Reload() error {
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	vals := map[string]json.RawMessage{}
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &vals); err != nil {
			return fmt.Errorf("settings: %s: %w", s.path, err)
		}
	}
	s.mu.Lock()
	s.values = vals
	s.mu.Unlock()
	s.Notify()
	return nil
}

// Keys returns the stored keys, sorted.
func (s *Store) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.values))
	for k := range s.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Has reports whether key has a stored value.
func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.values[key]
	return ok
}

// Delete removes key (it reads as its default again).
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	if _, ok := s.values[key]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.values, key)
	s.mu.Unlock()
	return s.changed()
}

func (s *Store) raw(key string) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[key]
	return v, ok
}

func (s *Store) setRaw(key string, v json.RawMessage) (bool, error) {
	s.mu.Lock()
	if old, ok := s.values[key]; ok && string(old) == string(v) {
		s.mu.Unlock()
		return false, nil
	}
	s.values[key] = v
	s.mu.Unlock()
	return true, s.changed()
}

func (s *Store) changed() error {
	err := s.Save()
	if err != nil && s.OnError != nil {
		s.OnError(err)
	}
	s.Notify()
	return err
}

// Save writes the file now (Set and Delete already do). The write is
// atomic: a crash leaves the old file or the new one, never half of it.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	s.mu.Lock()
	data, err := json.MarshalIndent(s.values, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Get returns the value stored under key, or def if there is none or it
// doesn't decode as a T.
func Get[T any](s *Store, key string, def T) T {
	raw, ok := s.raw(key)
	if !ok {
		return def
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return def
	}
	return v
}

// Set stores v under key and saves. Subscribers are notified if the value
// changed.
func Set[T any](s *Store, key string, v T) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("settings: %s: %w", key, err)
	}
	_, err = s.setRaw(key, data)
	return err
}

// Setting is one typed setting: an observable value (mvvm.Observable) that
// reads and writes its key in a Store.
type Setting[T any] struct {
	store *Store
	key   string
	def   T
}

// Define returns the setting key of s with default def.
func Define[T any](s *Store, key string, def T) *Setting[T] {
	return &Setting[T]{store: s, key: key, def: def}
}

// Key returns the setting's key.
func (st *Setting[T]) Key() string { return st.key }

// Default returns the default value.
func (st *Setting[T]) Default() T { return st.def }

// Get returns the current value (the default when unset).
func (st *Setting[T]) Get() T { return Get(st.store, st.key, st.def) }

// Set stores v (and saves). Setting the default value keeps it stored, so
// a later change of the default doesn't change the user's choice; use
// Reset to follow the default again.
func (st *Setting[T]) Set(v T) { _ = Set(st.store, st.key, v) }

// Reset removes the stored value: the setting reads as its default.
func (st *Setting[T]) Reset() { _ = st.store.Delete(st.key) }

// IsSet reports whether a value is stored.
func (st *Setting[T]) IsSet() bool { return st.store.Has(st.key) }

// Subscribe implements mvvm.Listenable: fn runs when this setting's value
// changes (not on changes to other keys).
func (st *Setting[T]) Subscribe(fn func()) func() {
	last := st.Get()
	var mu sync.Mutex
	return st.store.Subscribe(func() {
		v := st.Get()
		mu.Lock()
		same := reflect.DeepEqual(v, last)
		last = v
		mu.Unlock()
		if !same {
			fn()
		}
	})
}
