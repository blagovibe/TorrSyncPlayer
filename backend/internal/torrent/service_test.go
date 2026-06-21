// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
