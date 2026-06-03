// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
)

// Глобальный сервис для тестов — создаётся один раз через TestMain
var testService *Service

// TestMain создаёт общий торрент-сервис для всех тестов
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "torrent-test-*")
	if err != nil {
		panic(err)
	}

	svc, err := NewService(tmpDir)
	if err != nil {
		panic(err)
	}
	testService = svc

	code := m.Run()

	svc.Close()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// TestNewService проверяет инициализацию торрент-сервиса
func TestNewService(t *testing.T) {
	t.Parallel()
	require.NotNil(t, testService)
	assert.NotNil(t, testService.client)
	assert.NotNil(t, testService.torrents)
	assert.NotEmpty(t, testService.dataDir)
}

// TestAddMagnet_EmptyURI проверяет валидацию пустой magnet-ссылки
func TestAddMagnet_EmptyURI(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := testService.AddMagnet(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не может быть пустой")
}

// TestAddMagnet_Timeout проверяет обработку невалидной magnet-ссылки
// с таймаутом (реальный торрент-клиент не сможет получить метаданные)
func TestAddMagnet_Timeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	magnetURI := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
	_, err := testService.AddMagnet(ctx, magnetURI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "таймаут")
}

// TestListTorrents_Empty проверяет получение пустого списка торрентов
func TestListTorrents_Empty(t *testing.T) {
	t.Parallel()
	torrents := testService.ListTorrents()
	assert.NotNil(t, torrents)
}

// TestRemoveTorrent_NotFound проверяет удаление несуществующего торрента
func TestRemoveTorrent_NotFound(t *testing.T) {
	t.Parallel()
	err := testService.RemoveTorrent("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestGetFiles_NotFound проверяет получение файлов несуществующего торрента
func TestGetFiles_NotFound(t *testing.T) {
	t.Parallel()
	_, err := testService.GetFiles("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestSelectFile_InvalidIndex проверяет выбор файла в несуществующем торренте
func TestSelectFile_InvalidIndex(t *testing.T) {
	t.Parallel()
	err := testService.SelectFile("nonexistent", 0)
	assert.Error(t, err)
}

// TestClose проверяет корректное закрытие сервиса
func TestClose(t *testing.T) {
	t.Parallel()
	// Используем глобальный сервис — просто проверяем что Close не паникует
	err := testService.Close()
	assert.NoError(t, err)
}

// TestClose_MultipleCalls проверяет что повторный Close не вызывает панику
func TestClose_MultipleCalls(t *testing.T) {
	t.Parallel()
	// Первый Close уже был в TestClose, проверяем что повторный не паникует
	err := testService.Close()
	assert.NoError(t, err)
}

// TestValidatePosition проверяет валидацию позиции воспроизведения
func TestValidatePosition(t *testing.T) {
	t.Parallel()
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
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	done := make(chan struct{})

	go func() {
		defer close(done)

		// Проверяем ListTorrents
		torrents := testService.ListTorrents()
		assert.NotNil(t, torrents)

		// Проверяем RemoveTorrent с несуществующим ID
		err := testService.RemoveTorrent("nonexistent-id-for-timeout-test")
		assert.Error(t, err)
	}()

	select {
	case <-done:
		// Тест завершился успешно
	case <-time.After(10 * time.Second):
		t.Fatal("тест завис — операции не завершились за 10 секунд")
	}
}
