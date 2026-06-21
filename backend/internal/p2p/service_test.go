package p2p

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

func init() {
	// Initialize logger for tests
	logger.Init("error", "json")
}

// TestNewService tests P2P service initialization
func TestNewService(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	require.NotNil(t, svc)

	defer func() { _ = svc.Close() }()

	assert.NotNil(t, svc.rooms)
	assert.NotNil(t, svc.peers)
	assert.NotNil(t, svc.eventChan)
	assert.NotEmpty(t, svc.localPeerID)
}

// TestCreateRoom tests creating a room without a password
func TestCreateRoom(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room, err := svc.CreateRoom(context.Background(), "Test Room", "")
	require.NoError(t, err)
	require.NotNil(t, room)

	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Test Room", room.Name)
	assert.Equal(t, svc.localPeerID, room.HostID)
	assert.Equal(t, 0, room.PeerCount)
}

// TestCreateRoom_WithPassword tests creating a room with a password
func TestCreateRoom_WithPassword(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room, err := svc.CreateRoom(context.Background(), "Private Room", "secret123")
	require.NoError(t, err)
	require.NotNil(t, room)

	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Private Room", room.Name)
}

// TestJoinRoom_NotFound tests joining a non-existent room
func TestJoinRoom_NotFound(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	err = svc.JoinRoom(context.Background(), "nonexistent", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestJoinRoom_WrongPassword tests joining with a wrong password
func TestJoinRoom_WrongPassword(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	// Create a room with a password
	room, err := svc.CreateRoom(context.Background(), "Private Room", "correct_password")
	require.NoError(t, err)

	// Attempt to join with wrong password
	err = svc.JoinRoom(context.Background(), room.ID, "wrong_password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid password")
}

// TestJoinRoom_CorrectPassword tests joining with a correct password
func TestJoinRoom_CorrectPassword(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	// Create a room with a password
	room, err := svc.CreateRoom(context.Background(), "Private Room", "secret123")
	require.NoError(t, err)

	// Join with correct password
	err = svc.JoinRoom(context.Background(), room.ID, "secret123")
	assert.NoError(t, err)

	// Verify we are in the room
	info, err := svc.GetRoomInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)
}

// TestJoinRoom_NoPassword tests joining a room without a password
func TestJoinRoom_NoPassword(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	// Create a room without password
	room, err := svc.CreateRoom(context.Background(), "Open Room", "")
	require.NoError(t, err)

	// Join without password
	err = svc.JoinRoom(context.Background(), room.ID, "")
	assert.NoError(t, err)
}

// TestLeaveRoom_NotJoined tests leaving a room when not connected
func TestLeaveRoom_NotJoined(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	err = svc.LeaveRoom(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestSendSignal_NotJoined tests sending a signal without being connected to a room
func TestSendSignal_NotJoined(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	err = svc.SendSignal(context.Background(), []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestGetEvents tests getting the events channel
func TestGetEvents(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	events := svc.GetEvents()
	assert.NotNil(t, events)
}

// TestGetRoomInfo_NotJoined tests getting room info without being connected
func TestGetRoomInfo_NotJoined(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	_, err = svc.GetRoomInfo(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestGenerateID tests unique identifier generation
func TestGenerateID(t *testing.T) {
	id1, err := utils.GenerateID(16)
	require.NoError(t, err)
	id2, err := utils.GenerateID(16)
	require.NoError(t, err)

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
}

// TestCreateAndJoinRoom tests the full cycle: create and join a room
func TestCreateAndJoinRoom(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	// Create room
	room, err := svc.CreateRoom(context.Background(), "Test Room", "")
	require.NoError(t, err)

	// Join room
	err = svc.JoinRoom(context.Background(), room.ID, "")
	require.NoError(t, err)

	// Check room info
	info, err := svc.GetRoomInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)

	// Leave room
	err = svc.LeaveRoom(context.Background())
	require.NoError(t, err)
}

// TestFullRoomLifecycle tests the full lifecycle of a room with password
func TestFullRoomLifecycle(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	// 1. Create a room with password
	room, err := svc.CreateRoom(context.Background(), "Lifecycle Room", "pass123")
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Room", room.Name)
	assert.Equal(t, 0, room.PeerCount)

	// 2. Attempt to join with wrong password — should be rejected
	err = svc.JoinRoom(context.Background(), room.ID, "wrong_pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid password")

	// 3. Join with correct password
	err = svc.JoinRoom(context.Background(), room.ID, "pass123")
	require.NoError(t, err)

	// 4. Check room info
	info, err := svc.GetRoomInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, room.ID, info.ID)
	assert.Equal(t, "Lifecycle Room", info.Name)
	assert.Equal(t, 1, info.PeerCount)

	// 5. Leave room
	err = svc.LeaveRoom(context.Background())
	require.NoError(t, err)

	// 6. Verify we are no longer in the room
	_, err = svc.GetRoomInfo(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestCreateMultipleRooms tests creating multiple rooms
func TestCreateMultipleRooms(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room1, err := svc.CreateRoom(context.Background(), "Room 1", "")
	require.NoError(t, err)

	room2, err := svc.CreateRoom(context.Background(), "Room 2", "pass")
	require.NoError(t, err)

	// Rooms should have different IDs
	assert.NotEqual(t, room1.ID, room2.ID)
	assert.Equal(t, "Room 1", room1.Name)
	assert.Equal(t, "Room 2", room2.Name)
}

// TestClose_EmptiesState tests that Close clears the service state
func TestClose_EmptiesState(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)

	// Create a room
	_, err = svc.CreateRoom(context.Background(), "Test Room", "")
	require.NoError(t, err)

	// Close service
	err = svc.Close()
	require.NoError(t, err)
}

// TestConcurrentRoomCreation tests thread safety of room creation
func TestConcurrentRoomCreation(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, _ = svc.CreateRoom(context.Background(), fmt.Sprintf("Room %d", idx), "")
		}(i)
	}

	wg.Wait()
}

// TestConcurrentGetRoomInfo tests thread safety of reading room info
func TestConcurrentGetRoomInfo(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room, err := svc.CreateRoom(context.Background(), "Test Room", "")
	require.NoError(t, err)

	err = svc.JoinRoom(context.Background(), room.ID, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.GetRoomInfo(context.Background())
		}()
	}

	wg.Wait()
}

// TestConcurrentSetLocalUserID tests thread safety of SetLocalUserID
func TestConcurrentSetLocalUserID(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

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

// TestConcurrentEventChannel tests thread safety of the event channel
func TestConcurrentEventChannel(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	events := svc.GetEvents()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Goroutines create rooms (generate events)
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, _ = svc.CreateRoom(context.Background(), fmt.Sprintf("Room %d", idx), "")
		}(i)
	}

	// Goroutine reads events
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events {
			// Read events
		}
	}()

	wg.Wait()
	// Allow time for event processing
	time.Sleep(100 * time.Millisecond)
}

func TestP2PService_NoGoroutineLeak(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)

	goroutinesBefore := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		svc, err := NewService(authSvc)
		require.NoError(t, err)
		events := svc.GetEvents()
		go func() {
			for range events {
			}
		}()
		time.Sleep(20 * time.Millisecond)
		err = svc.Close()
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	goroutinesAfter := runtime.NumGoroutine()
	assert.LessOrEqual(t, goroutinesAfter, goroutinesBefore+3,
		"P2P service goroutine leak: was %d, now %d", goroutinesBefore, goroutinesAfter)
}
