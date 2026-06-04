// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит обработчики для управления торрентами, P2P комнатами и синхронизацией.
package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/version"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/go-chi/chi/v5"
)

// validateTorrentID валидирует идентификатор торрента.
// ID должен быть hex-строкой длиной 40 символов (SHA1 infohash).
func validateTorrentID(id string) error {
	if len(id) != constants.TorrentIDLength {
		return fmt.Errorf("invalid torrent ID length: expected %d, got %d", constants.TorrentIDLength, len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		return fmt.Errorf("invalid torrent ID format: must be hex string")
	}
	return nil
}

// ── SSE Connection Management ──────────────────────────────────────────

const (
	// maxSSEConnections максимальное количество одновременных SSE соединений на комнату
	maxSSEConnections = constants.MaxSSEConnections

	// sseTimeout таймаут для SSE соединения
	sseTimeout = constants.SSETimout

	// ssePingInterval интервал отправки ping для поддержания SSE соединения
	ssePingInterval = constants.SSEPingInterval
)

// sseConnectionManager управляет активными SSE соединениями по комнатам
type sseConnectionManager struct {
	mu      sync.Mutex
	counts  map[string]int
	maxConn int
}

// newSSEConnectionManager создаёт менеджер SSE соединений
func newSSEConnectionManager(maxConn int) *sseConnectionManager {
	return &sseConnectionManager{
		counts:  make(map[string]int),
		maxConn: maxConn,
	}
}

// tryAcquire пытается получить разрешение на новое соединение для указанной комнаты
// Возвращает true если соединение разрешено
func (m *sseConnectionManager) tryAcquire(roomID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[roomID] >= m.maxConn {
		return false
	}
	m.counts[roomID]++
	return true
}

// release освобождает слот соединения для указанной комнаты
func (m *sseConnectionManager) release(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts[roomID] > 0 {
		m.counts[roomID]--
	}
}

// count возвращает общее количество активных соединений по всем комнатам
func (m *sseConnectionManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, count := range m.counts {
		total += count
	}
	return total
}

// sseManager глобальный менеджер SSE соединений
var sseManager = newSSEConnectionManager(maxSSEConnections)

// SSEEventHandler общая функция для обработки SSE событий.
// Используется для устранения дублирования логики SSE в разных обработчиках.
// Параметры:
//   - w: ResponseWriter для отправки данных
//   - r: HTTP запрос
//   - events: канал событий для подписки
//   - roomID: ID комнаты для ограничения соединений (пустая строка = без ограничений)
//   - logPath: путь для логирования
func SSEEventHandler(w http.ResponseWriter, r *http.Request, events <-chan models.P2PEvent, roomID string, logPath string) {
	// Проверяем лимит соединений если указан roomID
	if roomID != "" {
		if !sseManager.tryAcquire(roomID) {
			logger.Warn("SSE: превышен лимит соединений для комнаты", "roomID", roomID, "max", maxSSEConnections)
			WriteError(w, http.StatusTooManyRequests, "Слишком много соединений для этой комнаты. Попробуйте позже.")
			return
		}
		defer sseManager.release(roomID)
	}

	// Устанавливаем заголовки для SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "Streaming не поддерживается")
		return
	}

	// Создаём контекст с таймаутом для SSE соединения
	ctx, cancel := context.WithTimeout(r.Context(), sseTimeout)
	defer cancel()

	// Отправляем начальное событие
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
	flusher.Flush()

	// Таймер для ping (каждые 30 секунд)
	pingTicker := time.NewTicker(ssePingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Канал событий закрыт
				logger.Info("Канал SSE событий закрыт", "path", logPath)
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				logger.Error("Ошибка сериализации SSE события", "error", err, "eventType", event.Type)
				_, _ = fmt.Fprintf(w, "event: error\ndata: {\"message\":\"Ошибка обработки события\",\"type\":\"%s\"}\n\n", event.Type)
				flusher.Flush()
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
			flusher.Flush()

		case <-pingTicker.C:
			// Отправляем ping для поддержания соединения
			_, _ = fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()

		case <-ctx.Done():
			// Таймаут соединения или отмена контекста
			if ctx.Err() == context.DeadlineExceeded {
				logger.Info("SSE соединение закрыто по таймауту", "path", logPath)
				_, _ = fmt.Fprintf(w, "event: timeout\ndata: {\"message\":\"Connection timeout\"}\n\n")
				flusher.Flush()
			} else {
				logger.Info("Клиент SSE закрыл соединение", "path", logPath)
			}
			return
		}
	}
}

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

	WriteError(w, statusCode, clientMessage)
}

// ============ Pagination Helpers ============

