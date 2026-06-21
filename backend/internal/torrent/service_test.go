// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

func init() {
	// Initialize logger with stderr output for tests
	// This prevents blocking on stdout in Windows tests
	logger.Init("error", "json")
}

// createTestService creates a torrent service for tests with networking disabled
func createTestService(t *testing.T) *Service {
	t.Helper()

	bufferSvc := buffer.NewService(64 * 1024 * 1024)

	// Use ListenPort: 0 for dynamic free port selection
	// This prevents port conflicts during parallel test execution
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

// TestNewService tests torrent service initialization
func TestNewService(t *testing.T) {
	svc := createTestService(t)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.client)
	assert.NotNil(t, svc.torrents)
}

// TestAddMagnet_EmptyURI tests validation of empty magnet link
func TestAddMagnet_EmptyURI(t *testing.T) {
	svc := createTestService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.AddMagnet(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

// TestAddMagnet_InvalidURI tests validation of invalid magnet link
func TestAddMagnet_InvalidURI(t *testing.T) {
	svc := createTestService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.AddMagnet(ctx, "not-a-magnet-link")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid magnet URI")
}

// TestListTorrents_Empty tests getting an empty torrent list
func TestListTorrents_Empty(t *testing.T) {
	svc := createTestService(t)
	torrents := svc.ListTorrents(context.Background())
	assert.NotNil(t, torrents)
}

// TestRemoveTorrent_NotFound tests removing a non-existent torrent
func TestRemoveTorrent_NotFound(t *testing.T) {
	svc := createTestService(t)
	err := svc.RemoveTorrent(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestGetFiles_NotFound tests getting files of a non-existent torrent
func TestGetFiles_NotFound(t *testing.T) {
	svc := createTestService(t)
	_, err := svc.GetFiles(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSanitizeFilename tests filename sanitization
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/etc/passwd", "passwd"},
		{"../../../etc/hosts", "hosts"},
		{"normal.mp4", "normal.mp4"},
		{"file\r\n.mp4", "file.mp4"},
		{"file\x00name.mp4", "filename.mp4"},
		{"", "file"},
		{".", "file"},
		{"..", "file"},
		{"path/to/file.mkv", "file.mkv"},
	}
	for _, tt := range tests {
		result := sanitizeFilename(tt.input)
		assert.Equal(t, tt.expected, result, "sanitizeFilename(%q)", tt.input)
	}
}

// TestService_CreateWithNilBuffer tests that service can be created with nil buffer
func TestService_CreateWithNilBuffer(t *testing.T) {
	svc, err := NewService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Nil(t, svc.bufferService)
	svc.Close()
}

// TestMaxTorrentsConstant tests that the maxTorrents constant is within expected range
func TestMaxTorrentsConstant(t *testing.T) {
	assert.Greater(t, maxTorrents, 0)
	assert.LessOrEqual(t, maxTorrents, 1000)
}

// TestSelectFile_InvalidIndex tests selecting a file with invalid index
func TestSelectFile_InvalidIndex(t *testing.T) {
	svc := createTestService(t)
	err := svc.SelectFile(context.Background(), "nonexistent", -1)
	assert.Error(t, err)
}

// TestBufferInfo_NilBuffer tests GetBufferInfo when buffer service is nil
func TestBufferInfo_NilBuffer(t *testing.T) {
	svc, err := NewServiceWithOptions(nil, ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: true, ListenPort: 0})
	require.NoError(t, err)
	defer svc.Close()

	_, err = svc.GetBufferInfo(context.Background(), "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// TestUpdateBufferPosition_NilBuffer tests UpdateBufferPosition when buffer service is nil (should not panic)
func TestUpdateBufferPosition_NilBuffer(t *testing.T) {
	svc, err := NewServiceWithOptions(nil, ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: true, ListenPort: 0})
	require.NoError(t, err)
	defer svc.Close()

	// Should not panic
	svc.UpdateBufferPosition(context.Background(), "test", 1000)
}

// TestClose_MultipleCalls tests that Close can be called multiple times
func TestClose_MultipleCalls(t *testing.T) {
	svc, err := NewServiceWithOptions(nil, ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: true, ListenPort: 0})
	require.NoError(t, err)

	err = svc.Close()
	assert.NoError(t, err)

	err = svc.Close()
	assert.NoError(t, err)
}

// TestNewService_WithCustomOptions tests service creation with various options
func TestNewService_WithCustomOptions(t *testing.T) {
	tests := []struct {
		name string
		opts ServiceOptions
	}{
		{"all networking disabled", ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: true, ListenPort: 0}},
		{"memory capacity", ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: true, ListenPort: 0, MemoryStorageCapacity: 1024 * 1024}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewServiceWithOptions(nil, tt.opts)
			require.NoError(t, err)
			require.NotNil(t, svc)
			svc.Close()
		})
	}
}

// TestRemoveTorrent_Concurrent tests concurrent remove calls
func TestRemoveTorrent_Concurrent(t *testing.T) {
	svc := createTestService(t)

	t.Run("concurrent remove of non-existent", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			err := svc.RemoveTorrent(context.Background(), "nonexistent")
			assert.Error(t, err)
		}
	})
}
