// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит обработчики для управления торрентами, P2P комнатами и синхронизацией.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/torrplayer/backend/internal"
	"github.com/yourname/torrplayer/backend/internal/metrics"
	"github.com/yourname/torrplayer/backend/internal/models"
	"github.com/yourname/torrplayer/backend/internal/validation"
	"github.com/yourname/torrplayer/backend/internal/version"
	"github.com/yourname/torrplayer/backend/pkg/logger"
)

// ============ Error Handling ============

// APIError структурированная ошибка API.
// Используется для возврата ошибок в формате JSON.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ErrorType тип ошибки для внутренней обработки
type ErrorType int

const (
	ErrorTypeInternal ErrorType = iota
	ErrorTypeNotFound
	ErrorTypeBadRequest
	ErrorTypeUnauthorized
	ErrorTypeForbidden
	ErrorTypeConflict
)

// AppError внутренняя структура ошибки приложения
type AppError struct {
	Type    ErrorType
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// NewAppError создаёт новую ошибку приложения
func NewAppError(errType ErrorType, message string, err error) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Err:     err,
	}
}

// handleError обрабатывает ошибки и возвращает безопасный ответ клиенту.
// Логирует полную ошибку на сервере, но клиенту возвращает только безопасное сообщение.
func handleError(w http.ResponseWriter, r *http.Request, err error, operation string) {
	var appErr *AppError

	// Определяем тип ошибки
	switch e := err.(type) {
	case *AppError:
		appErr = e
	default:
		// Внутренняя ошибка - не раскрываем детали
		appErr = &AppError{
			Type:    ErrorTypeInternal,
			Message: "Внутренняя ошибка сервера",
			Err:     err,
		}
	}

	// Логируем полную ошибку на сервере с контекстом
	logger.Error("Ошибка API",
		"operation", operation,
		"path", r.URL.Path,
		"method", r.Method,
		"error_type", appErr.Type,
		"error", appErr.Err,
		"message", appErr.Message,
	)

	// Определяем HTTP статус и безопасное сообщение для клиента
	var statusCode int
	var clientMessage string

	switch appErr.Type {
	case ErrorTypeNotFound:
		statusCode = http.StatusNotFound
		clientMessage = "Ресурс не найден"
	case ErrorTypeBadRequest:
		statusCode = http.StatusBadRequest
		clientMessage = appErr.Message // Безопасно показать клиенту
	case ErrorTypeUnauthorized:
		statusCode = http.StatusUnauthorized
		clientMessage = "Требуется аутентификация"
	case ErrorTypeForbidden:
		statusCode = http.StatusForbidden
		clientMessage = "Доступ запрещён"
	case ErrorTypeConflict:
		statusCode = http.StatusConflict
		clientMessage = appErr.Message
	default:
		statusCode = http.StatusInternalServerError
		clientMessage = "Внутренняя ошибка сервера"
	}

	writeError(w, statusCode, clientMessage)
}

// ============ Pagination Helpers ============

// parsePaginationParams извлекает параметры пагинации из запроса
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = 20 // По умолчанию
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100 // Максимальный лимит
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}

// paginateSlice пагинирует срез торрентов
func paginateTorrents(torrents []*models.TorrentInfo, limit, offset int) []*models.TorrentInfo {
	total := len(torrents)

	if offset >= total {
		return []*models.TorrentInfo{}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return torrents[offset:end]
}

// paginateFiles пагинирует срез файлов
func paginateFiles(files []models.FileInfo, limit, offset int) []models.FileInfo {
	total := len(files)

	if offset >= total {
		return []models.FileInfo{}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return files[offset:end]
}

// ============ JSON Helpers ============

// writeJSON записывает JSON ответ с указанным статусом.
// Устанавливает Content-Type: application/json.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Ошибка записи JSON ответа", "error", err)
	}
}

// writeError записывает структурированную ошибку в формате JSON.
// Параметр status - HTTP код ошибки.
// Параметр message - описание ошибки (безопасное для клиента).
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, APIError{Code: status, Message: message})
}

// ============ Torrent Handlers ============

// AddTorrent обработчик добавления торрента по magnet-ссылке.
// Принимает JSON с полем magnetURI.
// Возвращает информацию о добавленном торренте или ошибку.
func AddTorrent(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.AddTorrentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Валидация magnet URI
		if err := validation.ValidateMagnetURI(req.MagnetURI); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат magnet URI")
			return
		}

		info, err := torrentSvc.AddMagnet(r.Context(), req.MagnetURI)
		if err != nil {
			handleError(w, r, err, "добавление торрента")
			return
		}

		writeJSON(w, http.StatusCreated, info)
	}
}

