package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"torrsyncplayer/logger"
)

// App структура приложения с dependency injection
type App struct {
	ctx            context.Context
	TorrentService TorrentServiceInterface
	P2PService     P2PServiceInterface
	SyncService    SyncServiceInterface
	shutdownChan   chan struct{}
}

// NewApp создает новый экземпляр приложения с внедренными зависимостями
func NewApp(torrent TorrentServiceInterface, p2p P2PServiceInterface, sync SyncServiceInterface) *App {
	return &App{
		TorrentService: torrent,
		P2PService:     p2p,
		SyncService:    sync,
		shutdownChan:   make(chan struct{}),
	}
}

// startup вызывается при запуске приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Info("TorrSyncPlayer started")

	// Настройка graceful shutdown
	go a.handleShutdown()

	// Инициализируем торрент-сервис
	if err := a.TorrentService.Init(ctx); err != nil {
		logger.Error("Failed to initialize TorrentService", "error", err)
	}

	// Инициализируем P2P сервис
	if err := a.P2PService.Init(ctx); err != nil {
		logger.Error("Failed to initialize P2PService", "error", err)
	}

	// Инициализируем сервис синхронизации
	if err := a.SyncService.Init(ctx); err != nil {
		logger.Error("Failed to initialize SyncService", "error", err)
	}

	// Устанавливаем P2P сервис для SyncService после инициализации
	a.SyncService.SetP2PService(a.P2PService)
}

// handleShutdown обрабатывает сигналы завершения
func (a *App) handleShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig)
	case <-a.shutdownChan:
		logger.Info("Shutdown requested")
	}

	a.Shutdown()
}

// Shutdown выполняет корректное завершение работы
func (a *App) Shutdown() {
	logger.Info("Starting graceful shutdown...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		// Останавливаем сервисы в правильном порядке
		if a.SyncService != nil {
			logger.Info("Stopping sync service...")
			// SyncService не требует explicit stop
		}

		if a.P2PService != nil {
			logger.Info("Closing P2P connections...")
			if err := a.P2PService.Close(); err != nil {
				logger.Error("Error closing P2P service", "error", err)
			}
		}

		if a.TorrentService != nil {
			logger.Info("Stopping HTTP server...")
			if err := a.TorrentService.StopHTTPServer(); err != nil {
				logger.Error("Error stopping HTTP server", "error", err)
			}
		}
	}()

	select {
	case <-done:
		logger.Info("Graceful shutdown completed")
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	}
}

// shutdown вызывается при завершении приложения (Wails)
func (a *App) shutdown(ctx context.Context) {
	a.Shutdown()
}

// GetAppInfo возвращает информацию о приложении
func (a *App) GetAppInfo() map[string]string {
	return map[string]string{
		"name":    "TorrSyncPlayer",
		"version": "1.0.0",
	}
}
