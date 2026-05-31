package main

import (
	"embed"
	"fmt"

	"torrsyncplayer/container"
	"torrsyncplayer/logger"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set during build via -ldflags
var version = "dev"

func main() {
	fmt.Printf("TorrSyncPlayer v%s\n", version)
	// Создаем DI контейнер
	c := container.New()

	// Регистрируем сервисы в контейнере
	c.RegisterSingleton("torrent", func() interface{} {
		return NewTorrentService()
	})
	c.RegisterSingleton("p2p", func() interface{} {
		return NewP2PService()
	})
	c.RegisterSingleton("sync", func() interface{} {
		return NewSyncService()
	})

	// Получаем сервисы из контейнера
	torrentService := c.ResolveOrPanic("torrent").(TorrentServiceInterface)
	p2pService := c.ResolveOrPanic("p2p").(P2PServiceInterface)
	syncService := c.ResolveOrPanic("sync").(SyncServiceInterface)

	// Создаем приложение с внедренными сервисами
	app := NewApp(torrentService, p2pService, syncService)

	err := wails.Run(&options.App{
		Title:  "TorrSyncPlayer",
		Width:  1280,
		Height: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
			app.TorrentService,
			app.P2PService,
			app.SyncService,
		},
	})

	if err != nil {
		logger.Error("Failed to start application", "error", err)
	}
}
