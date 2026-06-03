// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит роутер для маршрутизации HTTP запросов к обработчикам.
package api

import (
	"net/http"

	_ "github.com/blagovibe/TorrSyncPlayer/backend/docs"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"golang.org/x/time/rate"
)

// RouterConfig конфигурация роутера.
type RouterConfig struct {
	TorrentSvc  internal.TorrentService
	P2pSvc      internal.P2PService
	SyncSvc     internal.SyncService
	AuthStore   *auth.UserStore
	AuthService *auth.AuthService
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

	// Swagger UI
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// Health check (без rate limiting и аутентификации для мониторинга)
	r.Get(APIPathHealth, HealthCheck())

	// Version endpoint
	r.Get(APIPathVersion, VersionHandler())

	// Prometheus metrics endpoint
	r.Get(APIPathMetrics, MetricsHandler())

	// CSRF token endpoint для получения токена
	r.Get(APIPathCSRFToken, func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем session ID из запроса
		sessionID := extractSessionID(r)
		token, err := csrfStore.generateToken(sessionID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Ошибка генерации токена")
			return
		}
		w.Header().Set("X-CSRF-Token", token)
		WriteJSON(w, http.StatusOK, map[string]string{
			"csrfToken": token,
		})
	})

	// Создаём обработчик аутентификации
	authHandler := auth.NewAuthHandler(config.AuthStore, config.AuthService)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Auth endpoints с более строгим лимитом (10/мин) и без аутентификации
		r.Route("/auth", func(r chi.Router) {
			r.Use(NewRateLimiter(rate.Limit(0.17), 5)) // ~10 запросов/минуту
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/logout", config.AuthService.LogoutHandler)
		})

		// Защищённые endpoints
		r.Group(func(r chi.Router) {
			// Rate limiting: 60 запросов/минуту (1/сек, burst 10)
			r.Use(NewRateLimiter(rate.Limit(1), 10))
			// JWT аутентификация
			r.Use(config.AuthService.JWTMiddleware)

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
				r.Get("/{roomID}/events", RoomEvents(config.P2pSvc))
			})

			// Sync endpoints
			r.Route("/sync", func(r chi.Router) {
				r.Post("/play", SyncPlay(config.SyncSvc))
				r.Post("/pause", SyncPause(config.SyncSvc))
				r.Post("/seek", SyncSeek(config.SyncSvc))
				r.Get("/status", SyncStatus(config.SyncSvc))
			})

			// Detailed health check (требует JWT аутентификации)
			r.Get("/health/detailed", DetailedHealthCheck(config.TorrentSvc, config.P2pSvc, config.SyncSvc))
		})
	})

	logger.Info("HTTP роутер настроен")
	return r
}
