package main

import (
	"embed"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	logger := newLogger()

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

// newLogger 同时写 stdout（wails dev 可见）与用户目录下的日志文件
//（GUI 双击运行时没有控制台，真机排错靠它）：%AppData%\turtle\app.log。
func newLogger() *log.Logger {
	writers := []io.Writer{os.Stdout}
	if dir, err := os.UserConfigDir(); err == nil {
		logDir := filepath.Join(dir, "turtle")
		if err := os.MkdirAll(logDir, 0o755); err == nil {
			if f, err := os.OpenFile(
				filepath.Join(logDir, "app.log"),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND,
				0o644,
			); err == nil {
				writers = append(writers, f)
			}
		}
	}
	return log.New(io.MultiWriter(writers...), "[turtle] ", log.LstdFlags|log.Lmsgprefix)
}
