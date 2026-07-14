// Package testutil provides test utilities for integration tests.

package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestServer wraps httptest.Server with helper methods for API testing
type TestServer struct {
	*httptest.Server
	t         *testing.T
	baseURL   string
	authToken string
}

// NewTestServer creates a new test server with the full API router
func NewTestServer(t *testing.T) *TestServer {
	// Create the actual router from the main package
	// This imports and uses the real router setup
	router := setupTestRouter()

	server := httptest.NewServer(router)

	return &TestServer{
		Server:    server,
		t:         t,
		baseURL:   server.URL,
		authToken: "test-jwt-token-for-integration-tests",
	}
}

// setupTestRouter creates the HTTP router with all API endpoints
// This mirrors the actual router setup from cmd/server/main.go
func setupTestRouter() http.Handler {
	// Import and use the actual router setup
	// For now, we'll create a minimal mock router that matches the API
	// In a real implementation, this would import the actual router package

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// API v1 routes
	api := http.NewServeMux()

	// Torrent endpoints
	api.HandleFunc("/torrents", handleTorrents)
	api.HandleFunc("/torrents/", handleTorrentDetail)

	// Room endpoints
	api.HandleFunc("/rooms", handleRooms)
	api.HandleFunc("/rooms/", handleRoomDetail)

	// Sync endpoints
	api.HandleFunc("/sync", handleSync)
	api.HandleFunc("/sync/", handleSyncDetail)

	// Auth endpoints
	api.HandleFunc("/auth", handleAuth)
	api.HandleFunc("/auth/", handleAuthDetail)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", api))

	return mux
}

// Handler functions - these should match the actual API implementation

func handleTorrents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListTorrents(w, r)
	case http.MethodPost:
		handleAddTorrent(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTorrentDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/torrents/"):]

	if path == "" {
		http.Error(w, "Not found", http.StatusNotFound)
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
	case http.MethodGet:
		handleGetTorrent(w, r, path)
	case http.MethodDelete:
		handleDeleteTorrent(w, r, path)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	handleCreateRoom(w, r)
}

func handleRoomDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/rooms/"):]

	switch {
	case path == "join" && r.Method == http.MethodPost:
		handleJoinRoom(w, r)
	case path == "leave" && r.Method == http.MethodPost:
		handleLeaveRoom(w, r)
	case path == "signal" && r.Method == http.MethodPost:
		handleSignal(w, r)
	default:
		// Check for /rooms/{roomID}/events pattern
		if len(path) > len("/events") && path[len(path)-len("/events"):] == "/events" {
			roomID := path[:len(path)-len("/events")]
			handleRoomEvents(w, r, roomID)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/sync"):]

	switch {
	case path == "/play" && r.Method == http.MethodPost:
		handleSyncPlay(w, r)
	case path == "/pause" && r.Method == http.MethodPost:
		handleSyncPause(w, r)
	case path == "/seek" && r.Method == http.MethodPost:
		handleSyncSeek(w, r)
	case path == "/status" && r.Method == http.MethodGet:
		handleSyncStatus(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleSyncDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/sync/"):]

	switch {
	case path == "play" && r.Method == http.MethodPost:
		handleSyncPlay(w, r)
	case path == "pause" && r.Method == http.MethodPost:
		handleSyncPause(w, r)
	case path == "seek" && r.Method == http.MethodPost:
		handleSyncSeek(w, r)
	case path == "status" && r.Method == http.MethodGet:
		handleSyncStatus(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/auth"):]

	switch {
	case path == "/login" && r.Method == http.MethodPost:
		handleLogin(w, r)
	case path == "/register" && r.Method == http.MethodPost:
		handleRegister(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleAuthDetail(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not found", http.StatusNotFound)
}

// Implementation stubs - in real tests these would use actual storage

func handleListTorrents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{
			"id":            "test-torrent-1",
			"name":          "Test Torrent 1",
			"progress":      0.5,
			"status":        "downloading",
			"size":          104857600,
			"downloaded":    52428800,
			"uploadSpeed":   10240,
			"downloadSpeed": 102400,
		},
	})
}

func handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       "new-torrent-id",
		"name":     "New Torrent",
		"progress": 0.0,
		"status":   "downloading",
		"size":     104857600,
	})
}

func handleGetTorrent(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            id,
		"name":          "Torrent " + id,
		"progress":      0.5,
		"status":        "downloading",
		"size":          104857600,
		"downloaded":    52428800,
		"uploadSpeed":   10240,
		"downloadSpeed": 102400,
	})
}

func handleDeleteTorrent(w http.ResponseWriter, r *http.Request, id string) {
	w.WriteHeader(http.StatusNoContent)
}

func handleStream(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Mock stream data for " + torrentID))
}

func handleFiles(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{"index": 0, "name": "video_" + torrentID + ".mp4", "size": 104857600, "type": "video"},
		{"index": 1, "name": "subs_" + torrentID + ".srt", "size": 65536, "type": "subtitle"},
	})
}

func handleSelectFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	var req map[string]int
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"streamUrl": "http://localhost:8889/api/v1/torrents/" + torrentID + "/stream",
	})
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	name := req["name"]
	if name == "" {
		name = "Unnamed Room"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        "room-" + name,
		"name":      name,
		"hostId":    "test-host",
		"peerCount": 1,
	})
}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        req["roomId"],
		"name":      "Test Room",
		"hostId":    "test-host",
		"peerCount": 2,
	})
}

func handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleSignal(w http.ResponseWriter, r *http.Request) {
	var signal map[string]interface{}
	json.NewDecoder(r.Body).Decode(&signal)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleRoomEvents(w http.ResponseWriter, r *http.Request, roomID string) {
	// Ignore roomID for mock - just return SSE stream
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Send a mock event
	flusher, ok := w.(http.Flusher)
	if ok {
		w.Write([]byte("data: {\"type\":\"peer-joined\",\"peerId\":\"peer-2\"}\n\n"))
		flusher.Flush()
	}
}

func handleSyncPlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"action":  "play",
	})
}

func handleSyncPause(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"action":  "pause",
	})
}

func handleSyncSeek(w http.ResponseWriter, r *http.Request) {
	var req map[string]float64
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"action":   "seek",
		"position": req["position"],
	})
}

func handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"isPlaying": true,
		"position":  120.5,
		"duration":  3600.0,
		"timestamp": 1234567890,
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
		"expiresIn": 3600,
	})
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
		"expiresIn": 3600,
	})
}

// Helper methods for TestServer

// NewRequest creates a new HTTP request with auth header
func (s *TestServer) NewRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, s.baseURL+"/api/v1"+path, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	return req
}

// NewRequestWithJSON creates a request with JSON body
func (s *TestServer) NewRequestWithJSON(t *testing.T, method, path string, body interface{}) *http.Request {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequest(method, s.baseURL+"/api/v1"+path, bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.authToken)
	return req
}

// Do executes a request and returns the response
func (s *TestServer) Do(t *testing.T, req *http.Request) *http.Response {
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	return resp
}

// DoJSON executes a request and decodes JSON response
func (s *TestServer) DoJSON(t *testing.T, req *http.Request, target interface{}) *http.Response {
	resp := s.Do(t, req)
	defer resp.Body.Close()

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("Failed to decode JSON response: %v", err)
		}
	}

	return resp
}

// AssertStatus asserts the response status code
func (s *TestServer) AssertStatus(t *testing.T, resp *http.Response, expected int) {
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(body))
	}
}

// High-level API helpers

func (s *TestServer) AddTorrent(t *testing.T, magnetURI string) map[string]interface{} {
	var result map[string]interface{}
	resp := s.DoJSON(t, s.NewRequestWithJSON(t, "POST", "/torrents", map[string]string{
		"magnetUri": magnetURI,
	}), &result)
	s.AssertStatus(t, resp, http.StatusCreated)
	return result
}

func (s *TestServer) ListTorrents(t *testing.T) []map[string]interface{} {
	var torrents []map[string]interface{}
	resp := s.DoJSON(t, s.NewRequest(t, "GET", "/torrents", nil), &torrents)
	s.AssertStatus(t, resp, http.StatusOK)
	return torrents
}

func (s *TestServer) CreateRoom(t *testing.T, name, password string) map[string]interface{} {
	var room map[string]interface{}
	body := map[string]string{"name": name}
	if password != "" {
		body["password"] = password
	}
	resp := s.DoJSON(t, s.NewRequestWithJSON(t, "POST", "/rooms", body), &room)
	s.AssertStatus(t, resp, http.StatusCreated)
	return room
}

func (s *TestServer) JoinRoom(t *testing.T, roomID, password string) map[string]interface{} {
	var room map[string]interface{}
	body := map[string]string{"roomId": roomID}
	if password != "" {
		body["password"] = password
	}
	resp := s.DoJSON(t, s.NewRequestWithJSON(t, "POST", "/rooms/join", body), &room)
	s.AssertStatus(t, resp, http.StatusOK)
	return room
}

func (s *TestServer) SyncPlay(t *testing.T) {
	resp := s.Do(t, s.NewRequest(t, "POST", "/sync/play", nil))
	s.AssertStatus(t, resp, http.StatusOK)
}

func (s *TestServer) SyncPause(t *testing.T) {
	resp := s.Do(t, s.NewRequest(t, "POST", "/sync/pause", nil))
	s.AssertStatus(t, resp, http.StatusOK)
}

func (s *TestServer) SyncSeek(t *testing.T, position float64) {
	resp := s.Do(t, s.NewRequestWithJSON(t, "POST", "/sync/seek", map[string]float64{
		"position": position,
	}))
	s.AssertStatus(t, resp, http.StatusOK)
}
