//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/atspi"
)

type pendingAX struct {
	tree   *AXTree
	action func(uint64)
}

var atspiBridge struct {
	once    sync.Once
	mu      sync.Mutex
	done    bool
	b       *atspi.Bridge
	pending map[WindowID]pendingAX // trees set while connecting
}

// SetAccessibilityTree publishes the accessible content of a window over
// AT-SPI (nil clears it). The app joins the accessibility bus in the
// background on first use; without one (no session bus, accessibility
// off) this does nothing. action runs, on a D-Bus goroutine, when
// assistive technology activates a node.
func SetAccessibilityTree(w PlatformWindow, tree *AXTree, action func(uint64)) {
	if w == nil {
		return
	}
	atspiBridge.once.Do(func() {
		atspiBridge.pending = map[WindowID]pendingAX{}
		go connectATSPI()
	})
	atspiBridge.mu.Lock()
	if !atspiBridge.done {
		atspiBridge.pending[w.ID()] = pendingAX{tree, action}
		atspiBridge.mu.Unlock()
		return
	}
	b := atspiBridge.b
	atspiBridge.mu.Unlock()
	if b != nil {
		b.SetWindow(uint64(w.ID()), tree, action)
	}
}

func connectATSPI() {
	var b *atspi.Bridge
	if session, ok := sessionBus(); ok {
		if addr, err := atspi.BusAddress(session); err == nil {
			b, _ = atspi.Connect(addr, filepath.Base(os.Args[0]))
		}
	}
	atspiBridge.mu.Lock()
	atspiBridge.b, atspiBridge.done = b, true
	pending := atspiBridge.pending
	atspiBridge.pending = nil
	atspiBridge.mu.Unlock()
	if b == nil {
		return
	}
	for id, p := range pending {
		if p.tree != nil {
			b.SetWindow(uint64(id), p.tree, p.action)
		}
	}
}

// sessionBus finds the session bus without ever autolaunching one. ok is
// false when there's neither a session bus nor $AT_SPI_BUS_ADDRESS.
func sessionBus() (string, bool) {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a, true
	}
	for _, dir := range []string{os.Getenv("XDG_RUNTIME_DIR"), "/run/user/" + strconv.Itoa(os.Getuid())} {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "bus")); err == nil {
			return "unix:path=" + filepath.Join(dir, "bus"), true
		}
	}
	return "", os.Getenv("AT_SPI_BUS_ADDRESS") != ""
}
