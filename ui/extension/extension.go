// Package extension lets an app be extended: by Go packages compiled in
// (the fast, fully trusted way), and by plugins loaded at run time (see
// package extension/wasm for sandboxed WebAssembly plugins).
//
// The pieces:
//
//   - A Manifest describes an extension: its ID, version, when to activate
//     it, what it contributes declaratively and what it may do.
//   - Point[T] is a typed extension point the app declares ("file icons",
//     "status bar items", "language servers"); extensions contribute values
//     to it, the app reads them.
//   - Contribution handlers turn a manifest's "contributes" sections into
//     app objects (commands, menus, settings) as soon as the extension is
//     registered, before its code runs; CommandContributions is one.
//   - Extensions are activated lazily: when an event in their Activation
//     list fires ("onStartup", "onCommand:git.commit", "onLanguage:go"),
//     or explicitly. Everything an extension registers through its Context
//     is undone when it's deactivated.
//
// Nothing here knows about IDEs: any app can take plugins with it.
package extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/minelifes/nectar_ui/ui/mvvm"
)

// Manifest describes an extension (an extension.json file, or a literal
// for compiled-in ones).
type Manifest struct {
	ID          string `json:"id"` // "publisher.name"
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	// Activation lists the events that activate the extension: "*" or
	// "onStartup" (at Start), "onCommand:<id>", or any event the app fires
	// ("onLanguage:go", "onView:git"). Empty = only when asked.
	Activation []string `json:"activation,omitempty"`
	// Contributes holds declarative contributions by section ("commands",
	// "menus", ...), handled by the app's contribution handlers.
	Contributes map[string]json.RawMessage `json:"contributes,omitempty"`
	// Main is the extension's code for loaders (a .wasm file).
	Main string `json:"main,omitempty"`
	// Permissions are what the extension may do ("commands",
	// "notifications", "storage", ...); loaders enforce them.
	Permissions []string `json:"permissions,omitempty"`
	// Requires lists extension IDs that must be activated first.
	Requires []string `json:"requires,omitempty"`
}

// Allowed reports whether the manifest asks for permission p.
func (m Manifest) Allowed(p string) bool { return slices.Contains(m.Permissions, p) }

// Extension is an extension's code.
type Extension interface {
	// Activate starts the extension. Register commands, contributions and
	// listeners through ctx: they're removed again on deactivation.
	Activate(ctx *Context) error
}

// Deactivator is implemented by extensions that clean up on deactivation
// (beyond what Context tracks).
type Deactivator interface {
	Deactivate() error
}

// ExtensionFunc adapts a function to Extension.
type ExtensionFunc func(ctx *Context) error

func (f ExtensionFunc) Activate(ctx *Context) error { return f(ctx) }

// State is an extension's lifecycle state.
type State uint8

const (
	Registered State = iota // known, code not running
	Active
	Failed // activation failed (see Info.Err)
)

func (s State) String() string { return [...]string{"registered", "active", "failed"}[s] }

// Info describes a registered extension.
type Info struct {
	Manifest Manifest
	State    State
	Err      error
}

type entry struct {
	m        Manifest
	factory  func() (Extension, error)
	ext      Extension
	ctx      *Context
	state    State
	err      error
	contribs []func() // undo the declarative contributions
}

// ContributionHandler applies one "contributes" section of a manifest and
// returns a function that removes it.
type ContributionHandler func(ext Manifest, section json.RawMessage) (remove func(), err error)

// Host owns the extensions of an app. It's a Listenable: it notifies when
// extensions are added, activated or removed.
type Host struct {
	mvvm.Notifier
	mu       sync.Mutex
	exts     map[string]*entry
	order    []string
	handlers map[string]ContributionHandler
	started  bool
	// OnError, if set, receives activation errors (they're also kept in
	// Info.Err).
	OnError func(id string, err error)
	// Services are values the app shares with extensions by name
	// (Context.Service).
	Services map[string]any
}

// NewHost makes an empty host.
func NewHost() *Host {
	return &Host{exts: map[string]*entry{}, handlers: map[string]ContributionHandler{}, Services: map[string]any{}}
}

// HandleContributions installs the handler for a "contributes" section.
// Install handlers before registering extensions.
func (h *Host) HandleContributions(section string, fn ContributionHandler) {
	h.mu.Lock()
	h.handlers[section] = fn
	h.mu.Unlock()
}

// Register adds a compiled-in extension. factory creates its code when it's
// first activated.
func (h *Host) Register(m Manifest, factory func() Extension) error {
	return h.RegisterLoader(m, func() (Extension, error) { return factory(), nil })
}

