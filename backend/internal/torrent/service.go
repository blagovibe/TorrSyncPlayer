// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package torrent предоставляет сервис для работы с торрентами.
// Управляет добавлением, удалением и стримингом торрентов через anacrolix/torrent.
// Использует структурированное логирование с контекстом операций.
package torrent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/storage"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Константы сервиса торрентов
const (
	gracefulShutdownTimeout = constants.TorrentGracefulShutdownTimeout
	maxTorrents             = 100
	maxStreamFileSize       = 100 * 1024 * 1024 * 1024 // 100 GB
)

// Service сервис управления торрентами.
// Предоставляет методы для добавления, удаления и стриминга торрентов.
// Потокобезопасен благодаря использованию sync.RWMutex.
// Всегда использует in-memory хранилище для данных торрентов.
type Service struct {
	mu            sync.RWMutex
	client        *torrent.Client
	torrents      map[string]*torrent.Torrent
	selectedFiles map[string]int // torrentID -> fileIndex
	bufferService *buffer.Service
}

// ServiceOptions содержит опции для настройки торрент-сервиса
type ServiceOptions struct {
	// NoDHT отключает DHT (для тестов)
	NoDHT bool
	// DisableUTP отключает UTP (для тестов)
	DisableUTP bool
	// DisableTCP отключает TCP (для тестов)
	DisableTCP bool
	// ListenPort порт для прослушивания (0 = случайный)
	ListenPort int
	// MemoryStorageCapacity максимальный размер in-memory хранилища (0 = без ограничений)
	MemoryStorageCapacity int64
}

// readCloserWithClose обёртка для io.ReadSeekCloser с безопасным закрытием.
// Гарантирует идемпотентное закрытие (Close можно вызывать многократно).
type readCloserWithClose struct {
	io.ReadSeekCloser
	closed bool
	mu     sync.Mutex
}

// Close закрывает reader только один раз (идемпотентное закрытие)
func (r *readCloserWithClose) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.ReadSeekCloser.Close()
}

// NewService создаёт новый сервис торрентов с in-memory хранилищем.
// Параметр bufferService - сервис буферизации (может быть nil).
// Возвращает инициализированный сервис или ошибку.
func NewService(bufferService *buffer.Service) (*Service, error) {
	return NewServiceWithOptions(bufferService, ServiceOptions{})
}

// NewServiceWithOptions создаёт новый сервис торрентов с расширенными опциями.
// Позволяет настроить параметры сети для тестирования.
// Всегда использует in-memory хранилище для данных торрентов.
func NewServiceWithOptions(bufferService *buffer.Service, opts ServiceOptions) (*Service, error) {
	// Конфигурация торрент-клиента
	cfg := torrent.NewDefaultClientConfig()

	// Всегда используем in-memory хранилище
	memStorage := storage.NewMemoryStorage(opts.MemoryStorageCapacity)
	cfg.DefaultStorage = memStorage
	logger.Info("Torrent: используется in-memory хранилище", "capacity", opts.MemoryStorageCapacity)

	cfg.NoUpload = false
	cfg.Seed = true

	// Применяем опции для тестирования
	if opts.NoDHT {
		cfg.NoDHT = true
	}
	if opts.DisableUTP {
		cfg.DisableUTP = true
	}
	if opts.DisableTCP {
		cfg.DisableTCP = true
	}
	if opts.ListenPort != 0 {
		cfg.ListenPort = opts.ListenPort
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		logger.Error("Torrent: не удалось создать торрент-клиент", "error", err)
		return nil, fmt.Errorf("не удалось создать торрент-клиент: %w", err)
	}

	logger.Info("Torrent: сервис инициализирован")

	return &Service{
		client:        client,
		torrents:      make(map[string]*torrent.Torrent),
		selectedFiles: make(map[string]int),
		bufferService: bufferService,
	}, nil
}

