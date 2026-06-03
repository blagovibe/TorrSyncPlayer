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
	"os"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Константы сервиса торрентов
const (
	// gracefulShutdownTimeout таймаут для корректной остановки сервиса
	gracefulShutdownTimeout = constants.TorrentGracefulShutdownTimeout

	// dataDirPermissions права доступа к директории данных
	dataDirPermissions = constants.DataDirPermissions
)

// Service сервис управления торрентами.
// Предоставляет методы для добавления, удаления и стриминга торрентов.
// Потокобезопасен благодаря использованию sync.RWMutex.
type Service struct {
	mu            sync.RWMutex
	client        *torrent.Client
	torrents      map[string]*torrent.Torrent
	dataDir       string
	selectedFiles map[string]int // torrentID -> fileIndex
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

// NewService создаёт новый сервис торрентов.
// Параметр dataDir - директория для хранения данных торрентов.
// Создаёт директорию если не существует.
// Возвращает инициализированный сервис или ошибку.
func NewService(dataDir string) (*Service, error) {
	// Создаём директорию для данных если не существует
	if err := os.MkdirAll(dataDir, dataDirPermissions); err != nil {
		logger.Error("Torrent: не удалось создать директорию данных", "dataDir", dataDir, "error", err)
		return nil, fmt.Errorf("не удалось создать директорию данных: %w", err)
	}

	// Конфигурация торрент-клиента
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = dataDir
	cfg.NoUpload = false
	cfg.Seed = true

	client, err := torrent.NewClient(cfg)
	if err != nil {
		logger.Error("Torrent: не удалось создать торрент-клиент", "error", err)
		return nil, fmt.Errorf("не удалось создать торрент-клиент: %w", err)
	}

	logger.Info("Torrent: сервис инициализирован", "dataDir", dataDir)

	return &Service{
		client:        client,
		torrents:      make(map[string]*torrent.Torrent),
		dataDir:       dataDir,
		selectedFiles: make(map[string]int),
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
func (s *Service) RemoveTorrent(id string) error {
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

	logger.Info("Torrent: выбран файл для стриминга",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", files[fileIndex].DisplayPath(),
	)
	return nil
}

// ServeFile обрабатывает HTTP стриминг файла торрента.
// Поддерживает Range запросы для перемотки через http.ServeContent.
// Автоматически определяет Content-Type по расширению файла.
// Возвращает 400 если файл не выбран, 404 если торрент не найден.
func (s *Service) ServeFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	// Одна блокировка для обеих операций - предотвращает гонку данных
	// когда другой поток может удалить торрент между двумя RLock
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
	s.mu.RUnlock()

	files := t.Files()
	if fileIndex >= len(files) {
		logger.Warn("Torrent: неверный индекс файла при стриминге",
			"torrentID", torrentID,
			"fileIndex", fileIndex,
			"totalFiles", len(files),
		)
		http.Error(w, "Неверный индекс файла", http.StatusBadRequest)
		return
	}

	file := files[fileIndex]
	reader := file.NewReader()

	// Закрываем reader после завершения стриминга для предотвращения утечки файловых дескрипторов
	// Используем обёртку с отложенным закрытием
	closer := &readCloserWithClose{ReadSeekCloser: reader}
	defer closer.Close()

	logger.Info("Torrent: начало стриминга",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", file.DisplayPath(),
		"fileSize", file.Length(),
	)

	// Используем http.ServeContent для полной поддержки RFC 7233
	// Автоматически обрабатывает Range запросы, Content-Range, If-Range и т.д.
	http.ServeContent(w, r, file.DisplayPath(), time.Now(), closer)
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

// torrentToInfo конвертирует torrent.Torrent в TorrentInfo.
// Извлекает информацию о торренте: ID, имя, размер, прогресс, статус.
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
