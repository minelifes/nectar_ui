// Package wasm runs extensions compiled to WebAssembly in a sandbox (with
// wazero, pure Go: no cgo). A plugin can't touch files, the network or the
// environment; it can only call the host functions below, and only those
// its manifest's permissions allow. Calls are time-limited and memory is
// capped, so a broken plugin can't hang or exhaust the app.
//
// # ABI
//
// Strings cross the boundary as (pointer, length) pairs of UTF-8 bytes in
// the plugin's memory. The plugin exports:
//
//	nectar_alloc(size i32) -> ptr i32   memory for strings the host sends (required)
//	activate()                          optional
//	deactivate()                        optional
//	on_command(ptr, len i32)            a command it registered was run
//
// and may import, from module "nectar":
//
//	log(ptr, len i32)                                   always allowed
//	notify(kind, ptr, len i32)                          "notifications": 0 info, 1 success, 2 warning, 3 error
//	register_command(ptr, len i32) -> i32               "commands": 0 = ok, -1 = denied
//	execute_command(ptr, len i32) -> i32                "commands": 1 = ran, 0 = not found, -1 = denied
//	storage_get(kptr, klen i32) -> i64                  "storage": (ptr << 32 | len) of the value, 0 = none
//	storage_set(kptr, klen, vptr, vlen i32) -> i32      "storage": 0 = ok, -1 = denied
//
// WASI (preview 1) is provided without any files or environment, so Go
// (GOOS=wasip1 -buildmode=c-shared), TinyGo and Rust (wasm32-wasip1)
// plugins work; their stdout and stderr go to the log.
//
// A Go plugin:
//
//	//go:wasmimport nectar register_command
//	func registerCommand(ptr unsafe.Pointer, n uint32) int32
//
//	//go:wasmexport activate
//	func activate() { s := "hello.say"; registerCommand(unsafe.Pointer(unsafe.StringData(s)), uint32(len(s))) }
package wasm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/minelifes/nectar_ui/ui/extension"
	"github.com/minelifes/nectar_ui/ui/settings"
)

// Permission names a plugin's manifest can ask for.
const (
	PermCommands      = "commands"
	PermNotifications = "notifications"
	PermStorage       = "storage"
)

// Options configure the plugins a Loader makes.
type Options struct {
	// Log receives log lines and stdout/stderr output (nil = discard).
	Log func(ext, line string)
	// Notify shows a notification (kind: 0 info, 1 success, 2 warning,
	// 3 error), e.g. with material.ShowToast. nil = log it.
	Notify func(ext string, kind int, msg string)
	// Storage keeps plugin data, under "ext.<id>.<key>" (nil = in memory).
	Storage *settings.Store
	// MemoryLimitPages caps a plugin's memory in 64 KiB pages (default
	// 1024 = 64 MiB).
	MemoryLimitPages uint32
	// CallTimeout bounds each call into a plugin (default 5s).
	CallTimeout time.Duration
}

// Loader returns an extension.Loader that runs manifests' Main .wasm file.
func Loader(opt Options) extension.Loader {
	opt = opt.withDefaults()
	return func(m extension.Manifest, folder fs.FS) (extension.Extension, error) {
		if m.Main == "" {
			return nil, errors.New("wasm: manifest has no main")
		}
		code, err := fs.ReadFile(folder, m.Main)
		if err != nil {
			return nil, err
		}
		return &plugin{m: m, code: code, opt: opt}, nil
	}
}

// Load makes an extension from wasm code directly (no folder).
func Load(m extension.Manifest, code []byte, opt Options) extension.Extension {
	return &plugin{m: m, code: code, opt: opt.withDefaults()}
}

func (opt Options) withDefaults() Options {
	if opt.MemoryLimitPages == 0 {
		opt.MemoryLimitPages = 1024
	}
	if opt.CallTimeout <= 0 {
		opt.CallTimeout = 5 * time.Second
	}
	if opt.Storage == nil {
		opt.Storage = settings.Memory()
	}
	return opt
}