// AddMagnet добавляет торрент по magnet-ссылке.
// Параметр ctx - контекст для отмены операции.
// Параметр magnetURI - magnet-ссылка на торрент.
// Ожидает получения метаданных (таймаут через контекст).
// Возвращает информацию о торренте или ошибку.
func (s *Service) AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error) {
	// Валидация magnet URI
	if err := validation.ValidateMagnetURI(magnetURI); err != nil {
		logger.Warn("Torrent: невалидный magnet URI", "error", err)
		return nil, fmt.Errorf("невалидный magnet URI: %w", err)
	}

	// Логируем только хеш для безопасности (не полную magnet-ссылку)
	hash := sha256.Sum256([]byte(magnetURI))
	magnetHash := fmt.Sprintf("%x", hash[:8])

	logger.Info("Torrent: добавление торрента", "magnetHash", magnetHash)

	t, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		logger.Error("Torrent: не удалось добавить торрент", "magnetHash", magnetHash, "error", err)
		return nil, fmt.Errorf("не удалось добавить торрент: %w", err)
	}

	// Ожидаем получения метаданных
	select {
	case <-t.GotInfo():
		// Метаданные получены
		logger.Debug("Torrent: метаданные получены", "magnetHash", magnetHash)
	case <-ctx.Done():
		t.Drop()
		logger.Warn("Torrent: таймаут получения метаданных", "magnetHash", magnetHash, "error", ctx.Err())
		return nil, fmt.Errorf("таймаут получения метаданных: %w", ctx.Err())
	}

	torrentID := t.InfoHash().HexString()

	s.mu.Lock()
	if len(s.torrents) >= maxTorrents {
		s.mu.Unlock()
		t.Drop()
		logger.Warn("Torrent: превышен лимит торрентов", "torrentID", torrentID, "max", maxTorrents)
		return nil, errors.New(errors.ErrUnavailable, "превышен максимальный количество торрентов")
	}
	s.torrents[torrentID] = t
	s.mu.Unlock()

	info := s.torrentToInfo(t)

	// Валидация названия торрента
	if err := validation.ValidateTorrentName(info.Name); err != nil {
		logger.Warn("Torrent: некорректное название торрента", "torrentID", torrentID, "error", err)
		// Не прерываем операцию, но логируем
	}

	// Валидация размера
	if err := validation.ValidateFileSize(info.Size); err != nil {
		logger.Warn("Torrent: некорректный размер торрента", "torrentID", torrentID, "size", info.Size, "error", err)
	}

	logger.Info("Torrent: торрент добавлен",
		"torrentID", torrentID,
		"name", info.Name,
		"size", info.Size,
		"files", len(t.Files()),
	)

	return info, nil
}

// RemoveTorrent удаляет торрент по ID.
// Останавливает загрузку и удаляет торрент из клиента.
// Возвращает ошибку если торрент не найден.
func (s *Service) RemoveTorrent(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.torrents[id]
	if !exists {
		logger.Warn("Torrent: торрент не найден для удаления", "torrentID", id)
		return errors.NotFound("торрент", id)
	}

	torrentName := ""
	if t.Info() != nil {
		torrentName = t.Info().BestName()
	}

	t.Drop()
	delete(s.torrents, id)
	delete(s.selectedFiles, id)

	// Удаляем из сервиса буферизации
	if s.bufferService != nil {
		s.bufferService.UnregisterTorrent(id)
	}

	logger.Info("Torrent: торрент удалён", "torrentID", id, "name", torrentName)
	return nil
}

// ListTorrents возвращает список всех торрентов.
// Возвращает массив с информацией о каждом торренте (ID, имя, прогресс, статус).
func (s *Service) ListTorrents() []*models.TorrentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.TorrentInfo, 0, len(s.torrents))
	for _, t := range s.torrents {
		result = append(result, s.torrentToInfo(t))
	}

	logger.Debug("Torrent: получен список торрентов", "count", len(result))
	return result
}

