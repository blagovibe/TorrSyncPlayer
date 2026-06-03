// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMetrics_ConcurrentAccess проверяет потокобезопасность Metrics
// при одновременном вызове из нескольких горутин
func TestMetrics_ConcurrentAccess(t *testing.T) {
	// Создаём новый экземпляр для теста (не синглтон)
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Запускаем горутины для записи
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				m.RequestStarted()
				m.RequestSuccess()
				m.TorrentAdded()
				m.RoomCreated()
				m.PeerJoined()
				m.SyncOperation()
			}
		}()
	}

	// Запускаем горутины для чтения (проверка гонки данных)
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = m.GetUptime()
				_ = m.GetMemoryStats()
				_ = m.GetSnapshot()
				_ = m.FormatPrometheus()
			}
		}()
	}

	wg.Wait()

	// Проверяем что данные корректны
	assert.Equal(t, int64(numGoroutines*numOperations), m.requestsTotal)
	assert.Equal(t, int64(numGoroutines*numOperations), m.requestsSuccess)
	assert.Equal(t, int64(numGoroutines*numOperations), m.torrentsAdded)
	assert.Equal(t, int64(numGoroutines*numOperations), m.roomsCreated)
	assert.Equal(t, int64(numGoroutines*numOperations), m.peersTotal)
	assert.Equal(t, int64(numGoroutines*numOperations), m.syncOperations)
}

// TestMetrics_GetUptime_Race проверяет отсутствие гонки в GetUptime
func TestMetrics_GetUptime_Race(t *testing.T) {
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Горутина 1: читаем uptime
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = m.GetUptime()
		}
	}()

	// Горутина 2: читаем snapshot (который тоже читает uptime)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = m.GetSnapshot()
		}
	}()

	wg.Wait()
}

// TestMetrics_GetMemoryStats_Race проверяет отсутствие гонки в GetMemoryStats
func TestMetrics_GetMemoryStats_Race(t *testing.T) {
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Горутина 1: читаем memory stats
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = m.GetMemoryStats()
		}
	}()

	// Горутина 2: пишем метрики
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			m.RequestStarted()
			m.RequestSuccess()
		}
	}()

	wg.Wait()
}

// TestMetrics_ConcurrentTorrentOperations проверяет потокобезопасность
// при одновременном добавлении и удалении торрентов
func TestMetrics_ConcurrentTorrentOperations(t *testing.T) {
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	// Добавляем торренты
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.TorrentAdded()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(numGoroutines), m.torrentsActive)

	// Удаляем торренты
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.TorrentRemoved()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(0), m.torrentsActive)
}

// TestMetrics_ConcurrentRoomOperations проверяет потокобезопасность
// при одновременном создании и закрытии комнат
func TestMetrics_ConcurrentRoomOperations(t *testing.T) {
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	// Создаём комнаты
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.RoomCreated()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(numGoroutines), m.roomsActive)

	// Закрываем комнаты
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.RoomClosed()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(0), m.roomsActive)
}

// TestMetrics_ConcurrentPeerOperations проверяет потокобезопасность
// при одновременном подключении и отключении пиров
func TestMetrics_ConcurrentPeerOperations(t *testing.T) {
	m := &Metrics{
		startTime: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	// Подключаем пиров
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.PeerJoined()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(numGoroutines), m.peersTotal)

	// Отключаем пиров
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.PeerLeft()
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(0), m.peersTotal)
}
