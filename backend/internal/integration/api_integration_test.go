/**
 * @file api_integration_test.go
 * @brief Integration tests for the full HTTP API
 * 
 * Tests complete API flows including torrent management,
 * room operations, and sync functionality.
 * Run with: go test -tags=integration -v ./internal/integration/...
 */

package integration

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/integration/testutil"
)

//go:build integration
// +build integration

func TestHealthEndpoint(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	resp := server.Do(t, server.NewRequest(t, "GET", "/health", nil))
	defer resp.Body.Close()

	server.AssertStatus(t, resp, 200)
}

func TestTorrentLifecycle(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	// 1. List torrents (should be empty or have test data)
	torrents := server.ListTorrents(t)
	t.Logf("Initial torrents: %d", len(torrents))

	// 2. Add torrent via magnet
	magnetURI := "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12&dn=Test+Torrent"
	result := server.AddTorrent(t, magnetURI)

	if result["id"] == nil {
		t.Error("Expected torrent ID in response")
	}
	torrentID := result["id"].(string)
	t.Logf("Added torrent: %s", torrentID)

	// 3. List torrents again (should have 1 more)
	torrents = server.ListTorrents(t)
	t.Logf("Torrents after add: %d", len(torrents))

	found := false
	for _, torrent := range torrents {
		if torrent["id"] == torrentID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Added torrent %s not found in list", torrentID)
	}

	// 4. Get torrent files
	resp := server.DoJSON(t, server.NewRequest(t, "GET", "/torrents/"+torrentID+"/files", nil), nil)
	server.AssertStatus(t, resp, 200)
	defer resp.Body.Close()

	// 5. Select file for streaming
	resp = server.Do(t, server.NewRequestWithJSON(t, "POST", "/torrents/"+torrentID+"/select", map[string]int{
		"fileIndex": 0,
	}))
	server.AssertStatus(t, resp, 200)
	defer resp.Body.Close()

	// 6. Remove torrent
	resp = server.Do(t, server.NewRequest(t, "DELETE", "/torrents/"+torrentID, nil))
	server.AssertStatus(t, resp, 204)
	defer resp.Body.Close()

	// 7. Verify removed
	torrents = server.ListTorrents(t)
	found = false
	for _, torrent := range torrents {
		if torrent["id"] == torrentID {
			found = true
			break
		}
	}
	if found {
		t.Errorf("Torrent %s still exists after removal", torrentID)
	}
}

func TestRoomLifecycle(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	// 1. Create room
	room := server.CreateRoom(t, "Test Room", "")
	roomID := room["id"].(string)

	if roomID == "" {
		t.Error("Expected room ID in response")
	}
	t.Logf("Created room: %s", roomID)

	// 2. Join room (as host, should work)
	joined := server.JoinRoom(t, roomID, "")
	if joined["id"] != roomID {
		t.Errorf("Joined room ID mismatch: expected %s, got %v", roomID, joined["id"])
	}
	t.Logf("Joined room: %s", roomID)

	// 3. Sync operations
	server.SyncPlay(t)
	server.SyncPause(t)
	server.SyncSeek(t, 120.5)

	// 4. Leave room
	resp := server.Do(t, server.NewRequest(t, "POST", "/rooms/leave", nil))
	server.AssertStatus(t, resp, 204)
	defer resp.Body.Close()
}

func TestSyncOperations(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	// Create and join room first
	room := server.CreateRoom(t, "Sync Test Room", "")
	server.JoinRoom(t, room["id"].(string), "")

	// Test sync play
	server.SyncPlay(t)

	// Test sync pause
	server.SyncPause(t)

	// Test sync seek
	server.SyncSeek(t, 42.5)
	server.SyncSeek(t, 0.0)
	server.SyncSeek(t, 3600.0)
}

func TestAuthFlow(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	// Test login
	resp := server.DoJSON(t, server.NewRequestWithJSON(t, "POST", "/auth/login", map[string]string{
		"username": "testuser",
		"password": "testpass123",
	}), nil)
	server.AssertStatus(t, resp, 200)
	defer resp.Body.Close()

	// Test register
	resp = server.DoJSON(t, server.NewRequestWithJSON(t, "POST", "/auth/register", map[string]string{
		"username": "newuser",
		"password": "newpass123",
	}), nil)
	server.AssertStatus(t, resp, 201)
	defer resp.Body.Close()
}

func TestFullUserScenario(t *testing.T) {
	server := testutil.NewTestServer(t)
	defer server.Close()

	// Scenario: User adds torrent, creates room, invites friend, syncs playback

	// 1. Add torrent
	magnet := "magnet:?xt=urn:btih:1234567890abcdef1234567890abcdef12345678&dn=Movie.mp4"
	torrent := server.AddTorrent(t, magnet)
	torrentID := torrent["id"].(string)

	// 2. Select file
	resp := server.Do(t, server.NewRequestWithJSON(t, "POST", "/torrents/"+torrentID+"/select", map[string]int{
		"fileIndex": 0,
	}))
	server.AssertStatus(t, resp, 200)
	defer resp.Body.Close()

	// 3. Create room
	room := server.CreateRoom(t, "Movie Night", "secret123")
	roomID := room["id"].(string)

	// 4. Host syncs play
	server.SyncPlay(t)

	// 5. Friend would join (simulated)
	friendServer := testutil.NewTestServer(t)
	defer friendServer.Close()

	friendJoined := friendServer.JoinRoom(t, roomID, "secret123")
	if friendJoined["id"] != roomID {
		t.Error("Friend failed to join room")
	}

	// 6. Host seeks, friend receives sync
	server.SyncSeek(t, 600.0)

	// 7. Both leave
	server.Do(t, server.NewRequest(t, "POST", "/rooms/leave", nil))
	friendServer.Do(t, friendServer.NewRequest(t, "POST", "/rooms/leave", nil))
}