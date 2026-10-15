// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for torrent management, P2P rooms and synchronization.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	apperrors "github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/version"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

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
// REQUIRES AN ACCESS TOKEN - available only to authorized clients that send
// the startup token in the X-Access-Token header.
// Returns detailed information about service state:
//   - status: "ok" or "degraded"
//   - services: state of each service (torrent)
//   - version: application version
//   - uptime: uptime in seconds
//
// Returns 503 if any service is unavailable.
//
// Rooms and sync used to be brokered here; playback commands travel peer to
// peer now, and the server keeps no room state at all (idea-0064).
func DetailedHealthCheck(torrentSvc internal.TorrentService) http.HandlerFunc {
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
