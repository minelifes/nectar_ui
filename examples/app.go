package main

import (
	"log"
	"github.com/minelifes/nectar_ui/ui"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func main() {
	app := ui.NewApp(ui.DefaultConfig().WithTitle("Nectar UI — input").WithSize(720, 480))
	if err := app.Run(MyApp{}); err != nil {
		log.Fatal(err)
	}
}

type MyApp struct{}

type appState struct{ w.StateBase }

func (a appState) Build(ctx w.BuildContext) w.Widget {
	return w.Container{
		Child: w.MouseRegion{
			Child: w.Text{
				Text: "sd",
			},
		},
	}
}

func (MyApp) CreateState() w.State { return &appState{} }
