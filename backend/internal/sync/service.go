// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package sync предоставляет сервис синхронизации воспроизведения.
// Обеспечивает синхронизацию позиции воспроизведения между пирами с компенсацией задержки сети.
// Использует структурированное логирование с контекстом операций.
package sync

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

type Service struct {
	mu        sync.RWMutex
	status    models.SyncStatus
	isClosed  bool
	closeOnce sync.Once
}

// NewService создаёт новый сервис синхронизации.
// Инициализирует начальное состояние: воспроизведение остановлено, позиция 0.
// Возвращает инициализированный сервис.
func NewService() *Service {
	svc := &Service{
		status: models.SyncStatus{
			IsPlaying: false,
			Position:  0,
			Duration:  0,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	logger.Info("Sync: сервис инициализирован")
	return svc
}

// Play запускает воспроизведение.
// Устанавливает флаг IsPlaying = true и обновляет таймстамп.
// Возвращает текущий статус синхронизации.
func (s *Service) Play() models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка воспроизведения на закрытом сервисе")
		return s.status
	}

	s.status.IsPlaying = true
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: воспроизведение запущено", "position", s.status.Position)
	return s.status
}

// Pause приостанавливает воспроизведение.
// Устанавливает флаг IsPlaying = false и обновляет таймстамп.
// Возвращает текущий статус синхронизации.
func (s *Service) Pause() models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка паузы на закрытом сервисе")
		return s.status
	}

	s.status.IsPlaying = false
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: воспроизведение приостановлено", "position", s.status.Position)
	return s.status
}

// Seek выполняет перемотку на указанную позицию.
// Параметр position - позиция в секундах (0 - 86400).
// Валидирует позицию перед применением.
// Возвращает обновлённый статус или ошибку если позиция некорректна.
func (s *Service) Seek(position float64) (models.SyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка перемотки на закрытом сервисе")
		return s.status, errors.New(errors.ErrUnavailable, "сервис закрыт")
	}

	// Валидация позиции
	if err := validation.ValidatePosition(position); err != nil {
		logger.Warn("Sync: невалидная позиция для перемотки", "position", position, "error", err)
		return s.status, errors.Wrap(errors.ErrInvalidInput, "невалидная позиция", err)
	}

	oldPosition := s.status.Position
	s.status.Position = position
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: перемотка", "oldPosition", oldPosition, "newPosition", position)
	return s.status, nil
}

// GetStatus возвращает текущий статус воспроизведения.
// Включает позицию, длительность, состояние воспроизведения и таймстамп.
func (s *Service) GetStatus() models.SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// SetDuration устанавливает длительность медиафайла.
// Параметр duration - длительность в секундах (должна быть положительной).
// Возвращает ошибку если значение некорректно.
func (s *Service) SetDuration(duration float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка установки длительности на закрытом сервисе")
		return errors.New(errors.ErrUnavailable, "сервис закрыт")
	}

	if duration < 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		logger.Warn("Sync: некорректная длительность", "duration", duration)
		return errors.InvalidInput(fmt.Sprintf("некорректная длительность: %f", duration))
	}

	s.status.Duration = duration
	logger.Info("Sync: установлена длительность", "duration", duration)
	return nil
}

// SyncWithLatency синхронизирует воспроизведение с учётом задержки сети.
// Параметр peerStatus - статус удалённого пира.
// Параметр latencyMs - задержка сети в миллисекундах.
// Использует плавную подстройку позиции для избежания резких скачков.
// Возвращает обновлённый локальный статус.
func (s *Service) SyncWithLatency(peerStatus models.SyncStatus, latencyMs int) models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка синхронизации на закрытом сервисе")
		return s.status
	}

	// Компенсация задержки
	latencySeconds := float64(latencyMs) / constants.MsPerSecond

	// Рассчитываем ожидаемую позицию пира с учётом задержки
	expectedPosition := peerStatus.Position
	if peerStatus.IsPlaying {
		elapsed := float64(time.Now().UnixMilli()-peerStatus.Timestamp) / constants.MsPerSecond
		expectedPosition = peerStatus.Position + elapsed - latencySeconds
	}

	// Валидация рассчитанной позиции
	if err := validation.ValidatePosition(expectedPosition); err != nil {
		logger.Warn("Sync: получена некорректная позиция от пира",
			"peerPosition", peerStatus.Position,
			"expectedPosition", expectedPosition,
			"error", err,
		)
		return s.status
	}

	// Плавная подстройка позиции (не резкий скачок)
	positionDiff := expectedPosition - s.status.Position

	if math.Abs(positionDiff) > constants.MaxPositionJump {
		// Плавная подстройка
		s.status.Position += positionDiff * constants.SmoothAdjustmentRatio
	} else {
		// Небольшое расхождение - подстраиваемся полностью
		s.status.Position = expectedPosition
	}

	s.status.Timestamp = time.Now().UnixMilli()

	// Синхронизируем состояние воспроизведения
	if s.status.IsPlaying != peerStatus.IsPlaying {
		s.status.IsPlaying = peerStatus.IsPlaying
	}

	logger.Debug("Sync: синхронизация выполнена",
		"localPosition", s.status.Position,
		"peerPosition", peerStatus.Position,
		"expectedPosition", expectedPosition,
		"latencyMs", latencyMs,
		"positionDiff", positionDiff,
	)

	return s.status
}

// UpdatePosition обновляет текущую позицию воспроизведения.
// Вызывается локальным плеером при изменении позиции.
// Параметр position - новая позиция в секундах.
// Возвращает ошибку если позиция некорректна или сервис закрыт.
func (s *Service) UpdatePosition(position float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: попытка обновления позиции на закрытом сервисе")
		return fmt.Errorf("сервис закрыт")
	}

	if err := validation.ValidatePosition(position); err != nil {
		logger.Warn("Sync: невалидная позиция для обновления", "position", position, "error", err)
		return err
	}

	s.status.Position = position
	s.status.Timestamp = time.Now().UnixMilli()

	return nil
}

// Close закрывает сервис синхронизации.
// Останавливает воспроизведение и помечает сервис как закрытый.
// Безопасен для многократного вызова (использует sync.Once).
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.isClosed = true
		s.status.IsPlaying = false

		logger.Info("Sync: сервис остановлен")
	})
}
