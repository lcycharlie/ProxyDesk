package main

import (
	"context"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"proxydesk/internal/modernui"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AppName() string {
	return "ProxyDesk"
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "ProxyDesk",
		Width:  1180,
		Height: 760,
		AssetServer: &assetserver.Options{
			Assets: modernui.FS(),
		},
		BackgroundColour: &options.RGBA{R: 244, G: 248, B: 247, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("ProxyDesk modern UI startup failed:", err.Error())
	}
}
