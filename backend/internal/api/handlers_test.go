package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	stdsync "sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	syncsvc "github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Глобальные сервисы для API-тестов
var (
	apiP2pSvc  *p2p.Service
	apiSyncSvc *syncsvc.Service
	apiRouter  http.Handler
	apiAuthSvc *auth.AuthService

	torrentOnce    stdsync.Once
	apiTorrentSvc  *torrent.Service
	torrentInitErr error
)

func initTorrentService() {
	torrentOnce.Do(func() {
		// Создаём сервис буферизации для тестов
		bufferSvc := buffer.NewService(64 * 1024 * 1024) // 64 МБ для тестов
		// Используем ListenPort: 0 для динамического выбора свободного порта
		svc, err := torrent.NewServiceWithOptions(bufferSvc, torrent.ServiceOptions{
			NoDHT:      true,
			DisableUTP: true,
			DisableTCP: true,
			ListenPort: 0,
		})
		if err != nil {
			torrentInitErr = err
			return
		}
		apiTorrentSvc = svc
	})
}

// applyAuthHeaders attaches the headers TokenMiddleware expects: the
// per-process access token plus a client id identifying this caller.
func applyAuthHeaders(req *http.Request) {
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "test-client")
}

// applyAuthContext sets the client ID in the request context as TokenMiddleware would.
func applyAuthContext(req *http.Request) {
	const clientIDKey = "torrsyncplayer.clientID"
	ctx := context.WithValue(req.Context(), clientIDKey, "test-client")
	*req = *req.WithContext(ctx)
}

// applyAuthHeadersAs attaches auth headers for a specific client id. Needed for
// multi-peer tests: the shared access token is per-process, so peers differ only
// by X-Client-ID — which is exactly what the host model and the room
// membership check are keyed on.
func applyAuthHeadersAs(req *http.Request, clientID string) {
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, clientID)
}

// TestMain создаёт общие сервисы для всех API-тестов
func TestMain(m *testing.M) {
	// Increase rate limits for tests to avoid 429 errors
	os.Setenv(constants.EnvAPIRateLimit, "100")
	os.Setenv(constants.EnvAPIRateBurst, "1000")

	// Инициализируем логгер до создания сервисов
	logger.Init("error", "text")

	// Recreate global rate limiter with test values
	ResetGlobalRateLimiter()

	authService, err := auth.NewAuthService()
	if err != nil {
		panic(err)
	}
	apiAuthSvc = authService
	p2pSvc, err := p2p.NewService(authService)
	if err != nil {
		panic(err)
	}
	apiP2pSvc = p2pSvc

	apiSyncSvc = syncsvc.NewService(apiP2pSvc)
	initTorrentService()
	if torrentInitErr != nil {
		panic(torrentInitErr)
	}

	apiRouter = NewRouter(RouterConfig{
		TorrentSvc:  apiTorrentSvc,
		P2pSvc:      apiP2pSvc,
		SyncSvc:     apiSyncSvc,
		AuthService: authService,
	})

	code := m.Run()

	_ = apiTorrentSvc.Close()
	_ = p2pSvc.Close()
	apiSyncSvc.Close()
	os.Exit(code)
}

func parseJSON(t *testing.T, body *bytes.Buffer, v interface{}) {
	t.Helper()
	err := json.Unmarshal(body.Bytes(), &v)
	require.NoError(t, err)
}

// ============ Health Check Tests ============

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	parseJSON(t, rec.Body, &response)
	assert.Equal(t, "ok", response["status"])
}

// ============ Torrent Handler Tests ============

// TestAddTorrent_InvalidJSON проверяет обработку невалидного JSON
func TestAddTorrent_InvalidJSON(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]interface{}
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "Invalid request format", errResp["error"])
}