// parsePaginationParams извлекает параметры пагинации из запроса
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = constants.DefaultPaginationLimit
	offset = 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > constants.MaxPaginationLimit {
				limit = constants.MaxPaginationLimit
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

// ============ Torrent Handlers ============

// AddTorrent обработчик добавления торрента по magnet-ссылке.
// Принимает JSON с полем magnetURI.
// Возвращает информацию о добавленном торренте или ошибку.
//
// @Summary      Добавить торрент
// @Description  Добавляет торрент по magnet-ссылке и возвращает информацию о нём
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
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.AddTorrentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Валидация magnet URI
		if err := validation.ValidateMagnetURI(req.MagnetURI); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат magnet URI")
			return
		}

		info, err := torrentSvc.AddMagnet(r.Context(), req.MagnetURI)
		if err != nil {
			handleError(w, r, err, "добавление торрента")
			return
		}

		WriteJSON(w, http.StatusCreated, info)
	}
}

// RemoveTorrent обработчик удаления торрента по ID.
// ID передаётся в URL параметре.
// Возвращает ошибку 404 если торрент не найден.
//
// @Summary      Удалить торрент
// @Description  Удаляет торрент по его ID
// @Tags         torrents
// @Produce      json
// @Param        id   path      string  true  "Torrent ID"
// @Success      200  {object}  models.SuccessResponse
// @Failure      400  {object}  APIError
// @Failure      404  {object}  APIError
// @Router       /api/v1/torrents/{id} [delete]
func RemoveTorrent(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
			return
		}

		if err := torrentSvc.RemoveTorrent(torrentID); err != nil {
			handleError(w, r, NewAppError(ErrorTypeNotFound, "Торрент не найден", err), "удаление торрента")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Торрент удалён"})
	}
}

// ListTorrents обработчик получения списка всех торрентов с пагинацией.
// Поддерживает параметры limit и offset.
// Возвращает JSON с массивом торрентов и информацией о пагинации.
//
// @Summary      Список торрентов
// @Description  Возвращает список всех торрентов с пагинацией
// @Tags         torrents
// @Produce      json
// @Param        limit   query     int  false  "Лимит записей"   default(20)  maximum(100)
// @Param        offset  query     int  false  "Смещение"       default(0)
// @Success      200     {object}  models.TorrentListResponse
// @Router       /api/v1/torrents [get]
func ListTorrents(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePaginationParams(r)

		allTorrents := torrentSvc.ListTorrents()
		totalCount := len(allTorrents)

		// Применяем пагинацию
		paginatedTorrents := paginateTorrents(allTorrents, limit, offset)

		response := models.NewTorrentListResponse(paginatedTorrents, totalCount, limit, offset)
		WriteJSON(w, http.StatusOK, response)
	}
}

// GetFiles обработчик получения списка файлов торрента с пагинацией.
// ID торрента передаётся в URL параметре.
// Поддерживает параметры limit и offset.
// Возвращает массив файлов с индексами, именами и размерами.
//
// @Summary      Список файлов торрента
// @Description  Возвращает список файлов торрента с пагинацией
// @Tags         torrents
// @Produce      json
// @Param        id      path      string  true  "Torrent ID"
// @Param        limit   query     int     false "Лимит записей"  default(20)  maximum(100)
// @Param        offset  query     int     false "Смещение"      default(0)
// @Success      200     {object}  models.FileListResponse
// @Failure      400     {object}  APIError
// @Failure      404     {object}  APIError
// @Router       /api/v1/torrents/{id}/files [get]
func GetFiles(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
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
		WriteJSON(w, http.StatusOK, response)
	}
}

// SelectFile обработчик выбора файла для стриминга.
// Принимает JSON с полем fileIndex.
// Устанавливает приоритет загрузки для выбранного файла.
//
// @Summary      Выбрать файл для стриминга
// @Description  Выбирает файл торрента для стриминга по индексу
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "Torrent ID"
// @Param        request  body      models.SelectFileRequest   true  "Индекс файла"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/torrents/{id}/select [post]
func SelectFile(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
			return
		}

		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SelectFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if err := torrentSvc.SelectFile(torrentID, req.FileIndex); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка выбора файла", err), "выбор файла")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Файл выбран"})
	}
}

// StreamFile обработчик HTTP стриминга файла торрента.
// Поддерживает Range запросы для перемотки.
// ID торрента передаётся в URL параметре.
//
// @Summary      Стриминг файла
// @Description  Стримит выбранный файл торрента с поддержкой Range запросов
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
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
			return
		}

		torrentSvc.ServeFile(w, r, torrentID)
	}
}

// ============ Buffer Handlers ============

