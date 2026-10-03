package extension

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/minelifes/nectar_ui/ui/commands"
)

// CommandSpec is one entry of a manifest's "commands" section:
//
//	"contributes": {"commands": [
//	    {"id": "git.commit", "title": "Commit", "category": "Git", "keys": ["Ctrl+K Ctrl+C"]}
//	]}
type CommandSpec struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Category string   `json:"category,omitempty"`
	Keys     []string `json:"keys,omitempty"`
	Hidden   bool     `json:"hidden,omitempty"`
}

// commandBridge connects declared commands to the handlers extensions
// register when they activate.
type commandBridge struct {
	host     *Host
	reg      *commands.Registry
	mu       sync.Mutex
	handlers map[string]func()
}

const bridgeService = "extension.commands"

// UseCommands lets extensions add commands to reg: manifests' "commands"
// sections are shown in reg (the command palette, menus, keymaps) as soon
// as the extension is registered, before its code is loaded; running one
// activates the extension (event "onCommand:<id>") and calls the handler
// it registered with RegisterCommand. Call it before registering
// extensions.
func (h *Host) UseCommands(reg *commands.Registry) {
	b := &commandBridge{host: h, reg: reg, handlers: map[string]func(){}}
	h.mu.Lock()
	h.Services[bridgeService] = b
	h.mu.Unlock()
	h.HandleContributions("commands", b.contribute)
}

func (b *commandBridge) contribute(m Manifest, raw json.RawMessage) (func(), error) {
	var specs []CommandSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, err
	}
	cmds := make([]commands.Command, 0, len(specs))
	for _, sp := range specs {
		if sp.ID == "" {
			return nil, fmt.Errorf("a command without an id")
		}
		id := sp.ID
		cmds = append(cmds, commands.Command{ID: id, Title: sp.Title, Category: sp.Category, Keys: sp.Keys, Hidden: sp.Hidden,
			Run: func() { b.run(m.ID, id) }})
	}
	return b.reg.Register(cmds...), nil
}

// run activates the extension if needed and calls its handler.
func (b *commandBridge) run(ext, id string) {
	if err := b.host.Fire("onCommand:" + id); err != nil {
		return
	}
	if err := b.host.Activate(ext); err != nil {
		return
	}
	b.mu.Lock()
	fn := b.handlers[id]
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// RegisterCommand sets the code of command id for the extension of ctx
// (until it's deactivated). If the manifest didn't declare the command, it
// is added to the registry with id as its title.
func RegisterCommand(ctx *Context, id string, run func()) error {
	v, ok := ctx.Service(bridgeService)
	if !ok {
		return fmt.Errorf("extension: the app doesn't take commands (Host.UseCommands)")
	}
	b := v.(*commandBridge)
	b.mu.Lock()
	b.handlers[id] = run
	b.mu.Unlock()
	declared := false
	if raw, ok := ctx.Manifest.Contributes["commands"]; ok {
		var specs []CommandSpec
		if json.Unmarshal(raw, &specs) == nil {
			for _, sp := range specs {
				declared = declared || sp.ID == id
			}
		}
	}
	if !declared {
		ctx.Track(b.reg.Register(commands.Command{ID: id, Title: id, Run: run}))
	}
	ctx.Track(func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	})
	return nil
}

// ExecuteCommand runs a command of the app's registry by ID (from an
// extension); false if it doesn't exist or is disabled.
func ExecuteCommand(ctx *Context, id string) bool {
	v, ok := ctx.Service(bridgeService)
	if !ok {
		return false
	}
	c, ok := v.(*commandBridge).reg.Get(id)
	if !ok || !c.IsEnabled() {
		return false
	}
	c.Run()
	return true
}
