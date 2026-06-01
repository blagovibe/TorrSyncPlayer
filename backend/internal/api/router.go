// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит роутер для маршрутизации HTTP запросов к обработчикам.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/torrplayer/backend/internal"
	"github.com/yourname/torrplayer/backend/internal/auth"
	"github.com/yourname/torrplayer/backend/pkg/logger"
	"golang.org/x/time/rate"
)

// RouterConfig конфигурация роутера.
type RouterConfig struct {
	TorrentSvc internal.TorrentService
	P2pSvc     internal.P2PService
	SyncSvc    internal.SyncService
	AuthStore  *auth.UserStore
}

// NewRouter создаёт и настраивает HTTP роутер.
// Подключает middleware (SecurityHeaders, Recovery, CORS, CSRF, Logger, RateLimit, Auth) и регистрирует маршруты.
// Параметр config - конфигурация роутера с сервисами и хранилищем.
// Возвращает настроенный http.Handler.
func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Подключаем глобальные middleware (порядок важен!)
	r.Use(SecurityHeadersMiddleware) // 1. Заголовки безопасности (первый слой)
	r.Use(Recovery)                  // 2. Перехват паник
	r.Use(CORS)                      // 3. CORS обработка
	r.Use(Logger)                    // 4. Логирование
	r.Use(CSRFMiddleware)            // 5. CSRF защита

	// Health check (без rate limiting и аутентификации для мониторинга)
	r.Get("/health", HealthCheck())

	// Расширенный health check с проверкой сервисов
	r.Get("/health/detailed", DetailedHealthCheck(config.TorrentSvc, config.P2pSvc, config.SyncSvc))

	// Version endpoint
	r.Get("/api/v1/version", VersionHandler())

	// Prometheus metrics endpoint
	r.Get("/metrics", MetricsHandler())

	// CSRF token endpoint для получения токена
	r.Get("/api/v1/csrf-token", func(w http.ResponseWriter, r *http.Request) {
		token, err := csrfStore.generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Ошибка генерации токена")
			return
		}
		w.Header().Set("X-CSRF-Token", token)
		writeJSON(w, http.StatusOK, map[string]string{
			"csrfToken": token,
		})
	})

	// Создаём обработчик аутентификации
	authHandler := auth.NewAuthHandler(config.AuthStore)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Auth endpoints с более строгим лимитом (10/мин) и без аутентификации
		r.Route("/auth", func(r chi.Router) {
			r.Use(NewRateLimiter(rate.Limit(0.17), 5)) // ~10 запросов/минуту
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
		})

		// Защищённые endpoints
		r.Group(func(r chi.Router) {
			// Rate limiting: 60 запросов/минуту (1/сек, burst 10)
			r.Use(NewRateLimiter(rate.Limit(1), 10))
			// JWT аутентификация
			r.Use(auth.JWTMiddleware)

			// Torrent endpoints
			r.Route("/torrents", func(r chi.Router) {
				r.Get("/", ListTorrents(config.TorrentSvc))
				r.Post("/", AddTorrent(config.TorrentSvc))
				r.Delete("/{id}", RemoveTorrent(config.TorrentSvc))
				r.Get("/{id}/files", GetFiles(config.TorrentSvc))
				r.Post("/{id}/select", SelectFile(config.TorrentSvc))
				r.Get("/{id}/stream", StreamFile(config.TorrentSvc))
			})

			// P2P endpoints
			r.Route("/rooms", func(r chi.Router) {
				r.Post("/", CreateRoom(config.P2pSvc))
				r.Post("/join", JoinRoom(config.P2pSvc))
				r.Post("/leave", LeaveRoom(config.P2pSvc))
				r.Post("/signal", Signal(config.P2pSvc))
				r.Get("/events", RoomEvents(config.P2pSvc))
			})

			// Sync endpoints
			r.Route("/sync", func(r chi.Router) {
				r.Post("/play", SyncPlay(config.SyncSvc))
				r.Post("/pause", SyncPause(config.SyncSvc))
				r.Post("/seek", SyncSeek(config.SyncSvc))
				r.Get("/status", SyncStatus(config.SyncSvc))
			})
		})
	})

	logger.Info("HTTP роутер настроен")
	return r
}
