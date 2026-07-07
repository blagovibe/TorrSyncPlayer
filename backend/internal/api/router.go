// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains router for routing HTTP requests to handlers.
package api

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	// swagger docs import for side effects
	_ "github.com/blagovibe/TorrSyncPlayer/backend/docs"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// RouterConfig router configuration.
type RouterConfig struct {
	TorrentSvc  internal.TorrentService
	P2pSvc      internal.P2PService
	SyncSvc     internal.SyncService
	AuthStore   *auth.UserStore
	AuthService *auth.AuthService
	JWTTokenTTL time.Duration
}

// NewRouter creates and configures an HTTP router.
// Attaches middleware (SecurityHeaders, Recovery, CORS, ContentType, CSRF, Logger, RateLimit, Auth) and registers routes.
// Parameter config - router configuration with services and store.
// Returns a configured http.Handler.
func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Attach base middleware (order matters!)
	r.Use(SecurityHeadersMiddleware) // 1. Security headers (first layer)
	r.Use(Recovery)                  // 2. Panic recovery
	r.Use(CORS)                      // 3. CORS handling
	r.Use(ContentTypeMiddleware)     // 4. Content-Type validation
	r.Use(Logger)                    // 5. Logging

	// Swagger UI (without rate limiting for development convenience)
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// Health check (without CSRF and JWT authentication for monitoring)
	r.Get(APIPathHealth, HealthCheck())

	// Version endpoint
	r.Get(APIPathVersion, VersionHandler())

	// Prometheus metrics endpoint (per-IP rate limited, without CSRF/JWT for monitoring tools)
	r.With(PerIPRateLimiter).Get(APIPathMetrics, MetricsHandler())

// CSRF token endpoint for obtaining a token (per-IP rate limited with stricter limits)
		r.With(NewRateLimiter(rate.Limit(constants.CSRFRateLimit), constants.CSRFRateBurst)).Get(APIPathCSRFToken, func(w http.ResponseWriter, r *http.Request) {
			sessionID := extractSessionID(r)
			token, err := CSRFStore.generateToken(sessionID)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "Token generation error")
				return
			}
			w.Header().Set("X-CSRF-Token", token)
			WriteJSON(w, http.StatusOK, map[string]string{
				"csrfToken": token,
			})
		})

	// Apply JWT TTL if configured
	if config.JWTTokenTTL > 0 {
		config.AuthService.SetTokenTTL(config.JWTTokenTTL)
	}

	// Create auth handler
	authHandler := auth.NewAuthHandler(config.AuthStore, config.AuthService)

// Auth endpoints — without CSRF protection (public endpoints)
		// Rate limiting: 10 requests/minute (per-IP via NewRateLimiter)
		r.Route("/api/v1/auth", func(r chi.Router) {
			r.Use(NewRateLimiter(rate.Limit(0.17), 5))
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
		})

	// Protected endpoints — with Rate limiting, CSRF and JWT authentication
	r.Group(func(r chi.Router) {
		r.Use(NewRateLimiter(rate.Limit(1), 10)) // 60 requests/minute (per-IP) — applied before CSRF to prevent DoS on token store
		r.Use(CSRFMiddleware)                    // CSRF protection
		r.Use(config.AuthService.JWTMiddleware)  // JWT authentication

		// API v1
		r.Route("/api/v1", func(r chi.Router) {
			// Torrent endpoints
			r.Route("/torrents", func(r chi.Router) {
				r.Get("/", ListTorrents(config.TorrentSvc))
				r.Post("/", AddTorrent(config.TorrentSvc))
				r.Delete("/{id}", RemoveTorrent(config.TorrentSvc))
				r.Get("/{id}/files", GetFiles(config.TorrentSvc))
				r.Post("/{id}/select", SelectFile(config.TorrentSvc))
				r.Get("/{id}/stream", StreamFile(config.TorrentSvc))
				r.Post("/{id}/buffer/position", SetBufferPosition(config.TorrentSvc))
				r.Get("/{id}/buffer/info", GetBufferInfo(config.TorrentSvc))
			})

			// P2P endpoints
			r.Route("/rooms", func(r chi.Router) {
				r.Post("/", CreateRoom(config.P2pSvc))
				r.Post("/join", JoinRoom(config.P2pSvc))
				r.Post("/leave", LeaveRoom(config.P2pSvc))
				r.Post("/signal", Signal(config.P2pSvc))
				r.Get("/{roomID}/events", RoomEvents(config.P2pSvc))
			})

// Auth endpoints (protected — require JWT + CSRF)
				r.Post("/auth/logout", config.AuthService.LogoutHandler)
				r.Post("/auth/change-password", authHandler.ChangePassword)

			// Sync endpoints
			r.Route("/sync", func(r chi.Router) {
				r.Post("/play", SyncPlay(config.SyncSvc))
				r.Post("/pause", SyncPause(config.SyncSvc))
				r.Post("/seek", SyncSeek(config.SyncSvc))
				r.Get("/status", SyncStatus(config.SyncSvc))
			})

			// Detailed health check (requires JWT authentication)
			r.Get("/health/detailed", DetailedHealthCheck(config.TorrentSvc, config.P2pSvc, config.SyncSvc))
		})

		// 404 handler for unknown routes within the protected group
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			WriteError(w, http.StatusNotFound, "Route not found")
		})
	})

	// 404 handler for all other unknown routes
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "Route not found")
	})

	logger.Info("HTTP router configured")
	return r
}
