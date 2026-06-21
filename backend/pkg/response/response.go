// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package response provides common functions for forming HTTP responses.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON writes a JSON response with the specified status.
// Sets Content-Type: application/json.
// Logs encoding error and returns 500 Internal Server Error on failure.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, `{"error":"JSON marshal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(jsonData); err != nil {
		slog.Warn("response: failed to write JSON response", "error", err)
	}
}

// WriteError writes a structured error in JSON format.
// Parameter status - HTTP error code.
// Parameter message - error description (safe for client).
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{"code": status, "message": message})
}