// GetFiles возвращает список файлов торрента.
// Параметр torrentID - идентификатор торрента.
// Возвращает массив файлов с индексами, именами, размерами и путями.
// Возвращает ошибку если торрент не найден или метаданные не получены.
func (s *Service) GetFiles(torrentID string) ([]models.FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: торрент не найден для получения файлов", "torrentID", torrentID)
		return nil, errors.NotFound("торрент", torrentID)
	}

	if t.Info() == nil {
		logger.Warn("Torrent: метаданные торрента ещё не получены", "torrentID", torrentID)
		return nil, errors.New(errors.ErrUnavailable, "метаданные торрента ещё не получены")
	}

	files := t.Files()
	result := make([]models.FileInfo, 0, len(files))

	for i, f := range files {
		// Валидация размера файла
		if err := validation.ValidateFileSize(f.Length()); err != nil {
			logger.Warn("Torrent: некорректный размер файла",
				"torrentID", torrentID,
				"fileIndex", i,
				"error", err,
			)
		}

		result = append(result, models.FileInfo{
			Index: i,
			Name:  f.DisplayPath(),
			Size:  f.Length(),
		})
	}

	logger.Debug("Torrent: получен список файлов", "torrentID", torrentID, "fileCount", len(result))
	return result, nil
}

// SelectFile выбирает файл для стриминга.
// Параметр torrentID - идентификатор торрента.
// Параметр fileIndex - индекс файла в торренте.
// Устанавливает приоритет загрузки для выбранного файла.
// Возвращает ошибку если торрент не найден или индекс неверный.
func (s *Service) SelectFile(torrentID string, fileIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: торрент не найден для выбора файла", "torrentID", torrentID, "fileIndex", fileIndex)
		return errors.NotFound("торрент", torrentID)
	}

	files := t.Files()

	// Валидация индекса файла
	if err := validation.ValidateFileIndex(fileIndex, len(files)-1); err != nil {
		logger.Warn("Torrent: неверный индекс файла",
			"torrentID", torrentID,
			"fileIndex", fileIndex,
			"totalFiles", len(files),
			"error", err,
		)
		return errors.Wrap(errors.ErrInvalidInput, "неверный индекс файла", err)
	}

	// Отменяем загрузку всех файлов
	for _, f := range files {
		f.SetPriority(torrent.PiecePriorityNone)
	}

	// Выбираем нужный файл
	files[fileIndex].SetPriority(torrent.PiecePriorityNormal)
	s.selectedFiles[torrentID] = fileIndex

	// Регистрируем в сервисе буферизации
	if s.bufferService != nil {
		s.bufferService.RegisterTorrent(
			torrentID,
			files[fileIndex],
			constants.DefaultBufferPercent,
			constants.DefaultBufferDuration,
			constants.DefaultMaxBufferSize,
		)
	}

	logger.Info("Torrent: выбран файл для стриминга",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", files[fileIndex].DisplayPath(),
	)
	return nil
}

// UpdateBufferPosition обновляет текущую позицию воспроизведения для буферизации.
// Параметр torrentID - идентификатор торрента.
// Параметр position - позиция в байтах.
func (s *Service) UpdateBufferPosition(torrentID string, position int64) {
	if s.bufferService != nil {
		s.bufferService.UpdatePosition(torrentID, position)
	}
}

// GetBufferInfo возвращает информацию о состоянии буфера.
// Параметр torrentID - идентификатор торрента.
// Возвращает информацию о буфере или ошибку.
func (s *Service) GetBufferInfo(torrentID string) (*models.BufferInfo, error) {
	if s.bufferService == nil {
		return nil, errors.New(errors.ErrUnavailable, "сервис буферизации не инициализирован")
	}
	return s.bufferService.GetBufferInfo(torrentID)
}

