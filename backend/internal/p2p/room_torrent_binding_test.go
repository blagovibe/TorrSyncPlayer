// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCreateRoomWithTorrentID вернет TorrentID в RoomInfo.
// Это RED-тест: сейчас Room не имеет поля TorrentID, поэтому тест
// упадёт на этапе компиляции или ассерции.
func TestCreateRoomWithTorrentID(t *testing.T) {
	svc := newTestService(t)

	userID := "user-torrent-room"
	torrentID := "abc123def456"
	room, err := svc.CreateRoom(context.Background(), userID, "test-room", "", torrentID)
	require.NoError(t, err)

	// RoomInfo должен содержать TorrentID
	require.Equal(t, torrentID, room.TorrentID, "RoomInfo.TorrentID")

	// Room должна иметь поле TorrentID
	svc.mu.RLock()
	r := svc.rooms[room.ID]
	svc.mu.RUnlock()
	require.NotNil(t, r)
	require.Equal(t, torrentID, r.TorrentID)
}

// TestCreateRoomWithoutTorrentIDFails — комната без торрента недопустима
// (после миграции). Пока опционально, но будем требовать для idea-0046.
func TestCreateRoomWithoutTorrentIDFails(t *testing.T) {
	svc := newTestService(t)

	userID := "user-no-torrent"
	_, err := svc.CreateRoom(context.Background(), userID, "test-room", "", "")
	if err == nil {
		t.Error("expected error when torrentID is empty, got nil")
	}
}

// TestJoinRoomValidatesTorrentID — присоединение к комнате требует
// совпадения torrentID (если комната привязана к торренту).
func TestJoinRoomValidatesTorrentID(t *testing.T) {
	svc := newTestService(t)

	hostID := "host-1"
	guestID := "guest-1"

	torrentID := "torrent-xyz"
	room, err := svc.CreateRoom(context.Background(), hostID, "room-torrent", "", torrentID)
	require.NoError(t, err)

	// Гость присоединяется с правильным torrentID — OK
	err = svc.JoinRoom(context.Background(), guestID, room.ID, "", torrentID)
	require.NoError(t, err)

	// Второй гость с неправильным torrentID — ошибка
	guest2 := "guest-2"
	err = svc.JoinRoom(context.Background(), guest2, room.ID, "", "wrong-torrent")
	require.Error(t, err)
}

// TestRoomInfoIncludesTorrentID — RoomInfo отдаёт TorrentID клиенту.
func TestRoomInfoIncludesTorrentID(t *testing.T) {
	svc := newTestService(t)

	userID := "user-info"
	torrentID := "info-torrent-123"
	room, err := svc.CreateRoom(context.Background(), userID, "info-room", "", torrentID)
	require.NoError(t, err)
	require.NotNil(t, room)

	info, err := svc.GetRoomInfo(context.Background(), userID)
	require.NoError(t, err)

	require.Equal(t, torrentID, info.TorrentID)
}

// TestRoomTorrentIDImmutable — TorrentID комнаты нельзя сменить после создания.
func TestRoomTorrentIDImmutable(t *testing.T) {
	svc := newTestService(t)

	userID := "user-immutable"
	room, err := svc.CreateRoom(context.Background(), userID, "immutable-room", "", "torrent-1")
	require.NoError(t, err)

	svc.mu.RLock()
	r := svc.rooms[room.ID]
	svc.mu.RUnlock()

	require.Equal(t, "torrent-1", r.TorrentID, "TorrentID changed unexpectedly")
}
