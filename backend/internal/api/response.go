// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains common functions for forming HTTP responses.
package api

import (
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// WriteJSON writes a JSON response with the specified status.
// Sets Content-Type: application/json.
// Logs encoding error and returns 500 Internal Server Error on failure.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	response.WriteJSON(w, status, data)
}

// WriteError writes a structured error in JSON format.
// Parameter status - HTTP error code.
// Parameter message - error description (safe for client).
func WriteError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, map[string]string{"error": message})
}