// ServeFile обрабатывает HTTP стриминг файла торрента.
// Поддерживает Range запросы для перемотки через http.ServeContent.
// Автоматически определяет Content-Type по расширению файла.
// Возвращает 400 если файл не выбран, 404 если торрент не найден.
//
// Использует одну RLock для обеих проверок (существование торрента и выбор файла),
// чтобы предотвратить race condition когда торрент может быть удалён между двумя RLock.
func (s *Service) ServeFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	s.mu.RLock()
	t, exists := s.torrents[torrentID]
	if !exists {
		s.mu.RUnlock()
		logger.Warn("Torrent: торрент не найден для стриминга", "torrentID", torrentID)
		http.Error(w, "Торрент не найден", http.StatusNotFound)
		return
	}

	fileIndex, hasSelection := s.selectedFiles[torrentID]
	if !hasSelection {
		s.mu.RUnlock()
		logger.Warn("Torrent: файл не выбран для стриминга", "torrentID", torrentID)
		http.Error(w, "Файл не выбран для стриминга", http.StatusBadRequest)
		return
	}

	// Получаем список файлов пока блокировка активна
	files := t.Files()
	if fileIndex >= len(files) {
		s.mu.RUnlock()
		logger.Warn("Torrent: неверный индекс файла при стриминге",
			"torrentID", torrentID,
			"fileIndex", fileIndex,
			"totalFiles", len(files),
		)
		http.Error(w, "Неверный индекс файла", http.StatusBadRequest)
		return
	}

	file := files[fileIndex]

	if file.Length() > maxStreamFileSize {
		s.mu.RUnlock()
		logger.Warn("Torrent: файл превышает максимальный размер для стриминга",
			"torrentID", torrentID,
			"fileSize", file.Length(),
			"maxSize", maxStreamFileSize,
		)
		http.Error(w, "Файл слишком большой для стриминга", http.StatusBadRequest)
		return
	}

	reader := file.NewReader()
	s.mu.RUnlock()

	closer := &readCloserWithClose{ReadSeekCloser: reader}
	defer func() { _ = closer.Close() }()

	safeName := sanitizeFilename(file.DisplayPath())

	logger.Info("Torrent: начало стриминга",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", safeName,
		"fileSize", file.Length(),
	)

	http.ServeContent(w, r, safeName, time.Now(), closer)
}

// Close закрывает сервис торрентов с graceful shutdown.
// Останавливает торрент-клиент и освобождает ресурсы.
// Использует контекст с таймаутом для ожидания завершения активных загрузок.
// После вызова сервис не может быть использован.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		// Создаём контекст с таймаутом для graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		// Канал для сигнализации о завершении
		done := make(chan struct{})

		torrentCount := len(s.torrents)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Torrent: горутина close завершилась с паникой", "error", r)
				}
			}()
			s.client.Close()
			close(done)
		}()

		// Ожидаем завершения или таймаута
		select {
		case <-done:
			logger.Info("Torrent: сервис остановлен gracefully", "torrentCount", torrentCount)
		case <-ctx.Done():
			logger.Warn("Torrent: сервис остановлен с таймаутом", "torrentCount", torrentCount)
		}
	}

	return nil
}

func (s *Service) torrentToInfo(t *torrent.Torrent) *models.TorrentInfo {
	info := &models.TorrentInfo{
		ID:     t.InfoHash().HexString(),
		Status: "loading",
	}

	if t.Info() != nil {
		info.Name = t.Info().BestName()
		info.Size = t.Info().TotalLength()
		if info.Size == 0 {
			info.Progress = 0
		} else {
			info.Progress = float64(t.BytesCompleted()) / float64(info.Size)
		}

		if t.Complete().Bool() {
			info.Status = "seeding"
		} else if t.BytesCompleted() > 0 {
			info.Status = "downloading"
		}
	}

	return info
}

// sanitizeFilename очищает имя файла от потенциально опасных символов и путей.
// Предотвращает path traversal и CRLF-инъекции.
func sanitizeFilename(name string) string {
	name = path.Base(name)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}
