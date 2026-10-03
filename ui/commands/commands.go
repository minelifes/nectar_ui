// Package commands is an application command system: named actions with
// titles and categories, keyboard shortcuts (including multi-key
// sequences like Ctrl+K Ctrl+S) the user can rebind and save, and scopes
// that make commands available only while focus is inside part of the UI.
// Menus, command palettes and toolbars list and run the same commands.
//
//	host := commands.NewHost()
//	host.Registry.Register(commands.Command{
//	    ID: "file.save", Title: "Save", Category: "File", Keys: []string{"Mod+S"},
//	    Run: vm.Save,
//	})
//	app := commands.Commands{Host: host, Child: page}
//
//	// a part of the UI with its own commands (active while focused):
//	commands.Scope{Name: "editor", Commands: []commands.Command{{
//	    ID: "editor.duplicateLine", Title: "Duplicate Line", Keys: []string{"Mod+D"}, Run: ed.Duplicate,
//	}}, Child: editorView}
//
// "Mod" is ⌘ on macOS and Ctrl elsewhere. Keys found by bubbling: the
// focused widget gets a key first (a text field keeps Ctrl+C), then the
// scopes around it, innermost first, then the global commands.
package commands

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/minelifes/nectar_ui/ui/mvvm"
)

// Command is a named action.
type Command struct {
	// ID identifies the command ("file.save"); keymaps refer to it.
	ID string
	// Title is shown in menus and the command palette ("Save").
	Title string
	// Category groups commands ("File"); the palette shows "File: Save".
	Category string
	// Keys are the default shortcuts, in ParseKeys form ("Mod+S",
	// "Ctrl+K Ctrl+S"). A Keymap can override them.
	Keys []string
	// Run performs the command.
	Run func()
	// Enabled reports whether the command can run now; nil = always.
	Enabled func() bool
	// Hidden commands work from keys and menus but aren't listed by the
	// command palette.
	Hidden bool
}

// Label is the title with its category ("File: Save").
func (c Command) Label() string {
	if c.Category == "" {
		return c.Title
	}
	return c.Category + ": " + c.Title
}

// IsEnabled reports whether the command can run now.
func (c Command) IsEnabled() bool { return c.Run != nil && (c.Enabled == nil || c.Enabled()) }

// ---------------------------------------------------------------------------
// Registry

// Registry holds the global commands. It's safe for concurrent use and
// notifies subscribers when commands are added or removed.
type Registry struct {
	mvvm.Notifier
	mu    sync.Mutex
	items []*regEntry
}

type regEntry struct{ cmd Command }

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds commands and returns a function that removes them. A
// command with the ID of an existing one shadows it until removed.
func (r *Registry) Register(cmds ...Command) (unregister func()) {
	entries := make([]*regEntry, len(cmds))
	r.mu.Lock()
	for i, c := range cmds {
		entries[i] = &regEntry{c}
		r.items = append(r.items, entries[i])
	}
	r.mu.Unlock()
	r.Notify()
	return func() {
		r.mu.Lock()
		r.items = slices.DeleteFunc(r.items, func(e *regEntry) bool { return slices.Contains(entries, e) })
		r.mu.Unlock()
		r.Notify()
	}
}

// All returns the registered commands (the newest of each ID), sorted by
// label.
func (r *Registry) All() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	var out []Command
	for i := len(r.items) - 1; i >= 0; i-- {
		c := r.items[i].cmd
		if !seen[c.ID] {
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	sortCommands(out)
	return out
}

// Get returns the command with the given ID.
func (r *Registry) Get(id string) (Command, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.items) - 1; i >= 0; i-- {
		if r.items[i].cmd.ID == id {
			return r.items[i].cmd, true
		}
	}
	return Command{}, false
}

func sortCommands(cs []Command) {
	sort.SliceStable(cs, func(i, j int) bool { return strings.ToLower(cs[i].Label()) < strings.ToLower(cs[j].Label()) })
}

// ---------------------------------------------------------------------------
// Keymap

