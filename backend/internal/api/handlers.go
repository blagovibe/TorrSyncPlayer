// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for torrent management, P2P rooms and synchronization.
package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	apperrors "github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/version"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// validateTorrentID validates the torrent identifier.
// ID must be a hex string of 40 characters (SHA1 infohash).
func validateTorrentID(id string) error {
	if len(id) != constants.TorrentIDLength {
		return fmt.Errorf("invalid torrent ID length: expected %d, got %d", constants.TorrentIDLength, len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		return fmt.Errorf("invalid torrent ID format: must be hex string")
	}
	return nil
}

// ── SSE Connection Management ──────────────────────────────────────────

const (
	// maxSSEConnections maximum number of concurrent SSE connections per room
	maxSSEConnections = constants.MaxSSEConnections

	// sseTimeout timeout for SSE connection
	sseTimeout = constants.SSETimeout

	// ssePingInterval ping interval for keeping SSE connection alive
	ssePingInterval = constants.SSEPingInterval
)

// sseConnectionManager manages active SSE connections
// with both per-room and global connection limits
type sseConnectionManager struct {
	mu        sync.Mutex
	counts    map[string]int
	maxConn   int // per room
	maxGlobal int // global limit (0 = unlimited)
	global    int
}

// newSSEConnectionManager creates an SSE connection manager
func newSSEConnectionManager(maxConn int) *sseConnectionManager {
	return &sseConnectionManager{
		counts:    make(map[string]int),
		maxConn:   maxConn,
		maxGlobal: maxConn * 10, // 10x room limit as global cap
	}
}

// tryAcquire tries to acquire a connection slot for the specified room
// Returns true if the connection is allowed
func (m *sseConnectionManager) tryAcquire(roomID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[roomID] >= m.maxConn {
		return false
	}
	if m.maxGlobal > 0 && m.global >= m.maxGlobal {
		return false
	}
	m.counts[roomID]++
	m.global++
	return true
}

// release releases a connection slot for the specified room
func (m *sseConnectionManager) release(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[roomID] > 0 {
		m.counts[roomID]--
		m.global--
	}
}

// count returns the total number of active connections across all rooms
func (m *sseConnectionManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.global
}

// sseManager global SSE connection manager
var sseManager = newSSEConnectionManager(maxSSEConnections)

// SSEEventHandler common function for handling SSE events.
// Used to eliminate SSE logic duplication across different handlers.
// Parameters:
//   - w: ResponseWriter for sending data
//   - r: HTTP request
//   - events: event channel for subscription
//   - roomID: room ID for connection limiting (empty string = no limit)
//   - logPath: path for logging
func SSEEventHandler(w http.ResponseWriter, r *http.Request, events <-chan models.P2PEvent, roomID string, logPath string) {
	if roomID != "" {
		if !sseManager.tryAcquire(roomID) {
			logger.Warn("SSE: connection limit exceeded for room", "roomID", roomID, "max", maxSSEConnections)
			WriteError(w, http.StatusTooManyRequests, "Too many connections for this room. Please try again later.")
			return
		}
		defer sseManager.release(roomID)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "Streaming is not supported")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sseTimeout)
	defer cancel()

	if _, err := fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n"); err != nil {
		logger.Warn("SSE: error sending initial event", "error", err, "path", logPath)
		return
	}
	flusher.Flush()

	pingTicker := time.NewTicker(ssePingInterval)
	defer pingTicker.Stop()

	serializationErrors := 0
	const maxSerializationErrors = 5

	for {
		select {
		case event, ok := <-events:
			if !ok {
				logger.Info("SSE event channel closed", "path", logPath)
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				serializationErrors++
				logger.Error("SSE event serialization error", "error", err, "eventType", event.Type, "serializationErrors", serializationErrors)
				if serializationErrors >= maxSerializationErrors {
					logger.Error("SSE: too many serialization errors, disconnecting", "path", logPath)
					if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"message\":\"Critical server error\"}\n\n"); writeErr == nil {
						flusher.Flush()
					}
					return
				}
				if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"message\":\"Event processing error\"}\n\n"); writeErr != nil {
					logger.Warn("SSE: write error", "error", writeErr, "path", logPath)
					return
				}
				flusher.Flush()
				continue
			}
			serializationErrors = 0
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data)); err != nil {
				logger.Warn("SSE: write error", "error", err, "path", logPath)
				return
			}
			flusher.Flush()

		case <-pingTicker.C:
			if _, err := fmt.Fprintf(w, "event: ping\ndata: {}\n\n"); err != nil {
				logger.Warn("SSE: error writing ping", "error", err, "path", logPath)
				return
			}
			flusher.Flush()

		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				logger.Info("SSE connection closed by timeout", "path", logPath)
				if _, err := fmt.Fprintf(w, "event: timeout\ndata: {\"message\":\"Connection timeout\"}\n\n"); err != nil {
					return
				}
				flusher.Flush()
			} else {
				logger.Info("SSE client closed connection", "path", logPath)
			}
			return
		}
	}
}

