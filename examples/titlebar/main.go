// Custom title bar: the app draws into the strip with the window buttons.
//
//	CGO_ENABLED=0 go run ./examples/titlebar
//
// macOS keeps the traffic lights; Windows and Linux (X11) get drawn
// minimize / maximize / close buttons. Drag the empty parts of the bar to
// move the window; the buttons in it stay clickable.
package main

import (
	"fmt"
	"log"

	"github.com/minelifes/nectar_ui/ui"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func main() {
	cfg := ui.DefaultConfig().WithTitle("Aether").WithSize(1000, 640).WithCustomTitleBar(true)
	if err := ui.NewApp(cfg).Run(home{}); err != nil {
		log.Fatal(err)
	}
}

type home struct{}

func (home) CreateState() w.State { return &homeState{} }

type homeState struct {
	w.StateBase
	dark   bool
	clicks int
}

func (s *homeState) Build(w.BuildContext) w.Widget {
	return m.App{Dark: s.dark, Home: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		win := w.WatchWindow(ctx)
		info := win.TitleBar()
		return w.Column{Cross: w.CrossStretch, Children: []w.Widget{
			m.TitleBar{
				Title:   "Aether",
				Leading: []w.Widget{w.Icon{Icon: icons.Terminal}},
				Center:  w.Text{Text: "Welcome & Quick Start"},
				Actions: []w.Widget{
					m.IconButton{Icon: icons.Tune, OnPressed: func() { s.SetState(func() { s.clicks++ }) }},
					m.IconButton{Icon: icons.DarkMode, OnPressed: func() { s.SetState(func() { s.dark = !s.dark }) }},
				},
			},
			w.Expanded{Child: w.Center{Child: w.Column{ShrinkMain: true, Spacing: 8, Cross: w.CrossCenter, Children: []w.Widget{
				w.Text{Text: fmt.Sprintf("title bar: custom=%v system buttons=%v leading=%v", info.Custom, info.SystemButtons, info.Leading)},
				w.Text{Text: fmt.Sprintf("maximized=%v fullscreen=%v, settings pressed %d×", win.IsMaximized(), win.IsFullscreen(), s.clicks)},
				m.FilledButton{Label: "Toggle fullscreen", OnPressed: func() { win.SetFullscreen(!win.IsFullscreen()) }},
			}}}},
		}}
	}}}
}