// Keymap holds the user's shortcut changes on top of the commands' default
// Keys. Save it with json.Marshal (only the changes are written) and load
// it with json.Unmarshal. It's safe for concurrent use and notifies
// subscribers on change.
type Keymap struct {
	mvvm.Notifier
	mu        sync.Mutex
	overrides map[string][]Sequence // nil slice in the map = unbound
}

// NewKeymap returns a keymap without changes.
func NewKeymap() *Keymap { return &Keymap{} }

// SetKeys replaces the shortcuts of command id; no keys unbinds it.
func (k *Keymap) SetKeys(id string, keys ...string) error {
	seqs := make([]Sequence, 0, len(keys))
	for _, s := range keys {
		seq, err := ParseKeys(s)
		if err != nil {
			return err
		}
		seqs = append(seqs, seq)
	}
	k.mu.Lock()
	if k.overrides == nil {
		k.overrides = map[string][]Sequence{}
	}
	k.overrides[id] = seqs
	k.mu.Unlock()
	k.Notify()
	return nil
}

// Reset drops the change to command id: it gets its default keys again.
func (k *Keymap) Reset(id string) {
	k.mu.Lock()
	delete(k.overrides, id)
	k.mu.Unlock()
	k.Notify()
}

// Overridden reports whether id's keys were changed.
func (k *Keymap) Overridden(id string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, ok := k.overrides[id]
	return ok
}

// KeysFor returns the effective shortcuts of c: the keymap's, or c's
// defaults. Default keys that don't parse are skipped.
func (k *Keymap) KeysFor(c Command) []Sequence {
	if k != nil {
		k.mu.Lock()
		o, ok := k.overrides[c.ID]
		k.mu.Unlock()
		if ok {
			return o
		}
	}
	out := make([]Sequence, 0, len(c.Keys))
	for _, s := range c.Keys {
		if seq, err := ParseKeys(s); err == nil {
			out = append(out, seq)
		}
	}
	return out
}

// MarshalJSON writes the changes: {"file.save": ["Ctrl+Alt+S"]}.
func (k *Keymap) MarshalJSON() ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	m := make(map[string][]string, len(k.overrides))
	for id, seqs := range k.overrides {
		specs := make([]string, len(seqs))
		for i, s := range seqs {
			specs[i] = s.Spec()
		}
		m[id] = specs
	}
	return json.Marshal(m)
}

// UnmarshalJSON replaces the changes with the saved ones.
func (k *Keymap) UnmarshalJSON(data []byte) error {
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	o := make(map[string][]Sequence, len(m))
	for id, specs := range m {
		seqs := make([]Sequence, 0, len(specs))
		for _, sp := range specs {
			seq, err := ParseKeys(sp)
			if err != nil {
				return fmt.Errorf("commands: keymap entry %s: %w", id, err)
			}
			seqs = append(seqs, seq)
		}
		o[id] = seqs
	}
	k.mu.Lock()
	k.overrides = o
	k.mu.Unlock()
	k.Notify()
	return nil
}

// Conflict is a key sequence bound to more than one command.
type Conflict struct {
	Keys     Sequence
	Commands []string // IDs
}

// Conflicts finds sequences that run more than one of cmds, or where one
// command's keys are a prefix of another's (the longer one can't be typed).
func (k *Keymap) Conflicts(cmds []Command) []Conflict {
	type bound struct {
		seq Sequence
		id  string
	}
	var all []bound
	for _, c := range cmds {
		for _, s := range k.KeysFor(c) {
			all = append(all, bound{s, c.ID})
		}
	}
	var out []Conflict
	used := map[int]bool{}
	for i := range all {
		if used[i] {
			continue
		}
		cf := Conflict{Keys: all[i].seq, Commands: []string{all[i].id}}
		for j := i + 1; j < len(all); j++ {
			a, b := all[i].seq, all[j].seq
			if all[j].id != all[i].id && (a.Equal(b) || a.hasPrefix(b) || b.hasPrefix(a)) {
				cf.Commands = append(cf.Commands, all[j].id)
				used[j] = true
			}
		}
		if len(cf.Commands) > 1 {
			out = append(out, cf)
		}
	}
	return out
}