// TestAddTorrent_InvalidMagnetURI проверяет валидацию magnet URI
func TestAddTorrent_InvalidMagnetURI(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	tests := []struct {
		name      string
		magnetURI string
	}{
		{"empty", ""},
		{"plain text", "not-a-magnet-link"},
		{"partial magnet", "magnet:?xt="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"magnetUri":"` + tt.magnetURI + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestAddTorrent_Timeout проверяет таймаут при получении метаданных
func TestAddTorrent_Timeout(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	body := `{"magnetUri":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	// Устанавливаем короткий таймаут на контекст запроса
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Ожидаем 500 (таймаут получения метаданных) или 201 (если успели)
	assert.Contains(t, []int{http.StatusCreated, http.StatusInternalServerError}, rec.Code)
}

func TestListTorrents(t *testing.T) {
	handler := ListTorrents(apiTorrentSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Теперь ответ содержит пагинированную структуру
	var response models.TorrentListResponse
	parseJSON(t, rec.Body, &response)
	assert.NotNil(t, response.Torrents)
	assert.GreaterOrEqual(t, response.Limit, 0)
	assert.GreaterOrEqual(t, response.Offset, 0)
}

func TestRemoveTorrent_NotFound(t *testing.T) {
	handler := RemoveTorrent(apiTorrentSvc)

	r := chi.NewRouter()
	r.Delete("/torrents/{id}", handler)

	// Используем валидный формат torrentID (40 hex символов)
	req := httptest.NewRequest(http.MethodDelete, "/torrents/0123456789abcdef0123456789abcdef01234567", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRemoveTorrent_MissingID(t *testing.T) {
	handler := RemoveTorrent(apiTorrentSvc)

	r := chi.NewRouter()
	r.Delete("/torrents/", handler)

	req := httptest.NewRequest(http.MethodDelete, "/torrents/", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetFiles_NotFound(t *testing.T) {
	handler := GetFiles(apiTorrentSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/files", handler)

	// Используем валидный формат torrentID (40 hex символов)
	req := httptest.NewRequest(http.MethodGet, "/torrents/0123456789abcdef0123456789abcdef01234567/files", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSelectFile_InvalidJSON(t *testing.T) {
	handler := SelectFile(apiTorrentSvc)

	r := chi.NewRouter()
	r.Post("/torrents/{id}/select", handler)

	// Используем валидный формат torrentID (40 hex символов)
	req := httptest.NewRequest(http.MethodPost, "/torrents/0123456789abcdef0123456789abcdef01234567/select", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStreamFile_MissingID(t *testing.T) {
	handler := StreamFile(apiTorrentSvc, apiAuthSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/stream", handler)

	req := httptest.NewRequest(http.MethodGet, "/torrents//stream", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStreamFile_TicketAuth(t *testing.T) {
	const torrentID = "0123456789abcdef0123456789abcdef01234567"
	handler := StreamFile(apiTorrentSvc, apiAuthSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/stream", handler)

	// No ticket → unauthorized.
	req := httptest.NewRequest(http.MethodGet, "/torrents/"+torrentID+"/stream", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Valid ticket → authorized (stream itself may 404 if torrent not present,
	// but it must not be rejected for auth reasons).
	ticket, err := apiAuthSvc.GenerateStreamTicket("user-1", torrentID)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodGet,
		"/torrents/"+torrentID+"/stream?ticket="+ticket, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)

	// Tampered ticket → unauthorized.
	req = httptest.NewRequest(http.MethodGet,
		"/torrents/"+torrentID+"/stream?ticket="+ticket+"x", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStreamTicket_IssueAndVerify(t *testing.T) {
	const torrentID = "0123456789abcdef0123456789abcdef01234567"
	ticket, err := apiAuthSvc.GenerateStreamTicket("user-1", torrentID)
	require.NoError(t, err)

	userID, ok := apiAuthSvc.ValidateStreamTicket(ticket, torrentID)
	assert.True(t, ok)
	assert.Equal(t, "user-1", userID)

	// Wrong torrent → rejected.
	_, ok = apiAuthSvc.ValidateStreamTicket(ticket, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	assert.False(t, ok)

	// Empty ticket → rejected.
	_, ok = apiAuthSvc.ValidateStreamTicket("", torrentID)
	assert.False(t, ok)
}

func TestStreamTicket_Contract_IssueThenStream(t *testing.T) {
	const torrentID = "0123456789abcdef0123456789abcdef01234567"

	// 1. Issue a stream ticket via the authenticated endpoint.
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/torrents/"+torrentID+"/stream-ticket", nil)
	applyAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "issue ticket must return 200")

	var ticketResp models.StreamTicketResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ticketResp))
	assert.NotEmpty(t, ticketResp.Ticket)
	assert.Equal(t, torrentID, ticketResp.TorrentID)

	// 2. Use the ticket to access the public stream endpoint.
	streamURL := "/api/v1/torrents/" + torrentID + "/stream?ticket=" + ticketResp.Ticket
	req = httptest.NewRequest(http.MethodGet, streamURL, nil)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "valid ticket must not return 401")

	// 3. Tampered ticket must be rejected.
	req = httptest.NewRequest(http.MethodGet, streamURL+"x", nil)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 4. Ticket for wrong torrent must be rejected.
	wrongTicket, err := apiAuthSvc.GenerateStreamTicket("user-1", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/torrents/"+torrentID+"/stream?ticket="+wrongTicket, nil)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ============ P2P Handler Tests ============

func TestCreateRoom_Success(t *testing.T) {
	handler := CreateRoom(apiP2pSvc, apiTorrentSvc)

	body := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)
	assert.NotEmpty(t, room.ID)
	assert.Equal(t, "Test Room", room.Name)
}

func TestCreateRoom_WithPassword(t *testing.T) {
	handler := CreateRoom(apiP2pSvc, apiTorrentSvc)

	body := `{"name":"Private Room","password":"secret123","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)
	assert.Equal(t, "Private Room", room.Name)
}

func TestCreateRoom_InvalidJSON(t *testing.T) {
	handler := CreateRoom(apiP2pSvc, apiTorrentSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJoinRoom_Success(t *testing.T) {
	room, err := apiP2pSvc.CreateRoom(context.Background(), "test-user-1", "Test Room", "", "torrent-test")
	require.NoError(t, err)

	handler := JoinRoom(apiP2pSvc, apiTorrentSvc)

	body := `{"roomId":"` + room.ID + `","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.SuccessResponse
	parseJSON(t, rec.Body, &response)
	assert.Equal(t, "Joined the room", response.Message)
}

func TestJoinRoom_NotFound(t *testing.T) {
	handler := JoinRoom(apiP2pSvc, apiTorrentSvc)

	body := `{"roomId":"nonexistent","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJoinRoom_InvalidJSON(t *testing.T) {
	handler := JoinRoom(apiP2pSvc, apiTorrentSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLeaveRoom_Success(t *testing.T) {
	room, err := apiP2pSvc.CreateRoom(context.Background(), "test-user-1", "Test Room", "", "torrent-test")
	require.NoError(t, err)
	err = apiP2pSvc.JoinRoom(context.Background(), "test-user-1", room.ID, "", "torrent-test")
	require.NoError(t, err)

	handler := LeaveRoom(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/leave", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.SuccessResponse
	parseJSON(t, rec.Body, &response)
	assert.Equal(t, "Left the room", response.Message)
}

func TestLeaveRoom_NotJoined(t *testing.T) {
	handler := LeaveRoom(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/leave", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignal_NotJoined(t *testing.T) {
	handler := Signal(apiP2pSvc)

	body := `{"roomId":"test","signal":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/signal", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignal_InvalidJSON(t *testing.T) {
	handler := Signal(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/signal", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ============ Sync Handler Tests ============

func TestSyncPlay(t *testing.T) {
	// Create a room via router (goes through TokenMiddleware)
	createBody := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	// Join the room
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Call sync/play via router
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.True(t, status.IsPlaying)
}

func TestSyncPause(t *testing.T) {
	// Create a room via router
	createBody := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	// Join the room
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Start playback
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Call sync/pause via router
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/pause", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.False(t, status.IsPlaying)
}

func TestSyncSeek_Success(t *testing.T) {
	// Create a room via router
	createBody := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	// Join the room
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Call sync/seek via router
	body := `{"position":1.5}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	// A small (within MaxPositionJump) seek is applied directly.
	assert.Equal(t, 1.5, status.Position)
}

func TestSyncSeek_InvalidJSON(t *testing.T) {
	handler := SyncSeek(apiSyncSvc, apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSyncHostEnforcement(t *testing.T) {
	// Create a room as test-client (host)
	createBody := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	// Join as host
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Host can play
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Now create a second user and join the room
	secondAuthSvc, err := auth.NewAuthServiceWithToken(apiAuthSvc.AccessToken())
	require.NoError(t, err)
	_ = secondAuthSvc.AccessToken()

	secondJoinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(secondJoinBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "second-client")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Second client (non-host) tries to play - should be forbidden
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "second-client")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Second client tries to pause - should be forbidden
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/pause", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "second-client")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Second client tries to seek - should be forbidden
	body := `{"position":1.5}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "second-client")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSyncSeek_InvalidPosition(t *testing.T) {
	handler := SyncSeek(apiSyncSvc, apiP2pSvc)

	body := `{"position":-10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestSyncBuffering tests the buffering endpoint
func TestSyncBuffering(t *testing.T) {
	// Create a room via router
	createBody := `{"name":"Test Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	// Join the room
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Start playback first
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Set buffering to true
	body := `{"buffering":true}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.True(t, status.Buffering)

	// Set buffering to false
	body = `{"buffering":false}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	parseJSON(t, rec.Body, &status)
	assert.False(t, status.Buffering)
}

// TestSyncBuffering_NonHostMember закрепляет решение idea-0018: буферизацию
// сообщает любой участник комнаты, а не только хост.
//
// Проверка на границе: host-чекинг снят, но членство в комнате осталось.
// Не-буферизующий не-хост обязан иметь право сказать «у меня всё в порядке» —
// иначе после восстановления его потока комната осталась бы на паузе
// навсегда, потому что снять паузу он не может.
func TestSyncBuffering_NonHostMember(t *testing.T) {
	// Хост создаёт комнату и входит в неё.
	createBody := `{"name":"Buffering Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	parseJSON(t, rec.Body, &room)

	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, room.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Не-хост присоединяется к той же комнате.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "buffering-peer")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Не-хост сообщает о своей буферизации — 200, не 403.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering",
		bytes.NewBufferString(`{"buffering":true}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "buffering-peer")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code,
		"не-хост в комнате должен иметь право сообщить о буферизации (idea-0018)")

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.True(t, status.Buffering)

	// И снять её тоже — иначе комната залипнет на паузе.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering",
		bytes.NewBufferString(`{"buffering":false}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "buffering-peer")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	parseJSON(t, rec.Body, &status)
	assert.False(t, status.Buffering)
}

// TestSyncBuffering_NotInRoom: снятие host-чекинга не должно превращать
// эндпоинт в анонимный. Участник не в комнате получает 400 «Not connected to
// a room» — проверка членства остаётся в GetRoomInfo до вызова сервиса, и
// ответ должен быть 400, а не 403.
func TestSyncBuffering_NotInRoom(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering",
		bytes.NewBufferString(`{"buffering":true}`))
	req.Header.Set("Content-Type", "application/json")
	// Свежий client id, который не создавал и не входил ни в одну комнату.
	applyAuthHeadersAs(req, "outside-peer")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"участник не в комнате должен получать 400, а не 403")

	var errResp map[string]interface{}
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "Not connected to a room", errResp["error"])
}

// TestSyncStatus_NotInRoom: вызывающий не в комнате получает 400, а не 200 с
// нулевым статусом (bug-0019).
//
// Раньше ошибка GetRoomInfo глушилась, roomID оставался "", и GetStatus("")
// отдавал валидный на вид IsPlaying:false / Position:0. Клиент после
// реконнекта (bug-0012) не мог отличить «комнаты нет» от «комната на нуле» —
// и ставил паузу с прыжком на позицию 0 там, где комната играла с 42-й минуты.
func TestSyncStatus_NotInRoom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil)
	applyAuthHeadersAs(req, "status-outsider")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]interface{}
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "Not connected to a room", errResp["error"],
		"сообщение должно совпадать с остальными sync-хендлерами")
}

// TestSyncStatus_InRoom: в комнате — 200 с настоящими значениями комнаты.
// Проверяет, что 400 не превратился в «эндпоинт сломан».
func TestSyncStatus_InRoom(t *testing.T) {
	createBody := `{"name":"Status Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created models.RoomInfo
	parseJSON(t, rec.Body, &created)

	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, created.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Хост задаёт состояние, которое обязан увидеть статус.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek",
		bytes.NewBufferString(`{"position":1.5}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil)
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.True(t, status.IsPlaying)
	// 1.5 — внутри MaxPositionJump, поэтому перемотка применяется напрямую
	// (а не сглаживается, как прыжок за 2 s). Тест про статус, а не про
	// сглаживание, поэтому позицию выбираем однозначную. Главное — она
	// ненулевая: именно нули выдавал молчаливый 200 без комнаты.
	assert.InDelta(t, 1.5, status.Position, 0.01,
		"статус должен отдавать позицию комнаты, а не нули")
}

// ============ Router Integration Tests ============

// TestRoomInfo covers GET /api/v1/rooms/info — the endpoint bug-0012 needs so a
// client can re-learn who the host is and how many peers are in the room after
// an SSE reconnect.
//
// The path deliberately carries no {roomID}: the room comes from the caller's
// own membership, which is what keeps one client from asking about another's
// room.
func TestRoomInfo(t *testing.T) {
	createBody := `{"name":"Info Room","password":"","torrentId":"torrent-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created models.RoomInfo
	parseJSON(t, rec.Body, &created)
	require.NotEmpty(t, created.ID)

	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, created.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// A second peer joins so peerCount is distinguishable from 1.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "info-peer")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// The host asks about its own room.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var info models.RoomInfo
	parseJSON(t, rec.Body, &info)
	assert.Equal(t, created.ID, info.ID)
	assert.Equal(t, "Info Room", info.Name)
	assert.Equal(t, "test-client", info.HostUserID, "хост должен быть виден в ответе")
	assert.Equal(t, 2, info.PeerCount)
}

// TestRoomInfo_NotInRoom: without a room there is nothing to report, and the
// client must be able to tell that apart from a real room.
func TestRoomInfo_NotInRoom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "info-outsider")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]interface{}
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "Not connected to a room", errResp["error"])
}

// TestRoomInfo_CountsHostAndSurvivesLastGuestLeave walks the whole thing through
// the real router, because the bug was only visible end to end (bug-0021).
//
// The host creates the room and never joins — which is exactly what the client
// does — so it used to sit outside the roster. Two consequences, both fixed
// here: every count was one short, and the room was deleted the moment the last
// guest left, out from under a host who was still connected and watching.
func TestRoomInfo_CountsHostAndSurvivesLastGuestLeave(t *testing.T) {
	// 1. Host creates the room. It counts as one participant already.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(`{"name":"Counting Room","torrentId":"torrent-test"}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "e2e-host")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created models.RoomInfo
	parseJSON(t, rec.Body, &created)
	assert.Equal(t, 1, created.PeerCount, "the creator is a participant")

	// 2. A guest joins.
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, created.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "e2e-guest")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 3. Both are counted.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "e2e-host")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var info models.RoomInfo
	parseJSON(t, rec.Body, &info)
	assert.Equal(t, 2, info.PeerCount, "host + guest")
	assert.Equal(t, "e2e-host", info.HostUserID)

	// 4. The guest leaves. The room must survive: the host is still in it.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/leave", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "e2e-guest")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 5. The host is still a member and still counted.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "e2e-host")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code,
		"the room was deleted although the host is still connected")

	parseJSON(t, rec.Body, &info)
	assert.Equal(t, 1, info.PeerCount, "only the host remains")

	// 6. And the host can still drive playback.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "e2e-host")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRoomInfo_RosterAndBuffering covers idea-0017 end to end: the roster in
// /rooms/info, and the roster naming which peer is buffering.
//
// The client that arrives second is the interesting one — it never saw a
// peer_joined for anyone, so if it can still find the first peer and see that
// peer buffering, the snapshot really does stand in for the event stream.
func TestRoomInfo_RosterAndBuffering(t *testing.T) {
	// First client creates and enters the room.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString(`{"name":"Roster Room","torrentId":"torrent-test"}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "roster-first")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created models.RoomInfo
	parseJSON(t, rec.Body, &created)

	// Second client joins.
	joinBody := fmt.Sprintf(`{"roomId":"%s","password":"","torrentId":"torrent-test"}`, created.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(joinBody))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "roster-second")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// The late client's first look: it sees the host, which it never saw join.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "roster-second")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var info models.RoomInfo
	parseJSON(t, rec.Body, &info)
	require.Len(t, info.Peers, 2)
	assert.Equal(t, 2, info.PeerCount)
	assert.Equal(t, info.PeerCount, len(info.Peers),
		"the count and the roster disagree")

	first, ok := findPeerByUserID(info.Peers, "roster-first")
	require.True(t, ok, "the late client cannot see the peer that was already there")
	assert.True(t, first.IsHost, "the host is not marked isHost")
	assert.False(t, first.Buffering)

	// That first peer now reports buffering.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering",
		bytes.NewBufferString(`{"buffering":true}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "roster-first")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// The roster must name that peer, not merely say the room is buffering.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "roster-second")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	parseJSON(t, rec.Body, &info)
	require.Len(t, info.Peers, 2)

	first, ok = findPeerByUserID(info.Peers, "roster-first")
	require.True(t, ok, "the peer that reported buffering vanished from the roster")
	assert.True(t, first.Buffering, "the roster does not say which peer is buffering")

	second, ok := findPeerByUserID(info.Peers, "roster-second")
	require.True(t, ok)
	assert.False(t, second.Buffering, "an innocent peer was marked as buffering")

	// And catching up clears it.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sync/buffering",
		bytes.NewBufferString(`{"buffering":false}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, "roster-first")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/info", nil)
	applyAuthHeadersAs(req, "roster-second")
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	parseJSON(t, rec.Body, &info)
	first, ok = findPeerByUserID(info.Peers, "roster-first")
	require.True(t, ok)
	assert.False(t, first.Buffering, "the peer caught up but still shows buffering")
}

// findPeerByUserID locates a roster entry by its user ID.
func findPeerByUserID(roster []models.PeerInfo, userID string) (models.PeerInfo, bool) {
	for _, p := range roster {
		if p.UserID == userID {
			return p, true
		}
	}
	return models.PeerInfo{}, false
}

func TestRouter_HealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_CORS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:8889")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "http://localhost:8889", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestRouter_CORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/torrents", nil)
	req.Header.Set("Origin", "http://localhost:8889")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ============ Middleware Tests ============

func TestRecoveryMiddleware(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Recovery)
	r.Get("/panic", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	WriteJSON(rec, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var result map[string]string
	parseJSON(t, rec.Body, &result)
	assert.Equal(t, "value", result["key"])
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "test error")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "test error", errResp["error"])
}

func TestWriteErrorResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusNotFound, models.ErrorResponse{Error: "not found"})

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp models.ErrorResponse
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "not found", errResp.Error)
}

