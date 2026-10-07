// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains router for routing HTTP requests to handlers.
package api

import (
	"net/http"

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
	AuthService *auth.AuthService
}

// NewRouter creates and configures an HTTP router.
// Attaches middleware (SecurityHeaders, Recovery, CORS, ContentType, Logger, RateLimit, AccessToken) and registers routes.
// Parameter config - router configuration with services and the access-token service.
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

	// Health check — unauthenticated so monitoring works without the token
	r.Get(APIPathHealth, HealthCheck())

	// Version endpoint
	r.Get(APIPathVersion, VersionHandler())

	// Prometheus metrics endpoint (per-IP rate limited with stricter limits, without
	// the access token for monitoring tools)
	r.With(NewRateLimiter(rate.Limit(constants.MetricsRateLimit), constants.MetricsRateBurst)).Get(APIPathMetrics, MetricsHandler())

	// Public stream endpoint — authenticated via a signed stream ticket
	// (query param "?ticket=") rather than the access-token header, because media
	// players (libmpv) cannot attach headers to their own HTTP fetches.
	r.With(NewRateLimiter(rate.Limit(constants.StreamRateLimit), constants.StreamRateBurst)).Get("/api/v1/torrents/{id}/stream", StreamFile(config.TorrentSvc, config.AuthService))

	// Protected endpoints — per-IP rate limiting then access-token check.
	// No CSRF middleware: a cross-origin page cannot read the token and cannot
	// set X-Access-Token without a preflight the server does not approve.
	r.Group(func(r chi.Router) {
		r.Use(PerIPRateLimiter)                   // per-IP rate limiting (60 req/min) — applied first so a bad token cannot be used to flood
		r.Use(config.AuthService.TokenMiddleware) // access token + client id

		// API v1
		r.Route("/api/v1", func(r chi.Router) {
			// Torrent endpoints
			r.Route("/torrents", func(r chi.Router) {
				r.Get("/", ListTorrents(config.TorrentSvc))
				r.Post("/", AddTorrent(config.TorrentSvc))
				r.Delete("/{id}", RemoveTorrent(config.TorrentSvc))
				r.Get("/{id}/files", GetFiles(config.TorrentSvc))
				r.Post("/{id}/select", SelectFile(config.TorrentSvc))
				// NOTE: /{id}/stream is served publicly (outside this group) and
				// authenticated via a signed stream ticket, because libmpv cannot
				// attach the access-token header to its own HTTP fetches.
				r.Post("/{id}/stream-ticket", StreamTicket(config.TorrentSvc, config.AuthService))
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

			// Sync endpoints
			r.Route("/sync", func(r chi.Router) {
				r.Post("/play", SyncPlay(config.SyncSvc, config.P2pSvc))
				r.Post("/pause", SyncPause(config.SyncSvc, config.P2pSvc))
				r.Post("/seek", SyncSeek(config.SyncSvc, config.P2pSvc))
				r.Get("/status", SyncStatus(config.SyncSvc, config.P2pSvc))
			})

			// Detailed health check (requires the access token)
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
