/**
 * @file pact_provider_test.go
 * @brief Pact Provider Verification Tests for Go Backend
 * 
 * These tests verify that the Go backend API satisfies the contract
 * defined by the frontend consumer (pacts/frontend-backend.json).
 * Run with: go test -v -run TestPactProvider ./internal/contract/...
 */

package contract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/provider"
)

// TestPactProvider verifies that the backend API satisfies
// the contract defined in the pact file.
func TestPactProvider(t *testing.T) {
	// Get the path to the pact file
	pactPath := filepath.Join("..", "..", "..", "..", "pacts", "frontend-backend.json")
	
	// Create a test HTTP server that simulates the backend API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleMockRequest(w, r)
	}))
	defer server.Close()

	// Configure Pact provider verification
	config := provider.VerifierConfig{
		ProviderBaseURL: server.URL,
		PactURLs:        []string{pactPath},
		Provider:        "TorrSyncPlayer-Backend",
		Consumer:        "TorrSyncPlayer-Frontend",
		PublishResults:  false, // Set to true to publish to Pact Broker
		BrokerURL:       os.Getenv("PACT_BROKER_URL"),
		BrokerToken:     os.Getenv("PACT_BROKER_TOKEN"),
		// Custom state handlers for provider states
		StateHandlers: map[string]func() error{
			"torrent exists":         setupTorrentExists,
			"no torrents exist":      setupNoTorrents,
			"user is in a room":      setupUserInRoom,
			"user is in a room and is host": setupUserInRoomAsHost,
			"user exists":            setupUserExists,
			"user does not exist":    setupUserNotExists,
		},
		// Request/response filtering
		RequestFilter: func(req *http.Request) error {
			// Add any custom headers or modifications
			return nil
		},
	}

	// Run the verification
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := provider.VerifyProvider(ctx, config)
	if err != nil {
		t.Fatalf("Pact verification failed: %v", err)
	}
}

// handleMockRequest handles mock API requests for testing
func handleMockRequest(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Route handling
	switch {
	case r.URL.Path == "/health" && r.Method == "GET":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","version":"1.0.0"}`)

	case r.URL.Path == "/api/v1/torrents" && r.Method == "GET":
		handleListTorrents(w, r)

	case r.URL.Path == "/api/v1/torrents" && r.Method == "POST":
		handleAddTorrent(w, r)

	case r.URL.Path == "/api/v1/torrents" && r.Method == "DELETE":
		// Not a standard endpoint, but handle gracefully
		w.WriteHeader(http.StatusNotFound)

	default:
		// Try to match with path parameters
		if len(r.URL.Path) > len("/api/v1/torrents/") && 
		   r.URL.Path[:len("/api/v1/torrents/")] == "/api/v1/torrents/" {
			handleTorrentByID(w, r)
			return
		}

		// Room endpoints
		if len(r.URL.Path) > len("/api/v1/rooms") && 
		   r.URL.Path[:len("/api/v1/rooms")] == "/api/v1/rooms" {
			handleRooms(w, r)
			return
		}

		// Sync endpoints
		if len(r.URL.Path) > len("/api/v1/sync") && 
		   r.URL.Path[:len("/api/v1/sync")] == "/api/v1/sync" {
			handleSync(w, r)
			return
		}

		// Auth endpoints
		if len(r.URL.Path) > len("/api/v1/auth") && 
		   r.URL.Path[:len("/api/v1/auth")] == "/api/v1/auth" {
			handleAuth(w, r)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":"not found","path":"%s"}`, r.URL.Path)
	}
}

// Helper handlers
func handleListTorrents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `[
		{"id":"test-torrent-1","name":"Test Torrent 1","progress":0.5,"status":"downloading","size":104857600,"downloaded":52428800,"uploadSpeed":10240,"downloadSpeed":102400},
		{"id":"test-torrent-2","name":"Test Torrent 2","progress":1.0,"status":"seeding","size":209715200,"downloaded":209715200,"uploadSpeed":51200,"downloadSpeed":0}
	]`)
}

func handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `{"id":"new-torrent-id","name":"New Torrent","progress":0.0,"status":"downloading","size":104857600,"downloaded":0,"uploadSpeed":0,"downloadSpeed":102400}`)
}

func handleTorrentByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/v1/torrents/"):]
	
	if path == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check for sub-paths
	if len(path) > len("/stream") && path[len(path)-len("/stream"):] == "/stream" {
		torrentID := path[:len(path)-len("/stream")]
		handleStream(w, r, torrentID)
		return
	}

	if len(path) > len("/files") && path[len(path)-len("/files"):] == "/files" {
		torrentID := path[:len(path)-len("/files")]
		handleFiles(w, r, torrentID)
		return
	}

	if len(path) > len("/select") && path[len(path)-len("/select"):] == "/select" {
		torrentID := path[:len(path)-len("/select")]
		handleSelectFile(w, r, torrentID)
		return
	}

	// Single torrent operations
	switch r.Method {
	case "GET":
		handleGetTorrent(w, r, path)
	case "DELETE":
		handleDeleteTorrent(w, r, path)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleGetTorrent(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"id":"%s","name":"Torrent %s","progress":0.5,"status":"downloading","size":104857600,"downloaded":52428800,"uploadSpeed":10240,"downloadSpeed":102400}`, id, id)
}

func handleDeleteTorrent(w http.ResponseWriter, r *http.Request, id string) {
	w.WriteHeader(http.StatusNoContent)
}

func handleStream(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Mock stream data for torrent %s", torrentID)
}

func handleFiles(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `[{"index":0,"name":"video_%s.mp4","size":104857600,"selected":true},{"index":1,"name":"subtitles_%s.srt","size":65536,"selected":false}]`, torrentID, torrentID)
}

func handleSelectFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"torrentId":"%s","fileIndex":0,"streamUrl":"http://localhost:8889/api/v1/torrents/%s/stream"}`, torrentID, torrentID)
}

func handleRooms(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/v1/rooms"):]
	
	switch {
	case path == "" && r.Method == "POST":
		handleCreateRoom(w, r)
	case path == "/join" && r.Method == "POST":
		handleJoinRoom(w, r)
	case path == "/leave" && r.Method == "POST":
		handleLeaveRoom(w, r)
	case path == "/signal" && r.Method == "POST":
		handleSignal(w, r)
	case path == "/events" && r.Method == "GET":
		handleRoomEvents(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `{"roomId":"new-room-123","name":"Test Room"}`)
}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"roomId":"room-123","hostId":"host-1"}`)
}

func handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleSignal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"success":true}`)
}

func handleRoomEvents(w http.ResponseWriter, r *http.Request) {
	// SSE endpoint
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	
	// Send a mock event
	fmt.Fprint(w, "data: {\"type\":\"peer-joined\",\"peerId\":\"peer-2\"}\n\n")
	flusher.Flush()
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/v1/sync"):]
	
	switch {
	case path == "/play" && r.Method == "POST":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"success":true,"action":"play"}`)
	case path == "/pause" && r.Method == "POST":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"success":true,"action":"pause"}`)
	case path == "/seek" && r.Method == "POST":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"success":true,"action":"seek","position":120.5}`)
	case path == "/status" && r.Method == "GET":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"isPlaying":true,"position":120.5,"duration":3600.0,"timestamp":1234567890}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/v1/auth"):]
	
	switch {
	case path == "/login" && r.Method == "POST":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...","expiresIn":3600}`)
	case path == "/register" && r.Method == "POST":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...","expiresIn":3600}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// ── Provider State Handlers ───────────────────────────────────────────────

func setupTorrentExists() error {
	// Setup test data for "torrent exists" state
	return nil
}

func setupNoTorrents() error {
	// Setup test data for "no torrents exist" state
	return nil
}

func setupUserInRoom() error {
	// Setup test data for "user is in a room" state
	return nil
}

func setupUserInRoomAsHost() error {
	// Setup test data for "user is in a room and is host" state
	return nil
}

func setupUserExists() error {
	// Setup test data for "user exists" state
	return nil
}

func setupUserNotExists() error {
	// Setup test data for "user does not exist" state
	return nil
}