// SetBufferPosition обработчик установки текущей позиции воспроизведения для буферизации.
// Принимает JSON с полем position (позиция в байтах).
// Обновляет приоритеты загрузки кусков на основе новой позиции.
//
// @Summary      Установить позицию буфера
// @Description  Устанавливает текущую позицию воспроизведения для оптимизации буферизации
// @Tags         torrents
// @Accept       json
// @Produce      json
// @Param        id       path      string                           true  "Torrent ID"
// @Param        request  body      models.SetBufferPositionRequest  true  "Позиция в байтах"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/torrents/{id}/buffer/position [post]
func SetBufferPosition(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
			return
		}

		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SetBufferPositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Валидация позиции
		if req.Position < 0 {
			WriteError(w, http.StatusBadRequest, "Позиция не может быть отрицательной")
			return
		}

		torrentSvc.UpdateBufferPosition(torrentID, req.Position)

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Позиция обновлена"})
	}
}

// GetBufferInfo обработчик получения информации о состоянии буфера.
// Возвращает информацию о текущей позиции, границах буфера и процент загрузки.
//
// @Summary      Информация о буфере
// @Description  Возвращает информацию о состоянии буферизации для торрента
// @Tags         torrents
// @Produce      json
// @Param        id   path      string  true  "Torrent ID"
// @Success      200  {object}  models.BufferInfo
// @Failure      400  {object}  APIError
// @Failure      404  {object}  APIError
// @Router       /api/v1/torrents/{id}/buffer/info [get]
func GetBufferInfo(torrentSvc internal.TorrentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := chi.URLParam(r, "id")
		if torrentID == "" {
			WriteError(w, http.StatusBadRequest, "ID торрента не указан")
			return
		}

		if err := validateTorrentID(torrentID); err != nil {
			WriteError(w, http.StatusBadRequest, "Некорректный ID торрента")
			return
		}

		info, err := torrentSvc.GetBufferInfo(torrentID)
		if err != nil {
			handleError(w, r, NewAppError(ErrorTypeNotFound, "Информация о буфере не найдена", err), "получение информации о буфере")
			return
		}

		WriteJSON(w, http.StatusOK, info)
	}
}

// ============ P2P Handlers ============

// CreateRoom обработчик создания P2P комнаты.
// Принимает JSON с полями name и password (опционально).
// Возвращает информацию о созданной комнате.
//
// @Summary      Создать комнату
// @Description  Создаёт новую P2P комнату для синхронизации воспроизведения
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.CreateRoomRequest  true  "Данные комнаты"
// @Success      201      {object}  models.RoomInfo
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms [post]
func CreateRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.CreateRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		// Валидация названия комнаты
		if err := validation.ValidateRoomName(req.Name); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Некорректное название комнаты: %s", err.Error()))
			return
		}

		room, err := p2pSvc.CreateRoom(req.Name, req.Password)
		if err != nil {
			handleError(w, r, err, "создание комнаты")
			return
		}

		WriteJSON(w, http.StatusCreated, room)
	}
}

// JoinRoom обработчик присоединения к P2P комнате.
// Принимает JSON с полями roomID и password.
// Возвращает ошибку если комната не найдена или неверный пароль.
//
// @Summary      Присоединиться к комнате
// @Description  Присоединяет пользователя к существующей P2P комнате
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.JoinRoomRequest  true  "Данные для входа"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms/join [post]
func JoinRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.JoinRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if req.RoomID == "" {
			WriteError(w, http.StatusBadRequest, "ID комнаты не указан")
			return
		}

		// Валидация формата roomID (hex строка длиной 32 символа)
		if err := validation.ValidateRoomID(req.RoomID); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Некорректный ID комнаты: %s", err.Error()))
			return
		}

		if err := p2pSvc.JoinRoom(req.RoomID, req.Password); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка присоединения к комнате", err), "присоединение к комнате")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Присоединились к комнате"})
	}
}

// LeaveRoom обработчик выхода из P2P комнаты.
// Закрывает WebRTC подключение и удаляет пира из комнаты.
//
// @Summary      Покинуть комнату
// @Description  Выходит из текущей P2P комнаты
// @Tags         rooms
// @Produce      json
// @Success      200  {object}  models.SuccessResponse
// @Failure      400  {object}  APIError
// @Router       /api/v1/rooms/leave [post]
func LeaveRoom(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := p2pSvc.LeaveRoom(); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка выхода из комнаты", err), "выход из комнаты")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Вышли из комнаты"})
	}
}

// Signal обработчик отправки WebRTC сигнала.
// Принимает JSON с полем signal (бинарные данные в base64).
// Отправляет сигнал через data channel всем пирам в комнате.
//
// @Summary      Отправить WebRTC сигнал
// @Description  Отправляет WebRTC сигнал (SDP offer/answer, ICE candidate) через data channel
// @Tags         rooms
// @Accept       json
// @Produce      json
// @Param        request  body      models.SignalRequest  true  "WebRTC сигнал"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  APIError
// @Router       /api/v1/rooms/signal [post]
func Signal(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SignalRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		if err := p2pSvc.SendSignal(req.Signal); err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка отправки сигнала", err), "отправка сигнала")
			return
		}

		WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Сигнал отправлен"})
	}
}