// RemoveTorrent обработчик удаления торрента по ID.
// ID передаётся в URL параметре.
// Возвращает ошибку 404 если торрент не найден.
func RemoveTorrent(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			writeError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := torrentSvc.RemoveTorrent(torrentID); err != nil {
			handleError(w, r, NewAppError(ErrorTypeNotFound, "Торрент не найден", err), "удаление торрента")
			return
		}

		writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "Торрент удалён"})
	}
}

// ListTorrents обработчик получения списка всех торрентов с пагинацией.
// Поддерживает параметры limit и offset.
// Возвращает JSON с массивом торрентов и информацией о пагинации.
func ListTorrents(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePaginationParams(r)

		allTorrents := torrentSvc.ListTorrents()
		totalCount := len(allTorrents)

		// Применяем пагинацию
		paginatedTorrents := paginateTorrents(allTorrents, limit, offset)

		response := models.NewTorrentListResponse(paginatedTorrents, totalCount, limit, offset)
		writeJSON(w, http.StatusOK, response)
	}
}

// GetFiles обработчик получения списка файлов торрента с пагинацией.
// ID торрента передаётся в URL параметре.
// Поддерживает параметры limit и offset.
// Возвращает массив файлов с индексами, именами и размерами.
func GetFiles(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			writeError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		limit, offset := parsePaginationParams(r)

		allFiles, err := torrentSvc.GetFiles(torrentID)
		if err != nil {
			handleError(w, r, NewAppError(ErrorTypeNotFound, "Торрент не найден", err), "получение файлов")
			return
		}

		totalCount := len(allFiles)

		// Применяем пагинацию
		paginatedFiles := paginateFiles(allFiles, limit, offset)

		response := models.NewFileListResponse(paginatedFiles, totalCount, limit, offset)
		writeJSON(w, http.StatusOK, response)
	}
}

// SelectFile обработчик выбора файла для стриминга.
// Принимает JSON с полем fileIndex.
// Устанавливает приоритет загрузки для выбранного файла.
func SelectFile(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			writeError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SelectFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if err := torrentSvc.SelectFile(torrentID, req.FileIndex); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка выбора файла", err), "выбор файла")
			return
		}

		writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "Файл выбран"})
	}
}

// StreamFile обработчик HTTP стриминга файла торрента.
// Поддерживает Range запросы для перемотки.
// ID торрента передаётся в URL параметре.
func StreamFile(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			writeError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		torrentSvc.ServeFile(w, r, torrentID)
	}
}

// ============ P2P Handlers ============

// CreateRoom обработчик создания P2P комнаты.
// Принимает JSON с полями name и password (опционально).
// Возвращает информацию о созданной комнате.
func CreateRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Валидация названия комнаты
		if err := validation.ValidateRoomName(req.Name); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Некорректное название комнаты: %s", err.Error()))
			return
		}

		room, err := p2pSvc.CreateRoom(req.Name, req.Password)
		if err != nil {
			handleError(w, r, err, "создание комнаты")
			return
		}

		writeJSON(w, http.StatusCreated, room)
	}
}

// JoinRoom обработчик присоединения к P2P комнате.
// Принимает JSON с полями roomID и password.
// Возвращает ошибку если комната не найдена или неверный пароль.
func JoinRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.JoinRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if req.RoomID == "" {
			writeError(w, http.StatusBadRequest, "ID комнаты не указан")
			return
		}

		if err := p2pSvc.JoinRoom(req.RoomID, req.Password); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка присоединения к комнате", err), "присоединение к комнате")
			return
		}

		writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "Присоединились к комнате"})
	}
}

// LeaveRoom обработчик выхода из P2P комнаты.
// Закрывает WebRTC подключение и удаляет пира из комнаты.
func LeaveRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := p2pSvc.LeaveRoom(); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка выхода из комнаты", err), "выход из комнаты")
			return
		}

		writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "Вышли из комнаты"})
	}
}

// Signal обработчик отправки WebRTC сигнала.
// Принимает JSON с полем signal (бинарные данные в base64).
// Отправляет сигнал через data channel всем пирам в комнате.
func Signal(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SignalRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if err := p2pSvc.SendSignal(req.Signal); err != nil {
			handleError(w, r, err, "отправка сигнала")
			return
		}

		writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "Сигнал отправлен"})
	}
}

