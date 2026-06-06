// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Инициализируем логгер с выводом в stderr для тестов
	// Это предотвращает блокировку на stdout в Windows тестах
	logger.Init("error", "json")
}

// createTestService создаёт торрент-сервис для тестов с отключённой сетью
func createTestService(t *testing.T) *Service {
	t.Helper()

	bufferSvc := buffer.NewService(64 * 1024 * 1024)

	// Используем ListenPort: 0 для динамического выбора свободного порта
	// Это предотвращает конфликты портов при параллельном запуске тестов
	svc, err := NewServiceWithOptions(bufferSvc, ServiceOptions{
		NoDHT:      true,
		DisableUTP: true,
		DisableTCP: true,
		ListenPort: 0,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		func() { _ = svc.Close() }()
	})

	return svc
}

// TestNewService проверяет инициализацию торрент-сервиса
func TestNewService(t *testing.T) {
	svc := createTestService(t)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.client)
	assert.NotNil(t, svc.torrents)
}

// TestAddMagnet_EmptyURI проверяет валидацию пустой magnet-ссылки
func TestAddMagnet_EmptyURI(t *testing.T) {
	svc := createTestService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.AddMagnet(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не может быть пустой")
}

// TestAddMagnet_InvalidURI проверяет валидацию невалидной magnet-ссылки
func TestAddMagnet_InvalidURI(t *testing.T) {
	svc := createTestService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.AddMagnet(ctx, "not-a-magnet-link")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный magnet URI")
}

// TestListTorrents_Empty проверяет получение пустого списка торрентов
func TestListTorrents_Empty(t *testing.T) {
	svc := createTestService(t)
	torrents := svc.ListTorrents()
	assert.NotNil(t, torrents)
}

// TestRemoveTorrent_NotFound проверяет удаление несуществующего торрента
func TestRemoveTorrent_NotFound(t *testing.T) {
	svc := createTestService(t)
	err := svc.RemoveTorrent(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestGetFiles_NotFound проверяет получение файлов несуществующего торрента
func TestGetFiles_NotFound(t *testing.T) {
	svc := createTestService(t)
	_, err := svc.GetFiles("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestSelectFile_InvalidIndex проверяет выбор файла в несуществующем торренте
func TestSelectFile_InvalidIndex(t *testing.T) {
	svc := createTestService(t)
	err := svc.SelectFile("nonexistent", 0)
	assert.Error(t, err)
}

// TestClose проверяет корректное закрытие сервиса
func TestClose(t *testing.T) {
	svc := createTestService(t)
	err := svc.Close()
	assert.NoError(t, err)
}

// TestClose_MultipleCalls проверяет что повторный Close не вызывает панику
func TestClose_MultipleCalls(t *testing.T) {
	svc := createTestService(t)
	err := svc.Close()
	assert.NoError(t, err)
	// Повторный Close
	err = svc.Close()
	assert.NoError(t, err)
}

// TestValidatePosition проверяет валидацию позиции воспроизведения
func TestValidatePosition(t *testing.T) {
	tests := []struct {
		name    string
		pos     float64
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 100.5, false},
		{"valid max", 86400, false},
		{"negative", -1, true},
		{"too large", 86401, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidatePosition(tt.pos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTorrentID проверяет валидацию ID торрента
func TestValidateTorrentID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex id", "0123456789abcdef0123456789abcdef01234567", false},
		{"empty id", "", true},
		{"too short", "abc123", true},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"invalid chars", "xyz123456789abcdef0123456789abcdef0123456", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateTorrentID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestServiceWithTimeout проверяет что все операции завершаются за разумное время
func TestServiceWithTimeout(t *testing.T) {
	svc := createTestService(t)
	done := make(chan struct{})

	go func() {
		defer close(done)

		// Проверяем ListTorrents
		torrents := svc.ListTorrents()
		assert.NotNil(t, torrents)

		// Проверяем RemoveTorrent с несуществующим ID
		err := svc.RemoveTorrent(context.Background(), "nonexistent-id-for-timeout-test")
		assert.Error(t, err)
	}()

	select {
	case <-done:
		// Тест завершился успешно
	case <-time.After(10 * time.Second):
		t.Fatal("тест завис — операции не завершились за 10 секунд")
	}
}

// Подавляем предупреждение о неиспользуемом импорте
var _ = slog.NewJSONHandler