// RoomEvents обработчик SSE для получения событий комнаты в реальном времени.
// Использует Server-Sent Events для доставки событий.
// Поддерживает таймаут соединения, ping/pong и ограничение количества соединений.
//
// @Summary      События комнаты (SSE)
// @Description  Подписка на события P2P комнаты в реальном времени через Server-Sent Events
// @Tags         rooms
// @Produce      text/event-stream
// @Param        roomID  path  string  true  "Room ID"
// @Success      200     {string}  stream
// @Router       /api/v1/rooms/{roomID}/events [get]
func RoomEvents(p2pSvc internal.P2PService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomID := chi.URLParam(r, "roomID")
		events := p2pSvc.GetEvents()
		SSEEventHandler(w, r, events, roomID, r.URL.Path)
	}
}

// ============ Sync Handlers ============

// SyncPlay обработчик запуска синхронизированного воспроизведения.
// Устанавливает состояние isPlaying = true и обновляет таймстамп.
//
// @Summary      Запустить воспроизведение
// @Description  Запускает синхронизированное воспроизведение
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/play [post]
func SyncPlay(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Play()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncPause обработчик приостановки синхронизированного воспроизведения.
// Устанавливает состояние isPlaying = false и обновляет таймстамп.
//
// @Summary      Приостановить воспроизведение
// @Description  Приостанавливает синхронизированное воспроизведение
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/pause [post]
func SyncPause(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.Pause()
		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncSeek обработчик синхронизированной перемотки.
// Принимает JSON с полем position (в секундах).
// Валидирует позицию перед применением.
//
// @Summary      Перемотка
// @Description  Выполняет синхронизированную перемотку на указанную позицию
// @Tags         sync
// @Accept       json
// @Produce      json
// @Param        request  body      models.SeekRequest  true  "Позиция перемотки"
// @Success      200      {object}  models.SyncStatus
// @Failure      400      {object}  APIError
// @Router       /api/v1/sync/seek [post]
func SyncSeek(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ограничиваем размер тела запроса для защиты от DoS
		r.Body = http.MaxBytesReader(w, r.Body, validation.MaxRequestSize)

		var req models.SeekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Неверный формат запроса")
			return
		}

		status, err := syncSvc.Seek(req.Position)
		if err != nil {
			handleError(w, r, NewAppError(ErrorTypeBadRequest, "Ошибка перемотки", err), "перемотка")
			return
		}

		WriteJSON(w, http.StatusOK, status)
	}
}

// SyncStatus обработчик получения текущего статуса синхронизации.
// Возвращает позицию, длительность и состояние воспроизведения.
//
// @Summary      Статус синхронизации
// @Description  Возвращает текущий статус синхронизации воспроизведения
// @Tags         sync
// @Produce      json
// @Success      200  {object}  models.SyncStatus
// @Router       /api/v1/sync/status [get]
func SyncStatus(syncSvc internal.SyncService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := syncSvc.GetStatus()
		WriteJSON(w, http.StatusOK, status)
	}
}

// HealthCheck публичный обработчик проверки здоровья сервера.
// Возвращает только базовый статус без деталей о сервисах.
// Не требует аутентификации - используется для мониторинга (load balancers, k8s probes).
// Возвращает:
//   - status: "ok" если сервер работает
//
// @Summary      Проверка здоровья
// @Description  Базовая проверка работоспособности сервера (не требует аутентификации)
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /api/v1/health [get]
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"status": "ok",
		}

		WriteJSON(w, http.StatusOK, response)
	}
}

// DetailedHealthCheck расширенный health check с проверкой состояния сервисов.
// ТРЕБУЕТ JWT АУТЕНТИФИКАЦИИ - доступен только авторизованным пользователям.
// Возвращает детальную информацию о состоянии сервисов:
//   - status: "ok" или "degraded"
//   - services: состояние каждого сервиса (torrent, p2p, sync)
//   - version: версия приложения
//   - uptime: время работы в секундах
//
// Возвращает 503 если один из сервисов недоступен.
func DetailedHealthCheck(torrentSvc internal.TorrentService, p2pSvc internal.P2PService, syncSvc internal.SyncService) http.HandlerFunc {
	startTime := time.Now()

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
			"version":  version.Info(),
			"uptime":   time.Since(startTime).Seconds(),
		}

		WriteJSON(w, httpStatus, response)
	}
}

// MetricsHandler обработчик для метрик Prometheus.
// Возвращает метрики в формате Prometheus для мониторинга.
func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := metrics.GetInstance()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, m.FormatPrometheus())
	}
}

// VersionHandler обработчик для получения информации о версии сервера.
// Возвращает версию, commit hash и время сборки.
func VersionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, version.Info())
	}
}
