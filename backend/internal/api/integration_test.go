// Package api предоставляет интеграционные тесты для HTTP API.
// Тестирует полный цикл работы с API: регистрация, логин, торрент операции, комнаты.
// Этот файл содержит только дополнительные интеграционные тесты, не дублирующие handlers_test.go.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// testServices содержит моки сервисов для тестирования
type integrationTestServices struct {
	torrentSvc *integrationMockTorrentService
	p2pSvc     *integrationMockP2PService
	syncSvc    *integrationMockSyncService
	authStore  *auth.UserStore
}

// integrationMockTorrentService мок для TorrentService
type integrationMockTorrentService struct {
	torrents map[string]*models.TorrentInfo
	files    map[string][]models.FileInfo
}

func newIntegrationMockTorrentService() *integrationMockTorrentService {
	return &integrationMockTorrentService{
		torrents: make(map[string]*models.TorrentInfo),
		files:    make(map[string][]models.FileInfo),
	}
}

func (m *integrationMockTorrentService) AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error) {
	if magnetURI == "" {
		return nil, fmt.Errorf("empty magnet URI")
	}

	info := &models.TorrentInfo{
		ID:       "test-torrent-id-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:     "Test Torrent",
		Progress: 0.5,
		Status:   "downloading",
		Size:     1024 * 1024 * 100, // 100 MB
	}
	m.torrents[info.ID] = info
	m.files[info.ID] = []models.FileInfo{
		{Index: 0, Name: "video.mp4", Size: 1024 * 1024 * 100},
	}
	return info, nil
}

func (m *integrationMockTorrentService) RemoveTorrent(id string) error {
	if _, exists := m.torrents[id]; !exists {
		return fmt.Errorf("torrent not found: %s", id)
	}
	delete(m.torrents, id)
	delete(m.files, id)
	return nil
}

func (m *integrationMockTorrentService) ListTorrents() []*models.TorrentInfo {
	result := make([]*models.TorrentInfo, 0, len(m.torrents))
	for _, t := range m.torrents {
		result = append(result, t)
	}
	return result
}

func (m *integrationMockTorrentService) GetFiles(torrentID string) ([]models.FileInfo, error) {
	files, exists := m.files[torrentID]
	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", torrentID)
	}
	return files, nil
}

func (m *integrationMockTorrentService) SelectFile(torrentID string, fileIndex int) error {
	files, exists := m.files[torrentID]
	if !exists {
		return fmt.Errorf("torrent not found: %s", torrentID)
	}
	if fileIndex < 0 || fileIndex >= len(files) {
		return fmt.Errorf("invalid file index: %d", fileIndex)
	}
	return nil
}

func (m *integrationMockTorrentService) ServeFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("test file content"))
}

func (m *integrationMockTorrentService) Close() error {
	return nil
}

func (m *integrationMockTorrentService) UpdateBufferPosition(torrentID string, position int64) {
	//
}

func (m *integrationMockTorrentService) GetBufferInfo(torrentID string) (*models.BufferInfo, error) {
	//
	return &models.BufferInfo{
		TorrentID:       torrentID,
		FileIndex:       0,
		CurrentPosition: 0,
		BufferStart:     0,
		BufferEnd:       1024 * 1024,
		BufferSize:      1024 * 1024,
		BufferedBytes:   512 * 1024,
		BufferedPercent: 50.0,
		DownloadSpeed:   1024 * 1024,
		IsBuffering:     true,
	}, nil
}

// integrationMockP2PService мок для P2PService
type integrationMockP2PService struct {
	rooms       map[string]*models.RoomInfo
	events      chan models.P2PEvent
	currentRoom string
}

func newIntegrationMockP2PService() *integrationMockP2PService {
	return &integrationMockP2PService{
		rooms:  make(map[string]*models.RoomInfo),
		events: make(chan models.P2PEvent, 10),
	}
}

func (m *integrationMockP2PService) CreateRoom(name, password string) (*models.RoomInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("room name cannot be empty")
	}

	room := &models.RoomInfo{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      name,
		HostID:    "test-host-id",
		PeerCount: 1,
	}
	m.rooms[room.ID] = room
	m.currentRoom = room.ID
	return room, nil
}

func (m *integrationMockP2PService) JoinRoomWithToken(roomID, password, token string) error {
	// Для тестов просто вызываем JoinRoom
	return m.JoinRoom(roomID, password)
}

func (m *integrationMockP2PService) AuthenticatePeer(peerID, token string) error {
	// Для тестов всегда успешно
	return nil
}

