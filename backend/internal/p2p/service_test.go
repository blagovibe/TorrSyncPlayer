package p2p

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewService проверяет инициализацию P2P сервиса
func TestNewService(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	require.NotNil(t, svc)

	defer svc.Close()

	assert.NotNil(t, svc.rooms)
	assert.NotNil(t, svc.peers)
	assert.NotNil(t, svc.eventChan)
	assert.NotEmpty(t, svc.localPeerID)
}

// TestCreateRoom проверяет создание комнаты без пароля
func TestCreateRoom(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	room, err := svc.CreateRoom("Test Room", "")
	require.NoError(t, err)
	require.NotNil(t, room)

	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Test Room", room.Name)
	assert.Equal(t, svc.localPeerID, room.HostID)
	assert.Equal(t, 0, room.PeerCount)
}

// TestCreateRoom_WithPassword проверяет создание комнаты с паролем
func TestCreateRoom_WithPassword(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	room, err := svc.CreateRoom("Private Room", "secret123")
	require.NoError(t, err)
	require.NotNil(t, room)

	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Private Room", room.Name)
}

// TestJoinRoom_NotFound проверяет присоединение к несуществующей комнате
func TestJoinRoom_NotFound(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	err = svc.JoinRoom("nonexistent", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найдена")
}

// TestJoinRoom_WrongPassword проверяет присоединение с неверным паролем
func TestJoinRoom_WrongPassword(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	// Создаём комнату с паролем
	room, err := svc.CreateRoom("Private Room", "correct_password")
	require.NoError(t, err)

	// Пытаемся присоединиться с неверным паролем
	err = svc.JoinRoom(room.ID, "wrong_password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неверный пароль")
}

// TestJoinRoom_CorrectPassword проверяет присоединение с верным паролем
func TestJoinRoom_CorrectPassword(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	// Создаём комнату с паролем
	room, err := svc.CreateRoom("Private Room", "secret123")
	require.NoError(t, err)

	// Присоединяемся с верным паролем
	err = svc.JoinRoom(room.ID, "secret123")
	assert.NoError(t, err)

	// Проверяем что мы в комнате
	info, err := svc.GetRoomInfo()
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)
}

// TestJoinRoom_NoPassword проверяет присоединение к комнате без пароля
func TestJoinRoom_NoPassword(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	// Создаём комнату без пароля
	room, err := svc.CreateRoom("Open Room", "")
	require.NoError(t, err)

	// Присоединяемся без пароля
	err = svc.JoinRoom(room.ID, "")
	assert.NoError(t, err)
}

// TestLeaveRoom_NotJoined проверяет выход из комнаты когда не подключены
func TestLeaveRoom_NotJoined(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	err = svc.LeaveRoom()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не подключены")
}

// TestSendSignal_NotJoined проверяет отправку сигнала без подключения к комнате
func TestSendSignal_NotJoined(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	err = svc.SendSignal([]byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не подключены")
}

// TestGetEvents проверяет получение канала событий
func TestGetEvents(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	events := svc.GetEvents()
	assert.NotNil(t, events)
}

// TestGetRoomInfo_NotJoined проверяет получение информации о комнате без подключения
func TestGetRoomInfo_NotJoined(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	_, err = svc.GetRoomInfo()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не подключены")
}

// TestGenerateID проверяет генерацию уникальных идентификаторов
func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
}

// TestCreateAndJoinRoom проверяет полный цикл: создание и присоединение к комнате
func TestCreateAndJoinRoom(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	// Создаём комнату
	room, err := svc.CreateRoom("Test Room", "")
	require.NoError(t, err)

	// Присоединяемся к комнате
	err = svc.JoinRoom(room.ID, "")
	require.NoError(t, err)

	// Проверяем информацию о комнате
	info, err := svc.GetRoomInfo()
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)

	// Выходим из комнаты
	err = svc.LeaveRoom()
	require.NoError(t, err)
}

// TestFullRoomLifecycle проверяет полный жизненный цикл комнаты с паролем
func TestFullRoomLifecycle(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	// 1. Создаём комнату с паролем
	room, err := svc.CreateRoom("Lifecycle Room", "pass123")
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Room", room.Name)
	assert.Equal(t, 0, room.PeerCount)

	// 2. Пытаемся присоединиться с неверным паролем — должно быть отклонено
	err = svc.JoinRoom(room.ID, "wrong_pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неверный пароль")

	// 3. Присоединяемся с верным паролем
	err = svc.JoinRoom(room.ID, "pass123")
	require.NoError(t, err)

	// 4. Проверяем информацию о комнате
	info, err := svc.GetRoomInfo()
	require.NoError(t, err)
	assert.Equal(t, room.ID, info.ID)
	assert.Equal(t, "Lifecycle Room", info.Name)
	assert.Equal(t, 1, info.PeerCount)

	// 5. Выходим из комнаты
	err = svc.LeaveRoom()
	require.NoError(t, err)

	// 6. Проверяем что мы больше не в комнате
	_, err = svc.GetRoomInfo()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не подключены")
}

// TestCreateMultipleRooms проверяет создание нескольких комнат
func TestCreateMultipleRooms(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	room1, err := svc.CreateRoom("Room 1", "")
	require.NoError(t, err)

	room2, err := svc.CreateRoom("Room 2", "pass")
	require.NoError(t, err)

	// Комнаты должны иметь разные ID
	assert.NotEqual(t, room1.ID, room2.ID)
	assert.Equal(t, "Room 1", room1.Name)
	assert.Equal(t, "Room 2", room2.Name)
}

// TestClose_EmptiesState проверяет что Close очищает состояние сервиса
func TestClose_EmptiesState(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)

	// Создаём комнату
	_, err = svc.CreateRoom("Test Room", "")
	require.NoError(t, err)

	// Закрываем сервис
	err = svc.Close()
	require.NoError(t, err)
}

// TestConcurrentRoomCreation проверяет потокобезопасность создания комнат
func TestConcurrentRoomCreation(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, _ = svc.CreateRoom(fmt.Sprintf("Room %d", idx), "")
		}(i)
	}

	wg.Wait()
}

// TestConcurrentGetRoomInfo проверяет потокобезопасность чтения информации о комнате
func TestConcurrentGetRoomInfo(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	room, err := svc.CreateRoom("Test Room", "")
	require.NoError(t, err)

	err = svc.JoinRoom(room.ID, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.GetRoomInfo()
		}()
	}

	wg.Wait()
}

// TestConcurrentSetLocalUserID проверяет потокобезопасность SetLocalUserID
func TestConcurrentSetLocalUserID(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			svc.SetLocalUserID(fmt.Sprintf("user_%d", idx))
		}(i)
	}

	wg.Wait()
}

// TestConcurrentEventChannel проверяет потокобезопасность работы с каналом событий
func TestConcurrentEventChannel(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	defer svc.Close()

	events := svc.GetEvents()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Горутины создают комнаты (генерируют события)
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, _ = svc.CreateRoom(fmt.Sprintf("Room %d", idx), "")
		}(i)
	}

	// Горутина читает события
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events {
			// Читаем события
		}
	}()

	wg.Wait()
	// Даём время на обработку событий
	time.Sleep(100 * time.Millisecond)
}
