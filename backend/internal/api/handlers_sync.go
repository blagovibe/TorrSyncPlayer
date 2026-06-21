// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for playback synchronization.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
)

// SyncPlay handler for starting synchronized playback.
// Sets isPlaying = true and updates the timestamp.
//
// @Summary      Start playback
// @Description  Starts synchronized playback
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/play [post]
func SyncPlay(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Play(r.Context())
		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncPause handler for pausing synchronized playback.
// Sets isPlaying = false and updates the timestamp.
//
// @Summary      Pause playback
// @Description  Pauses synchronized playback
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/pause [post]
func SyncPause(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Pause(r.Context())
		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncSeek handler for synchronized seeking.
// Accepts JSON with the position field (in seconds).
// Validates position before applying.
//
// @Summary      Seek
// @Description  Performs synchronized seek to the specified position
// @Tags         sync
// @Accept       json
// @Produce      json
// @Param        request  body      models.SeekRequest  true  "Seek position"
// @Success      200      {object}  models.SyncStatus
// @Failure      400      {object}  APIError
// @Router       /api/v1/sync/seek [post]
func SyncSeek(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SeekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate position via centralized function
		if err := validation.ValidatePosition(req.Position); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid position: %s", err.Error()))
			return
		}

		status, err := syncSvc.Seek(r.Context(), req.Position)
		if err != nil {
			handleError(w, r, err, "seeking")
			return
		}

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncStatus handler for getting the current synchronization status.
// Returns position, duration and playback state.
//
// @Summary      Sync status
// @Description  Returns the current synchronization status
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/status [get]
func SyncStatus(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.GetStatus(r.Context())
		WriteJSON(w, http.StatusOK, status)
	}
}
