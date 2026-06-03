// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package metrics предоставляет метрики Prometheus для мониторинга сервера.
package metrics

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// Metrics структура для хранения метрик сервера
type Metrics struct {
	mu sync.RWMutex

	// Время запуска сервера
	startTime time.Time

	// Счётчики запросов
	requestsTotal   int64
	requestsSuccess int64
	requestsError   int64

	// Метрики торрентов
	torrentsAdded   int64
	torrentsRemoved int64
	torrentsActive  int64

	// Метрики комнат
	roomsCreated int64
	roomsActive  int64
	peersTotal   int64

	// Метрики синхронизации
	syncOperations int64
}

var (
	instance *Metrics
	once     sync.Once
)

// GetInstance возвращает синглтон метрик
func GetInstance() *Metrics {
	once.Do(func() {
		instance = &Metrics{
			startTime: time.Now(),
		}
	})
	return instance
}

// RequestStarted регистрирует начало обработки запроса
func (m *Metrics) RequestStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsTotal++
}

// RequestSuccess регистрирует успешный запрос
func (m *Metrics) RequestSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsSuccess++
}

// RequestError регистрирует ошибочный запрос
func (m *Metrics) RequestError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsError++
}

// TorrentAdded регистрирует добавление торрента
func (m *Metrics) TorrentAdded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.torrentsAdded++
	m.torrentsActive++
}

// TorrentRemoved регистрирует удаление торрента
func (m *Metrics) TorrentRemoved() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.torrentsRemoved++
	if m.torrentsActive > 0 {
		m.torrentsActive--
	}
}

// RoomCreated регистрирует создание комнаты
func (m *Metrics) RoomCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roomsCreated++
	m.roomsActive++
}

// RoomClosed регистрирует закрытие комнаты
func (m *Metrics) RoomClosed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.roomsActive > 0 {
		m.roomsActive--
	}
}

// PeerJoined регистрирует подключение пира
func (m *Metrics) PeerJoined() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peersTotal++
}

// PeerLeft регистрирует отключение пира
func (m *Metrics) PeerLeft() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.peersTotal > 0 {
		m.peersTotal--
	}
}

// SyncOperation регистрирует операцию синхронизации
func (m *Metrics) SyncOperation() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncOperations++
}

// GetUptime возвращает время работы сервера в секундах
func (m *Metrics) GetUptime() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.startTime).Seconds()
}

// GetMemoryStats возвращает статистику использования памяти
func (m *Metrics) GetMemoryStats() map[string]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]uint64{
		"alloc":       memStats.Alloc,
		"total_alloc": memStats.TotalAlloc,
		"sys":         memStats.Sys,
		"num_gc":      uint64(memStats.NumGC),
	}
}

// GetSnapshot возвращает снимок всех метрик
func (m *Metrics) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memStats := m.getMemoryStatsUnsafe()

	return map[string]interface{}{
		"uptime_seconds": m.getUptimeUnsafe(),
		"requests": map[string]int64{
			"total":   m.requestsTotal,
			"success": m.requestsSuccess,
			"error":   m.requestsError,
		},
		"torrents": map[string]int64{
			"added":   m.torrentsAdded,
			"removed": m.torrentsRemoved,
			"active":  m.torrentsActive,
		},
		"rooms": map[string]int64{
			"created": m.roomsCreated,
			"active":  m.roomsActive,
		},
		"peers": map[string]int64{
			"total": m.peersTotal,
		},
		"sync": map[string]int64{
			"operations": m.syncOperations,
		},
		"memory": memStats,
	}
}

// getUptimeUnsafe возвращает uptime без блокировки (вызывать под RLock)
func (m *Metrics) getUptimeUnsafe() float64 {
	return time.Since(m.startTime).Seconds()
}

// getMemoryStatsUnsafe возвращает статистику памяти без блокировки (вызывать под RLock)
func (m *Metrics) getMemoryStatsUnsafe() map[string]uint64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]uint64{
		"alloc":       memStats.Alloc,
		"total_alloc": memStats.TotalAlloc,
		"sys":         memStats.Sys,
		"num_gc":      uint64(memStats.NumGC),
	}
}

// FormatPrometheus форматирует метрики в формате Prometheus
func (m *Metrics) FormatPrometheus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memStats := m.getMemoryStatsUnsafe()

	var result string

	// Время работы
	result += "# HELP torrserver_uptime_seconds Uptime in seconds\n"
	result += "# TYPE torrserver_uptime_seconds gauge\n"
	result += fmt.Sprintf("torrserver_uptime_seconds %.2f\n", m.getUptimeUnsafe())

	// Запросы
	result += "# HELP torrserver_requests_total Total requests\n"
	result += "# TYPE torrserver_requests_total counter\n"
	result += fmt.Sprintf("torrserver_requests_total{status=\"success\"} %d\n", m.requestsSuccess)
	result += fmt.Sprintf("torrserver_requests_total{status=\"error\"} %d\n", m.requestsError)

	// Торренты
	result += "# HELP torrserver_torrents_active Active torrents\n"
	result += "# TYPE torrserver_torrents_active gauge\n"
	result += fmt.Sprintf("torrserver_torrents_active %d\n", m.torrentsActive)

	result += "# HELP torrserver_torrents_added_total Total added torrents\n"
	result += "# TYPE torrserver_torrents_added_total counter\n"
	result += fmt.Sprintf("torrserver_torrents_added_total %d\n", m.torrentsAdded)

	// Комнаты
	result += "# HELP torrserver_rooms_active Active rooms\n"
	result += "# TYPE torrserver_rooms_active gauge\n"
	result += fmt.Sprintf("torrserver_rooms_active %d\n", m.roomsActive)

	result += "# HELP torrserver_rooms_created_total Total created rooms\n"
	result += "# TYPE torrserver_rooms_created_total counter\n"
	result += fmt.Sprintf("torrserver_rooms_created_total %d\n", m.roomsCreated)

	// Пиры
	result += "# HELP torrserver_peers_total Total peers\n"
	result += "# TYPE torrserver_peers_total gauge\n"
	result += fmt.Sprintf("torrserver_peers_total %d\n", m.peersTotal)

	// Синхронизация
	result += "# HELP torrserver_sync_operations_total Total sync operations\n"
	result += "# TYPE torrserver_sync_operations_total counter\n"
	result += fmt.Sprintf("torrserver_sync_operations_total %d\n", m.syncOperations)

	// Память
	result += "# HELP torrserver_memory_alloc_bytes Allocated memory\n"
	result += "# TYPE torrserver_memory_alloc_bytes gauge\n"
	result += fmt.Sprintf("torrserver_memory_alloc_bytes %d\n", memStats["alloc"])

	result += "# HELP torrserver_memory_sys_bytes System memory\n"
	result += "# TYPE torrserver_memory_sys_bytes gauge\n"
	result += fmt.Sprintf("torrserver_memory_sys_bytes %d\n", memStats["sys"])

	result += "# HELP torrserver_gc_total Total GC cycles\n"
	result += "# TYPE torrserver_gc_total counter\n"
	result += fmt.Sprintf("torrserver_gc_total %d\n", memStats["num_gc"])

	return result
}
