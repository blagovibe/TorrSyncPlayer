// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for P2P room management.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// CreateRoom handler for creating a P2P room.
// Accepts JSON with name and password (optional) fields.
// Returns information about the created room.
//
// @Summary      Create room
// @Description  Creates a new P2P room for playback synchronization
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.CreateRoomRequest  true  "Room data"
// @Success      201      {object}  models.RoomInfo
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms [post]
func CreateRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate room name
		if err := validation.ValidateRoomName(req.Name); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid room name: %s", err.Error()))
			return
		}

		if req.Password != "" && len(req.Password) < 6 {
			WriteError(w, http.StatusBadRequest, "Room password must be at least 6 characters")
			return
		}
		if req.Password != "" && len(req.Password) > constants.MaxRoomPasswordLength {
			WriteError(w, http.StatusBadRequest, "Room password cannot exceed 72 characters")
			return
		}

		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		room, err := p2pSvc.CreateRoom(r.Context(), userID, req.Name, req.Password)
		if err != nil {
			handleError(w, r, err, "creating room")
			return
		}

		metrics.GetInstance().RoomCreated()
		WriteJSON(w, http.StatusCreated, room)
	}
}

// JoinRoom handler for joining a P2P room.
// Accepts JSON with roomID and password fields.
// Returns an error if the room is not found or password is wrong.
// Does NOT log the room password.
//
// @Summary      Join room
// @Description  Joins a user to an existing P2P room
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.JoinRoomRequest  true  "Login data"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms/join [post]
func JoinRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.JoinRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		if req.RoomID == "" {
			WriteError(w, http.StatusBadRequest, "Room ID is required")
			return
		}

		// Validate roomID format (hex string of 32 characters)
		if err := validation.ValidateRoomID(req.RoomID); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid room ID: %s", err.Error()))
			return
		}

		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		// Do NOT log req.Password - password must not appear in logs
		if err := p2pSvc.JoinRoom(r.Context(), userID, req.RoomID, req.Password); err != nil {
			logger.Warn("P2P: failed to join room", "roomID", req.RoomID, "error", err)
			handleError(w, r, err, "joining room")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Joined the room"})
	}
}

// LeaveRoom handler for leaving a P2P room.
// Closes WebRTC connection and removes the peer from the room.
//
// @Summary      Leave room
// @Description  Leaves the current P2P room
// @Tags         rooms
// @Produce      json
// @Success      200  {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms/leave [post]
func LeaveRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		if err := p2pSvc.LeaveRoom(r.Context(), userID); err != nil {
			handleError(w, r, err, "leaving room")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Left the room"})
	}
}

// Signal handler for sending WebRTC signals.
// Accepts JSON with the signal field (binary data in base64).
// Sends the signal via data channel to all peers in the room.
//
// @Summary      Send WebRTC signal
// @Description  Sends a WebRTC signal (SDP offer/answer, ICE candidate) via data channel
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.SignalRequest  true  "WebRTC signal"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms/signal [post]
func Signal(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size for DoS protection
		// Uses MaxSignalSize (64KB) instead of MaxRequestSize (1MB)
		// since WebRTC signals typically do not exceed 8KB
		r.Body = http.MaxBytesReader(w, r.Body, constants.MaxSignalSize)

		var req models.SignalRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate roomID if provided
		if req.RoomID != "" {
			if err := validation.ValidateRoomID(req.RoomID); err != nil {
				WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid room ID: %s", err.Error()))
				return
			}
		}

		// Validate signal size
		if len(req.Signal) > constants.MaxSignalSize {
			WriteError(w, http.StatusBadRequest, "Signal exceeds maximum allowed size")
			return
		}

		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		if err := p2pSvc.SendSignal(r.Context(), userID, req.Signal); err != nil {
			handleError(w, r, err, "sending signal")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Signal sent"})
	}
}

// RoomEvents SSE handler for receiving real-time room events.
// Uses Server-Sent Events for event delivery.
// Supports connection timeout, ping/pong and connection limit.
// Checks user's room membership before subscribing.
//
// @Summary      Room events (SSE)
// @Description  Subscribe to P2P room events in real-time via Server-Sent Events
// @Tags         rooms
// @Produce      text/event-stream
// @Param        roomID  path  string  true  "Room ID"
// @Success      200     {string}  stream
// @Router       /api/v1/rooms/{roomID}/events [get]
func RoomEvents(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomID := chi.URLParam(r, "roomID")

		// Validate roomID format
		if err := validation.ValidateRoomID(roomID); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid room ID: %s", err.Error()))
			return
		}

		// Get user ID from JWT claims
		claims := auth.GetClaims(r)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		// Check room membership before subscribing to SSE
		roomInfo, err := p2pSvc.GetRoomInfo(r.Context(), userID)
		if err != nil || roomInfo == nil || roomInfo.ID != roomID {
			logger.Warn("SSE: attempt to subscribe to another room", "roomID", roomID, "error", err)
			WriteError(w, http.StatusForbidden, "You are not a member of this room")
			return
		}

		events := p2pSvc.GetEvents(userID)
		if events == nil {
			WriteError(w, http.StatusServiceUnavailable, "Event streaming not yet implemented for multi-session")
			return
		}

		SSEEventHandler(w, r, events, roomID, r.URL.Path)
	}
}