func (m *integrationMockP2PService) SetLocalUserID(userID string) {
	// Для тестов ничего не делаем
}

func (m *integrationMockP2PService) JoinRoom(roomID, password string) error {
	if _, exists := m.rooms[roomID]; !exists {
		return fmt.Errorf("room not found: %s", roomID)
	}
	m.currentRoom = roomID
	return nil
}

func (m *integrationMockP2PService) LeaveRoom() error {
	if m.currentRoom == "" {
		return fmt.Errorf("not in a room")
	}
	m.currentRoom = ""
	return nil
}

func (m *integrationMockP2PService) SendSignal(signal []byte) error {
	if m.currentRoom == "" {
		return fmt.Errorf("not in a room")
	}
	return nil
}

func (m *integrationMockP2PService) GetEvents() chan models.P2PEvent {
	return m.events
}

func (m *integrationMockP2PService) RoomEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		if _, err := fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n"); err != nil {
			return
		}
		flusher.Flush()

		// Отправляем тестовое событие
		select {
		case m.events <- models.P2PEvent{Type: "test_event", Data: "test"}:
		default:
		}

		// Ждём закрытия соединения
		<-r.Context().Done()
	}
}

func (m *integrationMockP2PService) GetRoomInfo() (*models.RoomInfo, error) {
	if m.currentRoom == "" {
		return nil, fmt.Errorf("not in a room")
	}
	room, exists := m.rooms[m.currentRoom]
	if !exists {
		return nil, fmt.Errorf("room not found")
	}
	return room, nil
}

func (m *integrationMockP2PService) Close() error {
	close(m.events)
	return nil
}

// integrationMockSyncService мок для SyncService
type integrationMockSyncService struct {
	status models.SyncStatus
}

func newIntegrationMockSyncService() *integrationMockSyncService {
	return &integrationMockSyncService{
		status: models.SyncStatus{
			IsPlaying: false,
			Position:  0,
			Duration:  0,
			Timestamp: time.Now().UnixMilli(),
		},
	}
}

func (m *integrationMockSyncService) Play() models.SyncStatus {
	m.status.IsPlaying = true
	m.status.Timestamp = time.Now().UnixMilli()
	return m.status
}

func (m *integrationMockSyncService) Pause() models.SyncStatus {
	m.status.IsPlaying = false
	m.status.Timestamp = time.Now().UnixMilli()
	return m.status
}

func (m *integrationMockSyncService) Seek(position float64) (models.SyncStatus, error) {
	if position < 0 || position > 86400 {
		return m.status, fmt.Errorf("invalid position: %f", position)
	}
	m.status.Position = position
	m.status.Timestamp = time.Now().UnixMilli()
	return m.status, nil
}

func (m *integrationMockSyncService) GetStatus() models.SyncStatus {
	return m.status
}

func (m *integrationMockSyncService) SetDuration(duration float64) error {
	if duration < 0 {
		return fmt.Errorf("invalid duration: %f", duration)
	}
	m.status.Duration = duration
	return nil
}

func (m *integrationMockSyncService) SyncWithLatency(peerStatus models.SyncStatus, latencyMs int) models.SyncStatus {
	m.status.Position = peerStatus.Position
	m.status.IsPlaying = peerStatus.IsPlaying
	m.status.Timestamp = time.Now().UnixMilli()
	return m.status
}

func (m *integrationMockSyncService) UpdatePosition(position float64) error {
	if position < 0 || position > 86400 {
		return fmt.Errorf("invalid position: %f", position)
	}
	m.status.Position = position
	m.status.Timestamp = time.Now().UnixMilli()
	return nil
}

func (m *integrationMockSyncService) Close() {
	m.status.IsPlaying = false
}

// setupIntegrationTestServer создаёт тестовый сервер с моками для интеграционных тестов
func setupIntegrationTestServer() (*httptest.Server, *integrationTestServices) {
	// Инициализируем логгер
	logger.Init("error", "text")

	services := &integrationTestServices{
		torrentSvc: newIntegrationMockTorrentService(),
		p2pSvc:     newIntegrationMockP2PService(),
		syncSvc:    newIntegrationMockSyncService(),
		authStore:  auth.NewUserStore(),
	}

	// Создаём auth service для тестов
	authService := auth.NewAuthService([]byte("test-secret-for-integration-tests-32bytes!"))

	config := RouterConfig{
		TorrentSvc:  services.torrentSvc,
		P2pSvc:      services.p2pSvc,
		SyncSvc:     services.syncSvc,
		AuthStore:   services.authStore,
		AuthService: authService,
	}

	router := NewRouter(config)
	server := httptest.NewServer(router)

	return server, services
}

