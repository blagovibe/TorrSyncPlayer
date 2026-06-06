// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит общие функции для формирования HTTP ответов.
package api

import (
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// WriteJSON записывает JSON ответ с указанным статусом.
// Устанавливает Content-Type: application/json.
// Логирует ошибку кодирования и возвращает 500 Internal Server Error при сбое.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	response.WriteJSON(w, status, data)
}

// WriteError записывает структурированную ошибку в формате JSON.
// Параметр status - HTTP код ошибки.
// Параметр message - описание ошибки (безопасное для клиента).
func WriteError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, map[string]string{"error": message})
}