func TestWriteError_JSONStructure(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "bad request")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "bad request", errResp["error"])
}

// ============ ValidateMagnetURI Tests ============

// TestValidateMagnetURI проверяет валидацию magnet URI
func TestValidateMagnetURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"valid btih magnet", "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", false},
		{"empty string", "", true},
		{"plain text", "not a magnet link", true},
		{"partial magnet", "magnet:?xt=", true},
		{"magnet with short hash", "magnet:?xt=urn:btih:abc123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateMagnetURI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTorrentFile проверяет валидацию base64 торрент-файла
func TestValidateTorrentFile(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"valid empty", "", true}, // empty is invalid
		{"invalid base64", "not-base64!@#$", true},
		{"valid base64 but empty decoded", "Zg==", true}, // "f" as base64, but torrent parsing will fail later
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateTorrentFile(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============ Full Integration Flow Tests ============

func TestFullTorrentFlow(t *testing.T) {

	// 1. Получаем список торрентов (с токеном) - теперь с пагинацией
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	applyAuthHeaders(req)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.TorrentListResponse
	parseJSON(t, rec.Body, &response)
	assert.NotNil(t, response.Torrents)

	// 2. Пытаемся добавить невалидный magnet URI
	body := `{"magnetUri":"invalid"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Пытаемся удалить несуществующий торрент (используем валидный формат ID)
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567", nil)
	applyAuthHeaders(req)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestContextCancellation(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	body := `{"magnetUri":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Contains(t, []int{http.StatusCreated, http.StatusInternalServerError}, rec.Code)
}

func TestConcurrentRequests(t *testing.T) {
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			apiRouter.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}()
	}

	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent requests")
		}
	}
}