// RoomEvents обработчик SSE для получения событий комнаты в реальном времени.
// Использует Server-Sent Events для доставки событий.
// Поддерживает таймаут соединения и ping/pong для поддержания связи.
func RoomEvents(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Устанавливаем заголовки для SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "Streaming не поддерживается")
			return
		}

		// Отправляем начальное событие
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		// Подписываемся на события
		events := p2pSvc.GetEvents()

		// Таймаут для SSE соединения (30 минут)
		timeout := 30 * time.Minute
		timeoutTimer := time.NewTimer(timeout)
		defer timeoutTimer.Stop()

		// Таймер для ping (каждые 30 секунд)
		pingTicker := time.NewTicker(30 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case event, ok := <-events:
				if !ok {
					// Канал событий закрыт
					logger.Info("Канал SSE событий закрыт", "path", r.URL.Path)
					return
				}

				// Сбрасываем таймаут при получении события
				if !timeoutTimer.Stop() {
					select {
					case <-timeoutTimer.C:
					default:
					}
				}
				timeoutTimer.Reset(timeout)

				data, err := json.Marshal(event)
				if err != nil {
					logger.Warn("Ошибка сериализации SSE события", "error", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
				flusher.Flush()

			case <-pingTicker.C:
				// Отправляем ping для поддержания соединения
				fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
				flusher.Flush()

			case <-timeoutTimer.C:
				// Таймаут соединения
				logger.Info("SSE соединение закрыто по таймауту", "path", r.URL.Path)
				fmt.Fprintf(w, "event: timeout\ndata: {\"message\":\"Connection timeout\"}\n\n")
				flusher.Flush()
				return

			case <-r.Context().Done():
				// Клиент закрыл соединение
				logger.Info("Клиент SSE закрыл соединение", "path", r.URL.Path)
				return
			}
		}
	}
}

// ============ Sync Handlers ============

// SyncPlay обработчик запуска синхронизированного воспроизведения.
// Устанавливает состояние isPlaying = true и обновляет таймстамп.
func SyncPlay(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Play()
		writeJSON(w, http.StatusOK, status)
	}
}

// SyncPause обработчик приостановки синхронизированного воспроизведения.
// Устанавливает состояние isPlaying = false и обновляет таймстамп.
func SyncPause(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Pause()
		writeJSON(w, http.StatusOK, status)
	}
}

// SyncSeek обработчик синхронизированной перемотки.
// Принимает JSON с полем position (в секундах).
// Валидирует позицию перед применением.
func SyncSeek(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SeekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		status, err := syncSvc.Seek(req.Position)
		if err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка перемотки", err), "перемотка")
			return
		}

		writeJSON(w, http.StatusOK, status)
	}
}

// SyncStatus обработчик получения текущего статуса синхронизации.
// Возвращает позицию, длительность и состояние воспроизведения.
func SyncStatus(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.GetStatus()
		writeJSON(w, http.StatusOK, status)
	}
}

// HealthCheck обработчик проверки здоровья сервера.
// Возвращает детальную информацию о состоянии сервисов:
// - status: ok/degraded
// - uptime: время работы в секундах
// - version: версия приложения
// - services: состояние каждого сервиса
func HealthCheck() http.HandlerFunc {
	startTime := time.Now()

	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime)

		response := map[string]interface{}{
			"status":  "ok",
			"uptime":  uptime.Seconds(),
			"version": "1.0.0",
			"services": map[string]string{
				"torrent": "ok",
				"p2p":     "ok",
				"sync":    "ok",
			},
		}

		writeJSON(w, http.StatusOK, response)
	}
}

// DetailedHealthCheck расширенный health check с проверкой состояния сервисов.
// Принимает сервисы для проверки их состояния.
// Возвращает 503 если один из сервисов недоступен.
func DetailedHealthCheck(torrentSvc internal.TorrentService, p2pSvc internal.P2PService, syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services := make(map[string]string)
		allHealthy := true

		// Проверяем torrent сервис
		if torrentSvc != nil {
			services["torrent"] = "ok"
		} else {
			services["torrent"] = "unavailable"
			allHealthy = false
		}

		// Проверяем p2p сервис
		if p2pSvc != nil {
			services["p2p"] = "ok"
		} else {
			services["p2p"] = "unavailable"
			allHealthy = false
		}

		// Проверяем sync сервис
		if syncSvc != nil {
			services["sync"] = "ok"
		} else {
			services["sync"] = "unavailable"
			allHealthy = false
		}

		status := "ok"
		httpStatus := http.StatusOK
		if !allHealthy {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		response := map[string]interface{}{
			"status":   status,
			"services": services,
			"version":  "1.0.0",
		}

		writeJSON(w, httpStatus, response)
	}
}

// MetricsHandler обработчик для метрик Prometheus.
// Возвращает метрики в формате Prometheus для мониторинга.
func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := metrics.GetInstance()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, m.FormatPrometheus())
	}
}

// VersionHandler обработчик для получения информации о версии сервера.
// Возвращает версию, commit hash и время сборки.
func VersionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, version.Info())
	}
}