// plugin is one running wasm extension.
type plugin struct {
	m    extension.Manifest
	code []byte
	opt  Options

	mu       sync.Mutex // one call into the module at a time
	rt       wazero.Runtime
	mod      api.Module
	ctx      *extension.Context
	deferred []func() // host actions to run after the current call
}

func (p *plugin) log(line string) {
	if p.opt.Log != nil {
		p.opt.Log(p.m.ID, line)
	}
}

// Activate compiles and instantiates the module and calls its activate.
func (p *plugin) Activate(ctx *extension.Context) error {
	p.ctx = ctx
	bg := context.Background()
	rt := wazero.NewRuntimeWithConfig(bg, wazero.NewRuntimeConfig().
		WithMemoryLimitPages(p.opt.MemoryLimitPages).
		WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(bg, rt); err != nil {
		rt.Close(bg)
		return err
	}
	if err := p.hostModule(rt); err != nil {
		rt.Close(bg)
		return err
	}
	compiled, err := rt.CompileModule(bg, p.code)
	if err != nil {
		rt.Close(bg)
		return fmt.Errorf("wasm: compile: %w", err)
	}
	out := &lineWriter{fn: p.log}
	cfg := wazero.NewModuleConfig().WithName(p.m.ID).WithStdout(out).WithStderr(out).
		WithStartFunctions() // reactors: _initialize is called below
	callCtx, cancel := context.WithTimeout(bg, p.opt.CallTimeout)
	defer cancel()
	mod, err := rt.InstantiateModule(callCtx, compiled, cfg)
	if err != nil {
		rt.Close(bg)
		return fmt.Errorf("wasm: instantiate: %w", err)
	}
	p.rt, p.mod = rt, mod
	if mod.ExportedFunction("nectar_alloc") == nil {
		p.close()
		return errors.New("wasm: the module doesn't export nectar_alloc")
	}
	if err := p.call("_initialize"); err != nil {
		p.close()
		return err
	}
	if err := p.call("activate"); err != nil {
		p.close()
		return err
	}
	return nil
}

// Deactivate calls the module's deactivate and frees it.
func (p *plugin) Deactivate() error {
	p.mu.Lock()
	dead := p.mod == nil // stopped by a timeout
	p.mu.Unlock()
	var err error
	if !dead {
		err = p.call("deactivate")
	}
	p.close()
	return err
}

func (p *plugin) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rt != nil {
		p.rt.Close(context.Background())
		p.rt, p.mod = nil, nil
	}
}

// call runs an exported function if the module has it (with the timeout),
// then the host actions it asked for.
func (p *plugin) call(name string, args ...uint64) error {
	p.mu.Lock()
	err := p.callLocked(name, args...)
	deferred := p.deferred
	p.deferred = nil
	p.mu.Unlock()
	for _, fn := range deferred {
		fn()
	}
	return err
}

func (p *plugin) callLocked(name string, args ...uint64) error {
	if p.mod == nil {
		return errors.New("wasm: plugin not running")
	}
	fn := p.mod.ExportedFunction(name)
	if fn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.opt.CallTimeout)
	defer cancel()
	if _, err := fn.Call(ctx, args...); err != nil {
		if ctx.Err() != nil {
			// The module was closed by the timeout: it's unusable now.
			p.rt = nil
			p.mod = nil
			return fmt.Errorf("wasm: %s: timed out after %v", name, p.opt.CallTimeout)
		}
		return fmt.Errorf("wasm: %s: %w", name, err)
	}
	return nil
}

// sendString copies s into the module's memory (via nectar_alloc).
// Called with p.mu held.
func (p *plugin) sendString(ctx context.Context, mod api.Module, s string) (uint32, uint32, error) {
	alloc := mod.ExportedFunction("nectar_alloc")
	if alloc == nil {
		return 0, 0, errors.New("no nectar_alloc")
	}
	res, err := alloc.Call(ctx, uint64(len(s)))
	if err != nil || len(res) == 0 {
		return 0, 0, fmt.Errorf("nectar_alloc: %v", err)
	}
	ptr := uint32(res[0])
	if !mod.Memory().Write(ptr, []byte(s)) {
		return 0, 0, errors.New("nectar_alloc returned memory out of range")
	}
	return ptr, uint32(len(s)), nil
}

