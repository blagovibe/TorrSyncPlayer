// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package response предоставляет общие функции для формирования HTTP ответов.
package response

import (
	"encoding/json"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// WriteJSON записывает JSON ответ с указанным статусом.
// Устанавливает Content-Type: application/json.
// Логирует ошибку кодирования и возвращает 500 Internal Server Error при сбое.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Ошибка кодирования JSON ответа", "error", err, "status", status)
		// Пытаемся отправить ошибку клиенту (если заголовки ещё не отправлены)
		http.Error(w, `{"error":"Внутренняя ошибка сервера"}`, http.StatusInternalServerError)
	}
}

// WriteError записывает структурированную ошибку в формате JSON.
// Параметр status - HTTP код ошибки.
// Параметр message - описание ошибки (безопасное для клиента).
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{"code": status, "message": message})
}
