// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for playback synchronization.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// SyncPlay handler for starting synchronized playback.
// Sets isPlaying = true and updates the timestamp.
// Broadcasts sync event to all room participants via SSE.
//
// @Summary      Start playback
// @Description  Starts synchronized playback
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/play [post]
func SyncPlay(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		roomInfo, _ := p2pSvc.GetRoomInfo(r.Context(), userID)
		roomID := ""
		if roomInfo != nil {
			roomID = roomInfo.ID
		}

		status := syncSvc.Play(roomID)

		// Broadcast to room participants if user is in a room
		if roomInfo != nil {
			p2pSvc.BroadcastSync(roomInfo.ID, map[string]interface{}{
				"action":    "play",
				"status":    status,
				"initiator": userID,
			})
		}

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncPause handler for pausing synchronized playback.
// Sets isPlaying = false and updates the timestamp.
// Broadcasts sync event to all room participants via SSE.
//
// @Summary      Pause playback
// @Description  Pauses synchronized playback
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/pause [post]
func SyncPause(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		roomInfo, _ := p2pSvc.GetRoomInfo(r.Context(), userID)
		roomID := ""
		if roomInfo != nil {
			roomID = roomInfo.ID
		}

		status := syncSvc.Pause(roomID)

		// Broadcast to room participants if user is in a room
		if roomInfo != nil {
			p2pSvc.BroadcastSync(roomInfo.ID, map[string]interface{}{
				"action":    "pause",
				"status":    status,
				"initiator": userID,
			})
		}

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncSeek handler for synchronized seeking.
// Accepts JSON with the position field (in seconds).
// Validates position before applying.
// Broadcasts sync event to all room participants via SSE.
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
func SyncSeek(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SeekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate position via centralized function
		if err := validation.ValidatePosition(req.Position); err != nil {
			logger.Warn("SyncSeek: invalid position provided", "error", err)
			WriteError(w, http.StatusBadRequest, "Invalid position")
			return
		}

		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		roomInfo, _ := p2pSvc.GetRoomInfo(r.Context(), userID)
		roomID := ""
		if roomInfo != nil {
			roomID = roomInfo.ID
		}

		status, err := syncSvc.Seek(roomID, req.Position)
		if err != nil {
			handleError(w, r, err, "seeking")
			return
		}

		// Broadcast to room participants if user is in a room
		if roomInfo != nil {
			p2pSvc.BroadcastSync(roomInfo.ID, map[string]interface{}{
				"action":    "seek",
				"position":  req.Position,
				"status":    status,
				"initiator": userID,
			})
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
func SyncStatus(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine the room from the caller's current membership.
		roomID := ""
		if claims := auth.GetClaims(r); claims != nil {
			if roomInfo, err := p2pSvc.GetRoomInfo(r.Context(), claims.UserID); err == nil && roomInfo != nil {
				roomID = roomInfo.ID
			}
		}
		status := syncSvc.GetStatus(roomID)
		WriteJSON(w, http.StatusOK, status)
	}
}
