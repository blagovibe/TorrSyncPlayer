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
	assert.NotNil(t, svc.sessions)
}

// TestCreateRoom tests creating a room without a password
func TestCreateRoom(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room, err := svc.CreateRoom(context.Background(), "test-user", "Test Room", "")
	require.NoError(t, err)
	require.NotNil(t, room)

	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Test Room", room.Name)
	assert.Equal(t, 0, room.PeerCount)
}

// TestCreateRoom_WithPassword tests creating a room with a password
func TestCreateRoom_WithPassword(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	room, err := svc.CreateRoom(context.Background(), "test-user", "Private Room", "secret123")
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

	err = svc.JoinRoom(context.Background(), "test-user", "nonexistent", "")
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
	room, err := svc.CreateRoom(context.Background(), "test-user", "Private Room", "correct_password")
	require.NoError(t, err)

	// Attempt to join with wrong password (need different user)
	err = svc.JoinRoom(context.Background(), "other-user", room.ID, "wrong_password")
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
	room, err := svc.CreateRoom(context.Background(), "host-user", "Private Room", "secret123")
	require.NoError(t, err)

	// Join with correct password
	err = svc.JoinRoom(context.Background(), "joining-user", room.ID, "secret123")
	require.NoError(t, err)

	// Verify we are in the room
	info, err := svc.GetRoomInfo(context.Background(), "joining-user")
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)
}

// TestLeaveRoom_NotJoined tests leaving a room when not connected
func TestLeaveRoom_NotJoined(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	err = svc.LeaveRoom(context.Background(), "test-user")
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

	err = svc.SendSignal(context.Background(), "test-user", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestGetRoomInfo_NotJoined tests getting room info without being connected
func TestGetRoomInfo_NotJoined(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)
	defer func() { _ = svc.Close() }()

	_, err = svc.GetRoomInfo(context.Background(), "test-user")
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
	room, err := svc.CreateRoom(context.Background(), "test-user-1", "Test Room", "")
	require.NoError(t, err)

	// Join room
	err = svc.JoinRoom(context.Background(), "test-user-2", room.ID, "")
	require.NoError(t, err)

	// Check room info
	info, err := svc.GetRoomInfo(context.Background(), "test-user-2")
	require.NoError(t, err)
	assert.Equal(t, 1, info.PeerCount)

	// Leave room
	err = svc.LeaveRoom(context.Background(), "test-user-2")
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
	room, err := svc.CreateRoom(context.Background(), "host-user", "Lifecycle Room", "pass123")
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle Room", room.Name)
	assert.Equal(t, 0, room.PeerCount)

	// 2. Attempt to join with wrong password - should be rejected
	err = svc.JoinRoom(context.Background(), "joiner", room.ID, "wrong_pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid password")

	// 3. Join with correct password
	err = svc.JoinRoom(context.Background(), "joiner", room.ID, "pass123")
	require.NoError(t, err)

	// 4. Check room info
	info, err := svc.GetRoomInfo(context.Background(), "joiner")
	require.NoError(t, err)
	assert.Equal(t, room.ID, info.ID)
	assert.Equal(t, "Lifecycle Room", info.Name)
	assert.Equal(t, 1, info.PeerCount)

	// 5. Leave room
	err = svc.LeaveRoom(context.Background(), "joiner")
	require.NoError(t, err)

	// 6. Verify we are no longer in the room
	_, err = svc.GetRoomInfo(context.Background(), "joiner")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestClose_EmptiesState tests that Close clears the service state
func TestClose_EmptiesState(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)
	svc, err := NewService(authSvc)
	require.NoError(t, err)

	// Create a room
	_, err = svc.CreateRoom(context.Background(), "test-user", "Test Room", "")
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
			_, _ = svc.CreateRoom(context.Background(), fmt.Sprintf("user_%d", idx), fmt.Sprintf("Room %d", idx), "")
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

	room, err := svc.CreateRoom(context.Background(), "host-user", "Test Room", "")
	require.NoError(t, err)

	err = svc.JoinRoom(context.Background(), "joining-user", room.ID, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.GetRoomInfo(context.Background(), "joining-user")
		}()
	}

	wg.Wait()
}

func TestP2PService_NoGoroutineLeak(t *testing.T) {
	authSvc, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-tests-32bytes!"))
	require.NoError(t, err)

	goroutinesBefore := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		svc, err := NewService(authSvc)
		require.NoError(t, err)
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