// ============ TorrentID Validation Tests ============

func TestValidateTorrentID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex id", "0123456789abcdef0123456789abcdef01234567", false},
		{"valid uppercase", "0123456789ABCDEF0123456789ABCDEF01234567", false},
		{"empty id", "", true},
		{"too short", "abc123", true},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"invalid chars", "xyz123456789abcdef0123456789abcdef0123456", true},
		{"special chars", "0123456789abcdef0123456789abcdef0123456!", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateTorrentID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveTorrent_InvalidID(t *testing.T) {
	handler := RemoveTorrent(apiTorrentSvc)

	r := chi.NewRouter()
	r.Delete("/torrents/{id}", handler)

	// Невалидный ID (слишком короткий)
	req := httptest.NewRequest(http.MethodDelete, "/torrents/invalid", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	// Ожидаем 400 (невалидный ID) или 404 (не найден)
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, rec.Code)
}

func TestGetFiles_InvalidID(t *testing.T) {
	handler := GetFiles(apiTorrentSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/files", handler)

	// Невалидный ID
	req := httptest.NewRequest(http.MethodGet, "/torrents/invalid-id/files", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, rec.Code)
}

// TestRoomEvents_WithRoomID проверяет SSE endpoint для событий комнаты с параметром roomID в URL
func TestRoomEvents_WithRoomID(t *testing.T) {
	roomInfo, err := apiP2pSvc.CreateRoom(context.Background(), "host-user", "test-room", "", "torrent-test")
	require.NoError(t, err)
	roomID := roomInfo.ID

	// Join the room to get access to events
	err = apiP2pSvc.JoinRoom(context.Background(), "test-user", roomID, "", "torrent-test")
	require.NoError(t, err)
	handler := RoomEvents(apiP2pSvc)

	r := chi.NewRouter()
	r.Get("/rooms/{roomID}/events", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/rooms/"+roomID+"/events", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ============ Security Tests ============

func TestSecurity_SecurityHeaders(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'self'")
}

func TestSecurity_ContentType_RejectsNonJSON(t *testing.T) {
	handler := ContentTypeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestSecurity_ContentType_AcceptsJSON(t *testing.T) {
	handler := ContentTypeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSecurity_RateLimiter_BlocksExcess(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(0.01), 1)
	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should succeed
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "First request should succeed")

	// Subsequent requests should be rate limited (429 Too Many Requests)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Request %d should be rate limited", i+2)
	}
}

func TestSecurity_Router_HealthNoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSecurity_Router_ProtectedEndpointRejectsNoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSecurity_Pagination_MaxLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents?limit=999999", nil)
	limit, _ := parsePaginationParams(req)
	assert.LessOrEqual(t, limit, constants.MaxPaginationLimit)
}

func TestSecurity_Pagination_MaxOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents?offset=999999999", nil)
	_, offset := parsePaginationParams(req)
	assert.LessOrEqual(t, offset, constants.MaxPaginationOffset)
}

// mockP2PServiceForRoomEvents is a minimal mock for testing RoomEvents.
type mockP2PServiceForRoomEvents struct{}

func (m *mockP2PServiceForRoomEvents) CreateRoom(ctx context.Context, userID, name, password, torrentID string) (*models.RoomInfo, error) {
	return nil, nil
}
func (m *mockP2PServiceForRoomEvents) JoinRoom(ctx context.Context, userID, roomID, password, torrentID string) error {
	return nil
}
func (m *mockP2PServiceForRoomEvents) LeaveRoom(ctx context.Context, userID string) error {
	return nil
}
func (m *mockP2PServiceForRoomEvents) SendSignal(ctx context.Context, userID string, signal []byte) error {
	return nil
}
func (m *mockP2PServiceForRoomEvents) GetEvents(userID string) chan models.P2PEvent {
	return nil
}
func (m *mockP2PServiceForRoomEvents) GetRoomInfo(ctx context.Context, userID string) (*models.RoomInfo, error) {
	return &models.RoomInfo{ID: "0123456789abcdef0123456789abcdef", Name: "test", HostID: "host", HostUserID: "host-user", PeerCount: 1}, nil
}
func (m *mockP2PServiceForRoomEvents) IsHost(ctx context.Context, userID, roomID string) (bool, error) {
	return userID == "host-user", nil
}
func (m *mockP2PServiceForRoomEvents) BroadcastSync(roomID string, event models.SyncEvent) {}
func (m *mockP2PServiceForRoomEvents) TouchPeer(_ context.Context, _ string) error         { return nil }
func (m *mockP2PServiceForRoomEvents) TorrentsVisibleTo(_ context.Context, userID string) map[string]bool {
	return map[string]bool{}
}
func (m *mockP2PServiceForRoomEvents) SetPeerBuffering(_ context.Context, _ string, _ bool) {}
func (m *mockP2PServiceForRoomEvents) BroadcastPeerList(_ string)                           {}
func (m *mockP2PServiceForRoomEvents) BroadcastTorrent(_, _, _, _ string, _ int)            {}
func (m *mockP2PServiceForRoomEvents) Close() error                                         { return nil }

// TestRoomEvents_NilEventChannel verifies that a nil event channel returns 500
// instead of the old 503 "not implemented" message.
func TestRoomEvents_NilEventChannel(t *testing.T) {
	handler := RoomEvents(&mockP2PServiceForRoomEvents{})

	r := chi.NewRouter()
	r.Get("/rooms/{roomID}/events", handler)

	req := httptest.NewRequest(http.MethodGet, "/rooms/0123456789abcdef0123456789abcdef/events", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Event channel not available")
}
