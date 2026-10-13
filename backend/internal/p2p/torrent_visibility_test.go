// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0049 (слой 2): что пиру видно из торрентов.
//
// Токен доступа один на процесс, поэтому «знаешь токен — хозяин» не различает
// гостя комнаты и владельца сервера. Слой 1 закрыл разрушительную часть
// (удаление). Здесь — часть чтения: пиру видны только его собственные торренты
// и торренты комнат, в которых он состоит.
//
// Связка «комната ↔ торрент» уже есть (bug-0047), осталось её спросить.

// joinedRoom создаёт комнату на торренте и вводит в неё пира.
func joinedRoom(t *testing.T, svc *Service, host, guest, torrentID string) {
	t.Helper()
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, host, "Room", "", torrentID)
	require.NoError(t, err)

	if guest != "" {
		require.NoError(t, svc.JoinRoom(ctx, guest, info.ID, "", torrentID))
	}
}

func TestTorrentsVisibleTo_GuestSeesJoinedRoomTorrent(t *testing.T) {
	svc := newTestService(t)
	joinedRoom(t, svc, "host", "guest", "torrent-A")

	assert.True(t, svc.TorrentsVisibleTo(context.Background(), "guest")["torrent-A"],
		"гость комнаты обязан видеть торрент этой комнаты — иначе он не сможет играть")
}

// Главный сценарий слоя 2: чужой торрент не должен попадать в выдачу.
func TestTorrentsVisibleTo_GuestDoesNotSeeForeignTorrents(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	joinedRoom(t, svc, "host", "guest", "torrent-A")
	// Второй хост со своей комнатой — его торрент гостю не показываем.
	joinedRoom(t, svc, "other-host", "", "torrent-OTHER")

	visible := svc.TorrentsVisibleTo(ctx, "guest")
	assert.True(t, visible["torrent-A"])
	assert.NotContains(t, visible, "torrent-OTHER",
		"торрент чужой комнаты гостю не виден")
}

// Пир без комнаты не видит ничего: единственный торрент принадлежит хосту.
func TestTorrentsVisibleTo_RoomlessUserSeesNothing(t *testing.T) {
	svc := newTestService(t)
	joinedRoom(t, svc, "host", "", "torrent-A")

	assert.Empty(t, svc.TorrentsVisibleTo(context.Background(), "stranger"),
		"без комнаты пир не имеет права ни на что")
}

// Неизвестный идентификатор — тоже ничего. Отсутствие сессии не должно
// читаться как «разрешено»: иначе отказ в обслуживании выдавал бы доступ.
func TestTorrentsVisibleTo_UnknownUserSeesNothing(t *testing.T) {
	svc := newTestService(t)
	joinedRoom(t, svc, "host", "", "torrent-A")

	assert.Empty(t, svc.TorrentsVisibleTo(context.Background(), "never-seen"),
		"неизвестный идентификатор не должен давать доступ")
}

// Выход отбирает видимость, иначе вышедший гость сохранил бы доступ навсегда.
func TestTorrentsVisibleTo_LeavingRemovesVisibility(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	joinedRoom(t, svc, "host", "guest", "torrent-A")
	require.True(t, svc.TorrentsVisibleTo(ctx, "guest")["torrent-A"])

	require.NoError(t, svc.LeaveRoom(ctx, "guest"))
	assert.Empty(t, svc.TorrentsVisibleTo(ctx, "guest"),
		"после выхода видимость торрента комнаты должна исчезнуть")
}

// Хост своей комнаты видит её торрент: два слоя вместе не должны отрезать
// хозяина от его же данных.
func TestTorrentsVisibleTo_HostSeesOwnRoomTorrent(t *testing.T) {
	svc := newTestService(t)
	joinedRoom(t, svc, "host", "", "torrent-A")

	assert.True(t, svc.TorrentsVisibleTo(context.Background(), "host")["torrent-A"],
		"хост обязан видеть торрент своей комнаты")
}

// Комната без привязанного торрента не должна добавлять пустой ключ: он
// совпал бы с валидным torrentID и выдал бы лишний доступ.
func TestTorrentsVisibleTo_RoomWithoutTorrentAddsNothing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	joinedRoom(t, svc, "host", "", "torrent-A")

	svc.mu.Lock()
	for _, room := range svc.rooms {
		room.TorrentID = ""
	}
	svc.mu.Unlock()

	assert.Empty(t, svc.TorrentsVisibleTo(ctx, "host"),
		"комната без торрента не даёт видимости")
}

// Параллельные вызовы не должны падать: проверка видимости стоит в пути чтения
// каждого запроса, а состав комнат меняется извне.
func TestTorrentsVisibleTo_ConcurrentWithMembershipChanges(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	joinedRoom(t, svc, "host", "guest", "torrent-A")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			info, err := svc.CreateRoom(ctx, "churn", "Churn", "", "torrent-X")
			if err != nil {
				return
			}
			_ = svc.JoinRoom(ctx, "churn-guest", info.ID, "", "torrent-X")
			_ = svc.LeaveRoom(ctx, "churn-guest")
		}
	}()

	for i := 0; i < 200; i++ {
		assert.NotPanics(t, func() { svc.TorrentsVisibleTo(ctx, "guest") })
	}
	<-done
}
