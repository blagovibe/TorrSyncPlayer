package sync

import (
	"sync"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
)

// TestSyncWriteOpsWithPersistenceDoNotDeadlock pins that play, pause and seek
// return when persistence is enabled.
//
// All three hold s.mu for their whole body and call scheduleSave(), which used
// to take s.mu again. sync.Mutex is not reentrant, so each of these goroutines
// blocked on a lock it already owned and every other sync goroutine queued
// behind it. Against a server with persistence enabled — how cmd/server runs —
// the whole write side of the sync API hung forever. GetStatus only takes a read
// lock, which is why it kept answering while the writes were wedged: the API
// looked half-alive, which is what made this hard to spot.
//
// The guard `if s.persistence == nil { return }` sat before the second Lock, so
// every existing test — built without persistence — returned early and never
// reached the deadlock.
//
// The test drives all three operations from several goroutines, matching the
// concurrent access a load test produces. It fails on a hang rather than
// blocking the suite.
func TestSyncWriteOpsWithPersistenceDoNotDeadlock(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	store, err := persistence.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("persistence store: %v", err)
	}
	svc.SetPersistence(store)

	if svc.persistence == nil {
		t.Fatal("test setup failed: persistence is nil, so it would not exercise the deadlock")
	}

	const workers = 6
	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room := "room-" + string(rune('a'+i))
			svc.Play(room)
			svc.Pause(room)
			if _, err := svc.Seek(room, float64(i)); err != nil {
				t.Errorf("Seek: %v", err)
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("play/pause/seek deadlocked with persistence enabled (waited 20s)")
	}
}

// TestSyncGetStatusWorksWithoutWriteOps records the asymmetry that hid this bug:
// reads kept working while writes were deadlocked, so a health check against
// the sync API looks healthy on a completely wedged service.
func TestSyncGetStatusWorksWithoutWriteOps(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// SyncStatus carries playback state, not the room id, so assert on the
	// shape a never-played room returns.
	if got := svc.GetStatus("room-a"); got.IsPlaying {
		t.Fatalf("GetStatus: a never-played room must not report playing, got %+v", got)
	}
}