// getIntegrationTestCSRFToken получает CSRF токен для тестов
func getIntegrationTestCSRFToken(server *httptest.Server) (string, error) {
	resp, err := http.Get(server.URL + "/api/v1/csrf-token")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result["csrfToken"], nil
}

// getIntegrationTestAuthToken получает JWT токен для тестов
func getIntegrationTestAuthToken(t *testing.T, server *httptest.Server) string {
	t.Helper()

	// Получаем CSRF токен
	csrfToken, err := getIntegrationTestCSRFToken(server)
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}

	// Регистрируем пользователя с CSRF токеном
	registerBody := `{"username":"integrationtestuser","password":"testpass123"}`
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}

	var authResp models.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		t.Fatalf("Failed to decode auth response: %v", err)
	}

	return authResp.Token
}

// TestIntegrationCSRFTokenEndpoint тестирует получение CSRF токена
func TestIntegrationCSRFTokenEndpoint(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/csrf-token")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["csrfToken"] == "" {
		t.Error("Expected non-empty CSRF token")
	}
}

// TestIntegrationAuthRegistration тестирует регистрацию пользователя
func TestIntegrationAuthRegistration(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	// Получаем CSRF токен для POST запросов
	csrfToken, err := getIntegrationTestCSRFToken(server)
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "valid registration",
			body:     `{"username":"newuser","password":"newpass123"}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "missing username",
			body:     `{"password":"newpass123"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing password",
			body:     `{"username":"newuser"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "short password",
			body:     `{"username":"newuser","password":"short"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid json",
			body:     `invalid json`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CSRF-Token", csrfToken)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantCode {
				t.Errorf("Expected status %d, got %d", tt.wantCode, resp.StatusCode)
			}

			// Обновляем CSRF токен для следующего теста
			newToken := resp.Header.Get("X-CSRF-Token")
			if newToken != "" {
				csrfToken = newToken
			}
		})
	}
}

