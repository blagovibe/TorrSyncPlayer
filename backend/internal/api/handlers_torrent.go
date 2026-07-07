// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains handlers for torrent management.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
)

// AddTorrent handler for adding a torrent via magnet link.
// Accepts JSON with the magnetURI field.
// Returns information about the added torrent or an error.
//
// @Summary      Add torrent
// @Description  Adds a torrent via magnet link and returns its information
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        request  body      models.AddTorrentRequest  true  "Magnet URI"
// @Success      201      {object}  models.TorrentInfo
// @Failure      400      {object}  APIError
// @Failure      500      {object}  APIError
// @Router       /api/v1/torrents [post]
func AddTorrent(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.AddTorrentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate magnet URI
		if err := validation.ValidateMagnetURI(req.MagnetURI); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid magnet URI format")
			return
		}

		info, err := torrentSvc.AddMagnet(r.Context(), req.MagnetURI)
		if err != nil {
			handleError(w, r, err, "adding torrent")
			return
		}

		metrics.GetInstance().TorrentAdded()
		WriteJSON(w, http.StatusCreated, info)
	}
}

// RemoveTorrent handler for removing a torrent by ID.
// ID is passed in the URL parameter.
// Returns 404 error if the torrent is not found.
//
// @Summary      Remove torrent
// @Description  Removes a torrent by its ID
// @Tags         torrents
// @Produce      json
// @Param        id   path      string  true  "Torrent ID"
// @Success      200  {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Failure      404      {object}  APIError
// @Router       /api/v1/torrents/{id} [delete]
func RemoveTorrent(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		// Use context from request for cancellation support
		if err := torrentSvc.RemoveTorrent(r.Context(), torrentID); err != nil {
			handleError(w, r, err, "removing torrent")
			return
		}

		metrics.GetInstance().TorrentRemoved()
		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Torrent removed"})
	}
}

// ListTorrents handler for getting a list of all torrents with pagination.
// Supports limit and offset parameters.
// Returns JSON with an array of torrents and pagination information.
//
// @Summary      List torrents
// @Description  Returns a list of all torrents with pagination
// @Tags         torrents
// @Produce      json
// @Param        limit   query     int  false  "Record limit"   default(20)  maximum(100)
// @Param        offset  query     int  false  "Offset"       default(0)
// @Success      200     {object}  models.TorrentListResponse
// @Router       /api/v1/torrents [get]
func ListTorrents(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePaginationParams(r)

		allTorrents := torrentSvc.ListTorrents(r.Context())
		totalCount := len(allTorrents)

		// Apply pagination
		paginatedTorrents := paginateTorrents(allTorrents, limit, offset)

		response := models.NewTorrentListResponse(paginatedTorrents, totalCount, limit, offset)
		WriteJSON(w, http.StatusOK, response)
	}
}

// GetFiles handler for getting a list of torrent files with pagination.
// Torrent ID is passed in the URL parameter.
// Supports limit and offset parameters.
// Returns an array of files with indices, names and sizes.
//
// @Summary      List torrent files
// @Description  Returns a list of torrent files with pagination
// @Tags         torrents
// @Produce      json
// @Param        id      path      string  true  "Torrent ID"
// @Param        limit   query     int     false "Record limit"  default(20)  maximum(100)
// @Param        offset  query     int     false "Offset"      default(0)
// @Success      200     {object}  models.FileListResponse
// @Failure      400     {object}  APIError
// @Failure      404     {object}  APIError
// @Router       /api/v1/torrents/{id}/files [get]
func GetFiles(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		limit, offset := parsePaginationParams(r)

		allFiles, err := torrentSvc.GetFiles(r.Context(), torrentID)
		if err != nil {
			handleError(w, r, err, "getting files")
			return
		}

		totalCount := len(allFiles)

		// Apply pagination
		paginatedFiles := paginateFiles(allFiles, limit, offset)

		response := models.NewFileListResponse(paginatedFiles, totalCount, limit, offset)
		WriteJSON(w, http.StatusOK, response)
	}
}

// SelectFile handler for selecting a file for streaming.
// Accepts JSON with the fileIndex field.
// Sets download priority for the selected file.
//
// @Summary      Select file for streaming
// @Description  Selects a torrent file for streaming by index
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "Torrent ID"
// @Param        request  body      models.SelectFileRequest   true  "File index"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/torrents/{id}/select [post]
func SelectFile(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SelectFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate fileIndex - check for negative values
		if req.FileIndex < 0 {
			WriteError(w, http.StatusBadRequest, "File index cannot be negative")
			return
		}

		if err := torrentSvc.SelectFile(r.Context(), torrentID, req.FileIndex); err != nil {
			handleError(w, r, err, "selecting file")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "File selected"})
	}
}

// StreamFile handler for HTTP streaming of a torrent file.
// Supports Range requests for seeking.
// Torrent ID is passed in the URL parameter.
//
// @Summary      Stream file
// @Description  Streams the selected torrent file with Range request support
// @Tags         torrents
// @Produce      octet-stream
// @Param        id   path      string  true  "Torrent ID"
// @Success      200  {file}    binary
// @Failure      400  {object}  APIError
// @Failure      404  {object}  APIError
// @Router       /api/v1/torrents/{id}/stream [get]
func StreamFile(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		torrentSvc.ServeFile(w, r, torrentID)
	}
}

// ============ Buffer Handlers ============

// SetBufferPosition handler for setting the current playback position for buffering.
// Accepts JSON with the position field (position in bytes).
// Updates piece download priorities based on the new position.
//
// @Summary      Set buffer position
// @Description  Sets the current playback position for buffer optimization
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        id       path      string                           true  "Torrent ID"
// @Param        request  body      models.SetBufferPositionRequest  true  "Position in bytes"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/torrents/{id}/buffer/position [post]
func SetBufferPosition(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SetBufferPositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request format")
			return
		}

		// Validate position
		if req.Position < 0 {
			WriteError(w, http.StatusBadRequest, "Position cannot be negative")
			return
		}

		if err := torrentSvc.UpdateBufferPosition(r.Context(), torrentID, req.Position); err != nil {
			handleError(w, r, err, "updating buffer position")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Position updated"})
	}
}

// GetBufferInfo handler for getting buffer state information.
// Returns information about the current position, buffer boundaries and download percentage.
//
// @Summary      Buffer info
// @Description  Returns buffer state information for the torrent
// @Tags         torrents
// @Produce      json
// @Param        id   path      string  true  "Torrent ID"
// @Success      200  {object}  models.BufferInfo
// @Failure      400  {object}  APIError
// @Failure      404      {object}  APIError
// @Router       /api/v1/torrents/{id}/buffer/info [get]
func GetBufferInfo(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "Torrent ID is required")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid torrent ID")
			return
		}

		info, err := torrentSvc.GetBufferInfo(r.Context(), torrentID)
		if err != nil {
			handleError(w, r, err, "getting buffer info")
			return
		}

		WriteJSON(w, http.StatusOK, info)
	}
}
