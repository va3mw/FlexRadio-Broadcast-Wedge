package main

import (
	"embed"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/config"
	"github.com/va3mw/FlexRadio-Broadcast-Wedge/internal/logx"
)

//go:embed all:frontend/dist
var assets embed.FS

const version = "2.0.0"

func main() {
	cfg := config.Load()
	l := logx.New(filepath.Join(config.Dir(), "logs"))
	l.Log(logx.Info, "app", "Broadcast Wedge %s starting; settings in %s", version, config.Dir())

	app := NewApp(l, cfg)
	err := wails.Run(&options.App{
		Title:     "Broadcast Wedge " + version,
		Width:     380,
		Height:    640,
		MinWidth:  340,
		MinHeight: 420,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 236, G: 236, B: 236, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "va3mw-broadcastwedge-2",
		},
		Windows: &windows.Options{},
	})
	if err != nil {
		l.Log(logx.Error, "app", "wails: %v", err)
	}
}
