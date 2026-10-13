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
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// SyncPlay handler for starting synchronized playback.
// Only the room host can call this.
// Sets isPlaying = true and updates the timestamp.
// Broadcasts sync event to all room participants via SSE.
//
// @Summary      Start playback
// @Description  Starts synchronized playback (host only)
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Failure      400  {object}  models.ErrorResponse
// @Failure      403  {object}  models.ErrorResponse
// @Router       /api/v1/sync/play [post]
func SyncPlay(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomInfo, userID, ok := resolveCallerRoom(w, r, p2pSvc)
		if !ok {
			return
		}

		status, err := syncSvc.Play(r.Context(), roomInfo.ID, userID)
		if err != nil {
			handleError(w, r, err, "starting playback")
			return
		}

		// Broadcast to room participants
		p2pSvc.BroadcastSync(roomInfo.ID,
			models.NewSyncEvent(models.SyncActionPlay, status, userID))

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncPause handler for pausing synchronized playback.
// Only the room host can call this.
// Sets isPlaying = false and updates the timestamp.
// Broadcasts sync event to all room participants via SSE.
//
// @Summary      Pause playback
// @Description  Pauses synchronized playback (host only)
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Failure      400  {object}  models.ErrorResponse
// @Failure      403  {object}  models.ErrorResponse
// @Router       /api/v1/sync/pause [post]
func SyncPause(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomInfo, userID, ok := resolveCallerRoom(w, r, p2pSvc)
		if !ok {
			return
		}

		status, err := syncSvc.Pause(r.Context(), roomInfo.ID, userID)
		if err != nil {
			handleError(w, r, err, "pausing playback")
			return
		}

		// Broadcast to room participants
		p2pSvc.BroadcastSync(roomInfo.ID,
			models.NewSyncEvent(models.SyncActionPause, status, userID))

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncSeek handler for synchronized seeking.
// Only the room host can call this.
// Accepts JSON with the position field (in seconds).
// Validates position before applying.
// Broadcasts sync event to all room participants via SSE.
//
// @Summary      Seek
// @Description  Performs synchronized seek to the specified position (host only)
// @Tags         sync
// @Accept       json
// @Produce      json
// @Param        request  body      models.SeekRequest  true  "Seek position"
// @Success      200      {object}  models.SyncStatus
// @Failure      400      {object}  models.ErrorResponse
// @Failure      403      {object}  models.ErrorResponse
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

		roomInfo, userID, ok := resolveCallerRoom(w, r, p2pSvc)
		if !ok {
			return
		}

		status, err := syncSvc.Seek(r.Context(), roomInfo.ID, userID, req.Position)
		if err != nil {
			handleError(w, r, err, "seeking")
			return
		}

		// Broadcast to room participants
		// The payload carries the position the host ASKED for, while status
		// carries where smoothing actually moved the room. Both are needed.
		p2pSvc.BroadcastSync(roomInfo.ID,
			models.NewSyncEvent(models.SyncActionSeek, status, userID).
				WithRequestedPosition(req.Position))

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncBuffering handler for reporting a peer's buffering state.
// Any peer may report its own buffering state (not host-only, idea-0018):
// buffering is a local condition, so whoever runs out of data says so and the
// room follows. Restricting it to the host would strand the room, because a
// non-host peer that recovered could never say so and the pause would not lift.
//
// Room membership is enforced HERE, by resolving the caller's own room: a peer
// that is not in a room gets 400 "Not connected to a room", never 403.
// Broadcasts a sync event with the new buffering flag to all participants.
// Peers that receive it pause/resume playback to stay in sync.
//
// @Summary      Report buffering state
// @Description  Reports that the calling peer started or stopped buffering, so the room pauses/resumes with it. Any room member may report it; a peer not in a room gets 400.
// @Tags         sync
// @Accept       json
// @Produce      json
// @Param        request  body      models.BufferingRequest  true  "Buffering state"
// @Success      200      {object}  models.SyncStatus
// @Failure      400      {object}  models.ErrorResponse
// @Router       /api/v1/sync/buffering [post]
func SyncBuffering(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.BufferingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Membership check. Buffering has no host gate, so this lookup is the only
		// thing standing between an anonymous caller and the room's state:
		// resolving the caller's own room fails for anyone who is not in one.
		roomInfo, userID, ok := resolveCallerRoom(w, r, p2pSvc)
		if !ok {
			return
		}

		status, err := syncSvc.Buffering(r.Context(), roomInfo.ID, userID, req.Buffering)
		if err != nil {
			handleError(w, r, err, "reporting buffering state")
			return
		}

		p2pSvc.BroadcastSync(roomInfo.ID,
			models.NewSyncEvent(models.SyncActionBuffering, status, userID))

		// The roster has to name who is buffering, not merely that the room is
		// buffering: the room keeps one shared status and cannot answer that on
		// its own (idea-0017). Recorded on the peer, then published.
		p2pSvc.SetPeerBuffering(r.Context(), userID, req.Buffering)
		p2pSvc.BroadcastPeerList(roomInfo.ID)

		metrics.GetInstance().SyncOperation()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncStatus handler for getting the current synchronization status.
// Returns position, duration, playback state and buffering flag.
//
// The room comes from the caller's own membership; a caller outside any room
// gets 400 like every other sync endpoint (bug-0019). It used to answer 200
// with a zeroed status instead: GetStatus("") returns IsPlaying:false,
// Position:0 for an unknown room, which a client could not tell apart from a
// real room sitting at position 0. After a reconnect that ambiguity made the
// client pause and jump to 0 in a room actually playing at 42 minutes.
//
// @Summary      Sync status
// @Description  Returns the current synchronization status of the caller's room. A caller outside any room gets 400.
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Failure      400  {object}  models.ErrorResponse
// @Router       /api/v1/sync/status [get]
func SyncStatus(syncSvc internal.SyncService, p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomInfo, _, ok := resolveCallerRoom(w, r, p2pSvc)
		if !ok {
			return
		}

		status := syncSvc.GetStatus(roomInfo.ID)
		WriteJSON(w, http.StatusOK, status)
	}
}
