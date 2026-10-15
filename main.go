package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	logger := log.New(os.Stdout, "[turtle] ", log.LstdFlags|log.Lmsgprefix)

	app := NewApp(logger)

	err := wails.Run(&options.App{
		Title:     "Turtle · 显示器输入源切换",
		Width:     960,
		Height:    680,
		MinWidth:  720,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
		},
	})
	if err != nil {
		logger.Fatal(err)
	}
}