func readString(mod api.Module, ptr, n uint32) (string, bool) {
	if n > 1<<20 {
		return "", false
	}
	b, ok := mod.Memory().Read(ptr, n)
	if !ok {
		return "", false
	}
	return string(b), true
}

// hostModule defines the "nectar" imports.
func (p *plugin) hostModule(rt wazero.Runtime) error {
	b := rt.NewHostModuleBuilder("nectar")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, mod api.Module, ptr, n uint32) {
		if s, ok := readString(mod, ptr, n); ok {
			p.log(s)
		}
	}).Export("log")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, mod api.Module, kind, ptr, n uint32) int32 {
		if !p.m.Allowed(PermNotifications) {
			return -1
		}
		s, ok := readString(mod, ptr, n)
		if !ok {
			return -1
		}
		if p.opt.Notify != nil {
			p.opt.Notify(p.m.ID, int(kind), s)
		} else {
			p.log("notify: " + s)
		}
		return 0
	}).Export("notify")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, mod api.Module, ptr, n uint32) int32 {
		if !p.m.Allowed(PermCommands) {
			return -1
		}
		id, ok := readString(mod, ptr, n)
		if !ok || id == "" {
			return -1
		}
		err := extension.RegisterCommand(p.ctx, id, func() { p.onCommand(id) })
		if err != nil {
			p.log(err.Error())
			return -1
		}
		return 0
	}).Export("register_command")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, mod api.Module, ptr, n uint32) int32 {
		if !p.m.Allowed(PermCommands) {
			return -1
		}
		id, ok := readString(mod, ptr, n)
		if !ok {
			return -1
		}
		// The command may call back into this plugin: run it after the
		// current call returns, on the same goroutine.
		p.deferred = append(p.deferred, func() { extension.ExecuteCommand(p.ctx, id) })
		return 1
	}).Export("execute_command")
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod api.Module, kptr, klen uint32) uint64 {
		if !p.m.Allowed(PermStorage) {
			return 0
		}
		k, ok := readString(mod, kptr, klen)
		if !ok {
			return 0
		}
		v, ok := storageGet(p.opt.Storage, p.key(k))
		if !ok {
			return 0
		}
		ptr, n, err := p.sendString(ctx, mod, v)
		if err != nil {
			return 0
		}
		return uint64(ptr)<<32 | uint64(n)
	}).Export("storage_get")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, mod api.Module, kptr, klen, vptr, vlen uint32) int32 {
		if !p.m.Allowed(PermStorage) {
			return -1
		}
		k, ok1 := readString(mod, kptr, klen)
		v, ok2 := readString(mod, vptr, vlen)
		if !ok1 || !ok2 {
			return -1
		}
		if settings.Set(p.opt.Storage, p.key(k), v) != nil {
			return -1
		}
		return 0
	}).Export("storage_set")
	_, err := b.Instantiate(context.Background())
	return err
}

func (p *plugin) key(k string) string { return "ext." + p.m.ID + "." + k }

func storageGet(s *settings.Store, key string) (string, bool) {
	if !s.Has(key) {
		return "", false
	}
	return settings.Get(s, key, ""), true
}

// onCommand delivers a registered command to the plugin.
func (p *plugin) onCommand(id string) {
	p.mu.Lock()
	if p.mod == nil {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.opt.CallTimeout)
	ptr, n, err := p.sendString(ctx, p.mod, id)
	cancel()
	if err == nil {
		err = p.callLocked("on_command", uint64(ptr), uint64(n))
	}
	deferred := p.deferred
	p.deferred = nil
	p.mu.Unlock()
	if err != nil {
		p.log(err.Error())
	}
	for _, fn := range deferred {
		fn()
	}
}

// lineWriter turns output into log lines.
type lineWriter struct {
	fn  func(string)
	buf strings.Builder
	mu  sync.Mutex
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range b {
		if c == '\n' {
			w.fn(w.buf.String())
			w.buf.Reset()
			continue
		}
		if w.buf.Len() < 4096 {
			w.buf.WriteByte(c)
		}
	}
	return len(b), nil
}

var _ io.Writer = (*lineWriter)(nil)
