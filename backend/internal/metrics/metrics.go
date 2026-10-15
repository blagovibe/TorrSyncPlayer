// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package metrics provides Prometheus metrics for server monitoring.
package metrics

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type Metrics struct {
	mu sync.RWMutex

	startTime time.Time

	requestsTotal   int64
	requestsSuccess int64
	requestsError   int64

	torrentsAdded   int64
	torrentsRemoved int64
	torrentsActive  int64

	cacheMu      sync.Mutex
	cachedOutput string
	cacheTime    time.Time
	cacheTTL     time.Duration

	memStatsMu     sync.Mutex
	cachedMemStats map[string]uint64
	memStatsTime   time.Time
}

var (
	instance *Metrics
	once     sync.Once
)

// GetInstance returns the metrics singleton
func GetInstance() *Metrics {
	once.Do(func() {
		instance = &Metrics{
			startTime: time.Now(),
			cacheTTL:  5 * time.Second,
		}
	})
	return instance
}

// RequestStarted registers the start of a request
func (m *Metrics) RequestStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsTotal++
}

// RequestSuccess registers a successful request
func (m *Metrics) RequestSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsSuccess++
}

// RequestError registers a failed request
func (m *Metrics) RequestError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsError++
}

// TorrentAdded registers a torrent addition
func (m *Metrics) TorrentAdded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.torrentsAdded++
	m.torrentsActive++
}

// TorrentRemoved registers a torrent removal
func (m *Metrics) TorrentRemoved() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.torrentsRemoved++
	if m.torrentsActive > 0 {
		m.torrentsActive--
	}
}

// GetUptime returns the server uptime in seconds
func (m *Metrics) GetUptime() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.startTime).Seconds()
}

// getUptimeUnsafe returns uptime without locking (call under RLock)
func (m *Metrics) getUptimeUnsafe() float64 {
	return time.Since(m.startTime).Seconds()
}

// getMemoryStatsUnsafe returns memory stats without locking (call under RLock)
func (m *Metrics) getMemoryStatsUnsafe() map[string]uint64 {
	m.memStatsMu.Lock()
	defer m.memStatsMu.Unlock()

	if m.cachedMemStats != nil && time.Since(m.memStatsTime) < 15*time.Second {
		return m.cachedMemStats
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	m.cachedMemStats = map[string]uint64{
		"alloc":       memStats.Alloc,
		"total_alloc": memStats.TotalAlloc,
		"sys":         memStats.Sys,
		"num_gc":      uint64(memStats.NumGC),
	}
	m.memStatsTime = time.Now()

	return m.cachedMemStats
}

// FormatPrometheus formats metrics in Prometheus format
func (m *Metrics) FormatPrometheus() string {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if time.Since(m.cacheTime) < m.cacheTTL && m.cachedOutput != "" {
		return m.cachedOutput
	}

	m.mu.RLock()
	memStats := m.getMemoryStatsUnsafe()
	result := m.formatPrometheusUnsafe(memStats)
	m.mu.RUnlock()

	m.cachedOutput = result
	m.cacheTime = time.Now()

	return result
}

func (m *Metrics) formatPrometheusUnsafe(memStats map[string]uint64) string {
	var result string

	// Uptime
	result += "# HELP torrsyncplayer_uptime_seconds Uptime in seconds\n"
	result += "# TYPE torrsyncplayer_uptime_seconds gauge\n"
	result += fmt.Sprintf("torrsyncplayer_uptime_seconds %.2f\n", m.getUptimeUnsafe())

	// Requests
	result += "# HELP torrsyncplayer_requests_total Total requests\n"
	result += "# TYPE torrsyncplayer_requests_total counter\n"
	result += fmt.Sprintf("torrsyncplayer_requests_total{status=\"success\"} %d\n", m.requestsSuccess)
	result += fmt.Sprintf("torrsyncplayer_requests_total{status=\"error\"} %d\n", m.requestsError)

	// Torrents
	result += "# HELP torrsyncplayer_torrents_active Active torrents\n"
	result += "# TYPE torrsyncplayer_torrents_active gauge\n"
	result += fmt.Sprintf("torrsyncplayer_torrents_active %d\n", m.torrentsActive)

	result += "# HELP torrsyncplayer_torrents_added_total Total added torrents\n"
	result += "# TYPE torrsyncplayer_torrents_added_total counter\n"
	result += fmt.Sprintf("torrsyncplayer_torrents_added_total %d\n", m.torrentsAdded)

	// Memory
	result += "# HELP torrsyncplayer_memory_alloc_bytes Allocated memory\n"
	result += "# TYPE torrsyncplayer_memory_alloc_bytes gauge\n"
	result += fmt.Sprintf("torrsyncplayer_memory_alloc_bytes %d\n", memStats["alloc"])

	result += "# HELP torrsyncplayer_memory_sys_bytes System memory\n"
	result += "# TYPE torrsyncplayer_memory_sys_bytes gauge\n"
	result += fmt.Sprintf("torrsyncplayer_memory_sys_bytes %d\n", memStats["sys"])

	result += "# HELP torrsyncplayer_gc_total Total GC cycles\n"
	result += "# TYPE torrsyncplayer_gc_total counter\n"
	result += fmt.Sprintf("torrsyncplayer_gc_total %d\n", memStats["num_gc"])

	return result
}