// TestIntegrationAuthLogin тестирует вход в систему
func TestIntegrationAuthLogin(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	// Получаем CSRF токен
	csrfToken, err := getIntegrationTestCSRFToken(server)
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}

	// Сначала регистрируем пользователя
	registerBody := `{"username":"logintest","password":"loginpass123"}`
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}
	_ = resp.Body.Close()

	// Получаем новый CSRF токен для login
	csrfToken, err = getIntegrationTestCSRFToken(server)
	if err != nil {
		t.Fatalf("Failed to get CSRF token: %v", err)
	}

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "valid login",
			body:     `{"username":"logintest","password":"loginpass123"}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "wrong password",
			body:     `{"username":"logintest","password":"wrongpass"}`,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "non-existent user",
			body:     `{"username":"nonexistent","password":"somepass123"}`,
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CSRF-Token", csrfToken)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantCode {
				t.Errorf("Expected status %d, got %d", tt.wantCode, resp.StatusCode)
			}

			// Обновляем CSRF токен для следующего теста
			newToken := resp.Header.Get("X-CSRF-Token")
			if newToken != "" {
				csrfToken = newToken
			}
		})
	}
}

// TestIntegrationProtectedEndpointsWithoutAuth тестирует доступ к защищённым endpoints без аутентификации
func TestIntegrationProtectedEndpointsWithoutAuth(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	// GET запросы без JWT должны возвращать 401
	getEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/torrents"},
		{"GET", "/api/v1/rooms"},
		{"GET", "/api/v1/sync/status"},
	}

	for _, ep := range getEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			resp, err := http.Get(server.URL + ep.path)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected status 401, got %d", resp.StatusCode)
			}
		})
	}

	// POST запросы без JWT и без CSRF должны возвращать 403 (CSRF блокирует первым)
	postEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/v1/torrents", `{"magnetUri":"magnet:?xt=urn:btih:test"}`},
		{"POST", "/api/v1/rooms", `{"name":"test"}`},
	}

	for _, ep := range postEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			resp, err := http.Post(server.URL+ep.path, "application/json", strings.NewReader(ep.body))
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// POST без CSRF токена возвращает 403 Forbidden
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, got %d", resp.StatusCode)
			}
		})
	}
}

// TestIntegrationTorrentOperations тестирует операции с торрентами
func TestIntegrationTorrentOperations(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	token := getIntegrationTestAuthToken(t, server)

	// Тест добавления торрента
	t.Run("add torrent", func(t *testing.T) {
		body := `{"magnetUri":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/torrents", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		var torrent models.TorrentInfo
		if err := json.NewDecoder(resp.Body).Decode(&torrent); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if torrent.ID == "" {
			t.Error("Expected non-empty torrent ID")
		}
	})

	// Тест получения списка торрентов с пагинацией
	t.Run("list torrents with pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/torrents?limit=10&offset=0", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var result models.TorrentListResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Torrents == nil {
			t.Error("Expected non-nil torrents array")
		}

		if result.Limit != 10 {
			t.Errorf("Expected limit 10, got %d", result.Limit)
		}

		if result.Offset != 0 {
			t.Errorf("Expected offset 0, got %d", result.Offset)
		}
	})

	// Тест добавления торрента с невалидным magnet URI
	t.Run("add torrent with invalid magnet", func(t *testing.T) {
		body := `{"magnetUri":"invalid-magnet"}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/torrents", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

// TestIntegrationRoomOperations тестирует операции с комнатами
func TestIntegrationRoomOperations(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	token := getIntegrationTestAuthToken(t, server)

	var roomID string

	// Тест создания комнаты
	t.Run("create room", func(t *testing.T) {
		body := `{"name":"Test Room","password":""}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/rooms", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		var room models.RoomInfo
		if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if room.ID == "" {
			t.Error("Expected non-empty room ID")
		}

		if room.Name != "Test Room" {
			t.Errorf("Expected room name 'Test Room', got '%s'", room.Name)
		}

		roomID = room.ID
	})

	// Тест создания комнаты с невалидным именем
	t.Run("create room with invalid name", func(t *testing.T) {
		body := `{"name":""}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/rooms", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// Тест присоединения к комнате
	t.Run("join room", func(t *testing.T) {
		body := fmt.Sprintf(`{"roomId":"%s","password":""}`, roomID)
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/rooms/join", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Тест выхода из комнаты
	t.Run("leave room", func(t *testing.T) {
		body := `{}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/rooms/leave", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

// TestIntegrationSyncOperations тестирует операции синхронизации
func TestIntegrationSyncOperations(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	token := getIntegrationTestAuthToken(t, server)

	// Тест получения статуса синхронизации
	t.Run("get sync status", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/sync/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var status models.SyncStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
	})

	// Тест воспроизведения
	t.Run("sync play", func(t *testing.T) {
		body := `{}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/sync/play", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var status models.SyncStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !status.IsPlaying {
			t.Error("Expected IsPlaying to be true")
		}
	})

	// Тест перемотки
	t.Run("sync seek", func(t *testing.T) {
		body := `{"position":120.5}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/sync/seek", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var status models.SyncStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if status.Position != 120.5 {
			t.Errorf("Expected position 120.5, got %f", status.Position)
		}
	})

	// Тест перемотки с невалидной позицией
	t.Run("sync seek with invalid position", func(t *testing.T) {
		body := `{"position":-10}`
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/sync/seek", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

// TestIntegrationSecurityHeaders тестирует наличие заголовков безопасности
func TestIntegrationSecurityHeaders(t *testing.T) {
	server, _ := setupIntegrationTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
		"Content-Security-Policy",
	}

	for _, header := range headers {
		value := resp.Header.Get(header)
		if value == "" {
			t.Errorf("Expected header %s to be set", header)
		}
	}
}

// TestIntegrationPaginationParams тестирует параметры пагинации
func TestIntegrationPaginationParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "default params",
			query:      "",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "custom params",
			query:      "?limit=10&offset=5",
			wantLimit:  10,
			wantOffset: 5,
		},
		{
			name:       "limit exceeds max",
			query:      "?limit=200",
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "negative values",
			query:      "?limit=-1&offset=-5",
			wantLimit:  20,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com"+tt.query, nil)
			limit, offset := parsePaginationParams(req)

			if limit != tt.wantLimit {
				t.Errorf("Expected limit %d, got %d", tt.wantLimit, limit)
			}

			if offset != tt.wantOffset {
				t.Errorf("Expected offset %d, got %d", tt.wantOffset, offset)
			}
		})
	}
}

// Вспомогательные функции для тестов

// makeIntegrationRequest выполняет HTTP запрос с аутентификацией
func makeIntegrationRequest(t *testing.T, server *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()

	var req *http.Request
	var err error

	if body != "" {
		req, err = http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
	} else {
		req, err = http.NewRequest(method, server.URL+path, nil)
	}

	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	return resp
}

// decodeIntegrationJSON декодирует JSON ответ
func decodeIntegrationJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
}
