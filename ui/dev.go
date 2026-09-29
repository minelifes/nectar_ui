package ui

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minelifes/nectar_ui/ui/hotreload"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// devWindowKey saves the window size and mode across `nectar dev`
// restarts.
const devWindowKey = "nectar.window"

type devWindowState struct {
	W, H                  int
	Maximized, Fullscreen bool
}

// setupDev turns on the `nectar dev` features (package hotreload): live
// resource folders, the window restored across restarts, and the reload
// command on stdin. It does nothing in a normal run.
func (a *App) setupDev() {
	if !hotreload.Enabled() {
		return
	}
	for _, m := range a.config.devMounts {
		a.mountDevDir(m.prefix, m.dir)
	}
	win := a.window
	a.devStops = append(a.devStops, hotreload.Register(devWindowKey,
		func() ([]byte, error) {
			w, h := win.Size()
			return json.Marshal(devWindowState{w, h, win.IsMaximized(), win.IsFullscreen()})
		},
		func(data []byte) error {
			var st devWindowState
			if err := json.Unmarshal(data, &st); err != nil {
				return err
			}
			switch {
			case st.Fullscreen:
				win.SetFullscreen(true)
			case st.Maximized:
				win.Maximize()
			case st.W > 0 && st.H > 0:
				win.SetSize(st.W, st.H)
			}
			return nil
		}))
	if os.Getenv(hotreload.EnvControl) == "stdin" {
		go a.readDevCommands()
	}
}

// mountDevDir mounts dir under prefix and reloads what shows its files
// whenever they change.
func (a *App) mountDevDir(prefix, dir string) {
	if _, err := os.Stat(dir); err != nil {
		slog.Warn("nectar-ui: dev resources not found", "dir", dir, "err", err)
		return
	}
	a.resources.Mount(prefix, os.DirFS(dir))
	stop := hotreload.Watch(dir, nil, 0, func(changed []string) {
		a.Post(func() {
			for _, rel := range changed {
				widgets.Images.Evict(widgets.AssetImage{Name: path.Join(prefix, rel)})
			}
			if a.root != nil {
				a.root.Reassemble()
			}
		})
		slog.Info("nectar-ui: resources reloaded", "dir", filepath.Clean(dir), "files", strings.Join(changed, ", "))
	})
	a.devStops = append(a.devStops, stop)
}

// readDevCommands serves `nectar dev`: "reload" (or the pipe closing)
// saves the hotreload state and quits, so the new build can take over.
func (a *App) readDevCommands() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "reload" {
			break
		}
	}
	a.Post(func() {
		if err := hotreload.Save(); err != nil {
			slog.Warn("nectar-ui: saving dev state failed", "err", err)
		}
		a.window.Close()
	})
}

func (a *App) stopDev() {
	for _, stop := range a.devStops {
		stop()
	}
	a.devStops = nil
}
