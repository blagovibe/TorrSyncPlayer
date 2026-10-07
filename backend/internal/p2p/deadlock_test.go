package p2p

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
)

// TestCreateRoomWithPersistenceDoesNotDeadlock pins that creating a room
// returns when persistence is enabled.
//
// CreateRoom holds s.mu and calls scheduleSave(), which used to re-acquire that
// same non-reentrant mutex. With a nil persistence store scheduleSave returned
// before the second Lock, so every existing test passed; cmd/server enables
// persistence, so the re-acquire happened and the goroutine waited on a lock it
// already held. Room creation hung forever and, because every other P2P
// goroutine queued on s.mu, took the whole service with it.
//
// The test builds the service the way cmd/server does — persistence set — and
// fails on a hang rather than blocking the suite.
func TestCreateRoomWithPersistenceDoesNotDeadlock(t *testing.T) {
	authService, err := auth.NewAuthService([]byte("test-secret-key-for-p2p-deadlock-32bytes!"))
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}

	svc, err := NewService(authService)
	if err != nil {
		t.Fatalf("new p2p service: %v", err)
	}
	defer svc.Close()

	store, err := persistence.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("persistence store: %v", err)
	}
	svc.SetPersistence(store)

	if svc.persistence == nil {
		t.Fatal("test setup failed: persistence is nil, so it would not exercise the deadlock")
	}

	const goroutines = 8
	var wg sync.WaitGroup
	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := "user-" + string(rune('a'+i))
			_, err := svc.CreateRoom(context.Background(), user, "Room "+user, "")
			done <- err
		}(i)
	}

	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		close(done)
		for err := range done {
			if err != nil {
				t.Errorf("CreateRoom: %v", err)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatal("CreateRoom deadlocked with persistence enabled (waited 20s)")
	}
}