// ============ Error Handling ============

// handleError maps an error to an HTTP response using structured AppError types.
// Logs the full error server-side but returns only a safe message to the client.
func handleError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		if errors.Is(err, validation.ErrInvalidPosition) {
			appErr = apperrors.InvalidInput(err.Error())
		} else {
			appErr = apperrors.Internal(err.Error(), err)
		}
	}

	logger.Error("API error",
		"operation", operation,
		"path", r.URL.Path,
		"method", r.Method,
		"error_type", appErr.Type,
		"error", appErr.Err,
		"message", appErr.Message,
	)

	var statusCode int
	var clientMessage string

	switch appErr.Type {
	case apperrors.ErrNotFound:
		statusCode = http.StatusNotFound
		clientMessage = "Resource not found"
	case apperrors.ErrInvalidInput:
		statusCode = http.StatusBadRequest
		clientMessage = appErr.Message
	case apperrors.ErrUnauthorized:
		statusCode = http.StatusUnauthorized
		clientMessage = "Authentication required"
	case apperrors.ErrForbidden:
		statusCode = http.StatusForbidden
		clientMessage = "Access denied"
	case apperrors.ErrAlreadyExists:
		statusCode = http.StatusConflict
		clientMessage = appErr.Message
	case apperrors.ErrUnavailable:
		statusCode = http.StatusServiceUnavailable
		clientMessage = "Service temporarily unavailable"
	case apperrors.ErrTimeout:
		statusCode = http.StatusRequestTimeout
		clientMessage = "Request timed out"
	default:
		statusCode = http.StatusInternalServerError
		clientMessage = "Internal server error"
	}

	WriteError(w, statusCode, clientMessage)
}

// ============ Pagination Helpers ============

// parsePaginationParams extracts pagination parameters from the request
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = constants.DefaultPaginationLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > constants.MaxPaginationLimit {
				limit = constants.MaxPaginationLimit
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			if o > constants.MaxPaginationOffset {
				o = constants.MaxPaginationOffset
			}
			offset = o
		}
	}

	return limit, offset
}

// paginate is a generic helper that returns the sub-slice [offset:offset+limit]
// of items. Returns an empty slice when offset is out of range.
func paginate[T any](items []T, limit, offset int) []T {
	total := len(items)

	if offset >= total {
		return []T{}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return items[offset:end]
}

// HealthCheck public handler for server health check.
// Returns only basic status without service details.
// Does not require authentication - used for monitoring (load balancers, k8s probes).
// Returns:
//   - status: "ok" if the server is running
//
// @Summary      Health check
// @Description  Basic server health check (does not require authentication)
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /api/v1/health [get]
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// DetailedHealthCheck extended health check with service state verification.
// REQUIRES JWT AUTHENTICATION - available only to authorized users.
// Returns detailed information about service state:
//   - status: "ok" or "degraded"
//   - services: state of each service (torrent, p2p, sync)
//   - version: application version
//   - uptime: uptime in seconds
//
// Returns 503 if any service is unavailable.
func DetailedHealthCheck(torrentSvc internal.TorrentService, p2pSvc internal.P2PService, syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services := make(map[string]string)
		allHealthy := true

		// Check torrent service
		if torrentSvc != nil {
			services["torrent"] = "ok"
		} else {
			services["torrent"] = "unavailable"
			allHealthy = false
		}

		// Check p2p service
		if p2pSvc != nil {
			services["p2p"] = "ok"
		} else {
			services["p2p"] = "unavailable"
			allHealthy = false
		}

		// Check sync service
		if syncSvc != nil {
			services["sync"] = "ok"
		} else {
			services["sync"] = "unavailable"
			allHealthy = false
		}

		status := "ok"
		httpStatus := http.StatusOK
		if !allHealthy {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		response := map[string]interface{}{
			"status":   status,
			"services": services,
			"version":  version.Info(),
			"uptime":   metrics.GetInstance().GetUptime(),
		}

		WriteJSON(w, httpStatus, response)
	}
}

// MetricsHandler handler for Prometheus metrics.
// Returns metrics in Prometheus format for monitoring.
func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := metrics.GetInstance()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, m.FormatPrometheus()); err != nil {
			logger.Error("Metrics: failed to write response", "error", err)
		}
	}
}

// VersionHandler handler for getting server version information.
// Returns version, commit hash and build time.
func VersionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, version.Info())
	}
}

// WriteJSON writes a JSON response with the specified status.
// Delegates to pkg/response to keep a single JSON-encoding source of truth.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	response.WriteJSON(w, status, data)
}

// WriteError writes a structured error in JSON format ({"error": message}).
// Delegates to pkg/response.
func WriteError(w http.ResponseWriter, status int, message string) {
	response.WriteError(w, status, message)
}