// RegisterLoader adds an extension whose code is loaded on activation
// (from a file, a network, a sandbox); see LoadDir.
func (h *Host) RegisterLoader(m Manifest, load func() (Extension, error)) error {
	if m.ID == "" {
		return errors.New("extension: manifest without an id")
	}
	h.mu.Lock()
	if _, dup := h.exts[m.ID]; dup {
		h.mu.Unlock()
		return fmt.Errorf("extension: %s is already registered", m.ID)
	}
	e := &entry{m: m, factory: load}
	h.exts[m.ID] = e
	h.order = append(h.order, m.ID)
	handlers := make(map[string]ContributionHandler, len(h.handlers))
	for k, v := range h.handlers {
		handlers[k] = v
	}
	started := h.started
	h.mu.Unlock()
	// Declarative contributions apply right away (sorted for determinism).
	sections := make([]string, 0, len(m.Contributes))
	for s := range m.Contributes {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	var errs []error
	for _, s := range sections {
		fn, ok := handlers[s]
		if !ok {
			continue // unknown sections are other apps' business
		}
		undo, err := fn(m, m.Contributes[s])
		if err != nil {
			errs = append(errs, fmt.Errorf("extension %s: contributes.%s: %w", m.ID, s, err))
			continue
		}
		if undo != nil {
			h.mu.Lock()
			e.contribs = append(e.contribs, undo)
			h.mu.Unlock()
		}
	}
	h.Notify()
	if started && activatesOn(m, "onStartup") {
		if err := h.Activate(m.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Unregister deactivates and removes an extension, undoing its
// contributions.
func (h *Host) Unregister(id string) error {
	err := h.Deactivate(id)
	h.mu.Lock()
	e := h.exts[id]
	delete(h.exts, id)
	h.order = slices.DeleteFunc(h.order, func(x string) bool { return x == id })
	h.mu.Unlock()
	if e != nil {
		for i := len(e.contribs) - 1; i >= 0; i-- {
			e.contribs[i]()
		}
	}
	h.Notify()
	return err
}

func activatesOn(m Manifest, event string) bool {
	for _, a := range m.Activation {
		if a == "*" && (event == "onStartup" || strings.HasPrefix(event, "on")) {
			return true
		}
		if a == event {
			return true
		}
	}
	return false
}

// Start activates the extensions that activate "onStartup" (or "*"), and
// later-registered ones as they come.
func (h *Host) Start() error {
	h.mu.Lock()
	h.started = true
	h.mu.Unlock()
	return h.Fire("onStartup")
}

// Fire activates every registered extension whose Activation includes
// event. Already active ones are left alone.
func (h *Host) Fire(event string) error {
	h.mu.Lock()
	var ids []string
	for _, id := range h.order {
		e := h.exts[id]
		if e.state == Registered && activatesOn(e.m, event) {
			ids = append(ids, id)
		}
	}
	h.mu.Unlock()
	var errs []error
	for _, id := range ids {
		if err := h.Activate(id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Activate runs extension id's code (and first that of the extensions it
// Requires). Activating an active extension does nothing; a failed one is
// retried.
func (h *Host) Activate(id string) error { return h.activate(id, nil) }

func (h *Host) activate(id string, stack []string) error {
	if slices.Contains(stack, id) {
		return fmt.Errorf("extension: dependency cycle: %s", strings.Join(append(stack, id), " → "))
	}
	h.mu.Lock()
	e, ok := h.exts[id]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("extension: %s is not registered", id)
	}
	if e.state == Active {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()
	for _, dep := range e.m.Requires {
		if err := h.activate(dep, append(stack, id)); err != nil {
			return h.fail(e, fmt.Errorf("extension %s: requires %s: %w", id, dep, err))
		}
	}
	ext, err := e.factory()
	if err != nil {
		return h.fail(e, fmt.Errorf("extension %s: load: %w", id, err))
	}
	ctx := &Context{host: h, Manifest: e.m}
	if err := activateSafely(ext, ctx); err != nil {
		ctx.dispose()
		return h.fail(e, fmt.Errorf("extension %s: activate: %w", id, err))
	}
	h.mu.Lock()
	e.ext, e.ctx, e.state, e.err = ext, ctx, Active, nil
	h.mu.Unlock()
	h.Notify()
	return nil
}

// activateSafely turns a panic in an extension into an error.
func activateSafely(ext Extension, ctx *Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return ext.Activate(ctx)
}

func (h *Host) fail(e *entry, err error) error {
	h.mu.Lock()
	e.state, e.err = Failed, err
	onErr := h.OnError
	h.mu.Unlock()
	if onErr != nil {
		onErr(e.m.ID, err)
	}
	h.Notify()
	return err
}

// Deactivate stops an active extension: Deactivate is called and
// everything it registered through its Context is removed. Its declarative
// contributions stay (it can be activated again).
func (h *Host) Deactivate(id string) error {
	h.mu.Lock()
	e, ok := h.exts[id]
	if !ok || e.state != Active {
		h.mu.Unlock()
		return nil
	}
	ext, ctx := e.ext, e.ctx
	e.ext, e.ctx, e.state = nil, nil, Registered
	h.mu.Unlock()
	var err error
	if d, ok := ext.(Deactivator); ok {
		err = d.Deactivate()
	}
	ctx.dispose()
	h.Notify()
	return err
}

// Stop deactivates every extension (in reverse registration order).
func (h *Host) Stop() error {
	h.mu.Lock()
	ids := slices.Clone(h.order)
	h.mu.Unlock()
	var errs []error
	for i := len(ids) - 1; i >= 0; i-- {
		if err := h.Deactivate(ids[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Extensions describes the registered extensions, in registration order.
func (h *Host) Extensions() []Info {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Info, 0, len(h.order))
	for _, id := range h.order {
		e := h.exts[id]
		out = append(out, Info{Manifest: e.m, State: e.state, Err: e.err})
	}
	return out
}

// Get describes one extension.
func (h *Host) Get(id string) (Info, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.exts[id]
	if !ok {
		return Info{}, false
	}
	return Info{Manifest: e.m, State: e.state, Err: e.err}, true
}

// ManifestFile is the name LoadDir looks for in each extension folder.
const ManifestFile = "extension.json"

// Loader turns a manifest found by LoadDir into the extension's code; the
// folder holds the extension's files (Main among them).
type Loader func(m Manifest, folder fs.FS) (Extension, error)

// LoadDir registers every extension in a folder of extensions: each
// subfolder with an extension.json. The code is loaded with load when the
// extension activates. Folders that fail to register are reported together
// in the error; the others are registered.
func (h *Host) LoadDir(fsys fs.FS, load Loader) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		dir := d.Name()
		data, err := fs.ReadFile(fsys, path.Join(dir, ManifestFile))
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			errs = append(errs, fmt.Errorf("extension %s: %s: %w", dir, ManifestFile, err))
			continue
		}
		sub, err := fs.Sub(fsys, dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := h.RegisterLoader(m, func() (Extension, error) { return load(m, sub) }); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ---------------------------------------------------------------------------
// Context

// Context is what an active extension gets: its manifest, the host's
// services, and registration helpers whose effects are undone on
// deactivation.
type Context struct {
	Manifest Manifest
	host     *Host
	mu       sync.Mutex
	undo     []func()
}

// Host returns the extension host.
func (c *Context) Host() *Host { return c.host }

// Track registers a cleanup function, run (newest first) when the
// extension is deactivated. Use it for anything not registered through the
// Context's helpers (goroutines, files, subscriptions).
func (c *Context) Track(cleanup func()) {
	if cleanup == nil {
		return
	}
	c.mu.Lock()
	c.undo = append(c.undo, cleanup)
	c.mu.Unlock()
}

// Service returns the app service registered under name (Host.Services).
func (c *Context) Service(name string) (any, bool) {
	c.host.mu.Lock()
	defer c.host.mu.Unlock()
	v, ok := c.host.Services[name]
	return v, ok
}

func (c *Context) dispose() {
	c.mu.Lock()
	undo := c.undo
	c.undo = nil
	c.mu.Unlock()
	for i := len(undo) - 1; i >= 0; i-- {
		undo[i]()
	}
}

// ---------------------------------------------------------------------------
// Extension points

// Point is a typed extension point: a list of values contributed by
// extensions (or the app itself) that the app reads. It's a Listenable.
type Point[T any] struct {
	mvvm.Notifier
	ID    string
	mu    sync.Mutex
	items []*contribution[T]
}

// Contribution is one value contributed to a Point and who contributed it.
type Contribution[T any] struct {
	Extension string // "" for the app itself
	Value     T
}

type contribution[T any] struct{ c Contribution[T] }

// NewPoint declares an extension point.
func NewPoint[T any](id string) *Point[T] { return &Point[T]{ID: id} }

// Add contributes v on behalf of extension ext ("" = the app) and returns a
// function that removes it.
func (p *Point[T]) Add(ext string, v T) (remove func()) {
	it := &contribution[T]{Contribution[T]{ext, v}}
	p.mu.Lock()
	p.items = append(p.items, it)
	p.mu.Unlock()
	p.Notify()
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			p.items = slices.DeleteFunc(p.items, func(x *contribution[T]) bool { return x == it })
			p.mu.Unlock()
			p.Notify()
		})
	}
}

// Contribute adds v to p for the extension of ctx, removed automatically
// when it's deactivated.
func Contribute[T any](ctx *Context, p *Point[T], v T) {
	ctx.Track(p.Add(ctx.Manifest.ID, v))
}

// Values returns the contributed values, in contribution order.
func (p *Point[T]) Values() []T {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]T, len(p.items))
	for i, it := range p.items {
		out[i] = it.c.Value
	}
	return out
}

// Contributions returns the values with their contributors.
func (p *Point[T]) Contributions() []Contribution[T] {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Contribution[T], len(p.items))
	for i, it := range p.items {
		out[i] = it.c
	}
	return out
}

// Get returns the contributed values (so a Point is an
// mvvm.Observable[[]T] and widgets can Bind to it).
func (p *Point[T]) Get() []T { return p.Values() }
