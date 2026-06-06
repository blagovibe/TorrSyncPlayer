// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package response предоставляет общие функции для формирования HTTP ответов.
package response

import (
	"encoding/json"
	"net/http"
)

// WriteJSON записывает JSON ответ с указанным статусом.
// Устанавливает Content-Type: application/json.
// Логирует ошибку кодирования и возвращает 500 Internal Server Error при сбое.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, `{"error":"JSON marshal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(jsonData)
}

// WriteError записывает структурированную ошибку в формате JSON.
// Параметр status - HTTP код ошибки.
// Параметр message - описание ошибки (безопасное для клиента).
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{"code": status, "message": message})
}
