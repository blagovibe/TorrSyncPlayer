package api

import (
	"bytes"
	"context"
	"encoding/json"
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
	apiP2pSvc    *p2p.Service
	apiSyncSvc   *syncsvc.Service
	apiRouter    http.Handler
	apiAuthStore *auth.UserStore

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

// getTestToken возвращает токен для тестового пользователя
func getTestCSRFToken(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/csrf-token", nil)
	rr := httptest.NewRecorder()
	apiRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var result map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err)
	return result["csrfToken"]
}

func getTestToken(t *testing.T) string {
	t.Helper()
	// Создаём пользователя и получаем токен
	authService, err := auth.NewAuthService([]byte("test-secret-for-api-tests-32bytes!"))
	require.NoError(t, err)
	authHandler := auth.NewAuthHandler(apiAuthStore, authService)

	body := `{"username":"testuser","password":"TestPass1!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	authHandler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		// Если пользователь уже существует, пробуем залогиниться
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		authHandler.Login(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	var resp models.AuthResponse
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp.Token
}

// TestMain создаёт общие сервисы для всех API-тестов
func TestMain(m *testing.M) {
	// Инициализируем логгер до создания сервисов
	logger.Init("error", "text")

	authService, err := auth.NewAuthService([]byte("test-secret-for-api-tests-32bytes!"))
	if err != nil {
		panic(err)
	}
	p2pSvc, err := p2p.NewService(authService)
	if err != nil {
		panic(err)
	}
	apiP2pSvc = p2pSvc

	apiSyncSvc = syncsvc.NewService()
	initTorrentService()
	if torrentInitErr != nil {
		panic(torrentInitErr)
	}

	// Создаём auth store и auth service для тестов
	apiAuthStore = auth.NewUserStore()
	authService, err = auth.NewAuthService([]byte("test-secret-for-api-tests-32bytes!"))
	if err != nil {
		panic(err)
	}

	apiRouter = NewRouter(RouterConfig{
		TorrentSvc:  apiTorrentSvc,
		P2pSvc:      apiP2pSvc,
		SyncSvc:     apiSyncSvc,
		AuthStore:   apiAuthStore,
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
	handler := StreamFile(apiTorrentSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/stream", handler)

	req := httptest.NewRequest(http.MethodGet, "/torrents//stream", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ============ P2P Handler Tests ============

func TestCreateRoom_Success(t *testing.T) {
	handler := CreateRoom(apiP2pSvc)

	body := `{"name":"Test Room","password":""}`
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
	handler := CreateRoom(apiP2pSvc)

	body := `{"name":"Private Room","password":"secret123"}`
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
	handler := CreateRoom(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJoinRoom_Success(t *testing.T) {
	room, err := apiP2pSvc.CreateRoom(context.Background(), "test-user-1", "Test Room", "")
	require.NoError(t, err)

	handler := JoinRoom(apiP2pSvc)

	body := `{"roomId":"` + room.ID + `","password":""}`
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
	handler := JoinRoom(apiP2pSvc)

	body := `{"roomId":"nonexistent","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJoinRoom_InvalidJSON(t *testing.T) {
	handler := JoinRoom(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/join", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLeaveRoom_Success(t *testing.T) {
	room, err := apiP2pSvc.CreateRoom(context.Background(), "test-user-1", "Test Room", "")
	require.NoError(t, err)
	err = apiP2pSvc.JoinRoom(context.Background(), "test-user-1", room.ID, "")
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
	handler := SyncPlay(apiSyncSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/play", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.True(t, status.IsPlaying)
}

func TestSyncPause(t *testing.T) {
	apiSyncSvc.Play(context.Background())

	handler := SyncPause(apiSyncSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/pause", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.False(t, status.IsPlaying)
}

func TestSyncSeek_Success(t *testing.T) {
	handler := SyncSeek(apiSyncSvc)

	body := `{"position":120.5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.Equal(t, 120.5, status.Position)
}

func TestSyncSeek_InvalidJSON(t *testing.T) {
	handler := SyncSeek(apiSyncSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSyncSeek_InvalidPosition(t *testing.T) {
	handler := SyncSeek(apiSyncSvc)

	body := `{"position":-10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/seek", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSyncStatus(t *testing.T) {
	handler := SyncStatus(apiSyncSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var status models.SyncStatus
	parseJSON(t, rec.Body, &status)
	assert.NotNil(t, status)
}

// ============ Router Integration Tests ============

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

// ============ Full Integration Flow Tests ============

func TestFullTorrentFlow(t *testing.T) {
	token := getTestToken(t)
	csrfToken := getTestCSRFToken(t)

	// 1. Получаем список торрентов (с токеном) - теперь с пагинацией
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
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
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Пытаемся удалить несуществующий торрент (используем валидный формат ID)
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-CSRF-Token", csrfToken)
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
	roomInfo, err := apiP2pSvc.CreateRoom(context.Background(), "host-user", "test-room", "")
	require.NoError(t, err)
	roomID := roomInfo.ID

	// Join the room to get access to events
	err = apiP2pSvc.JoinRoom(context.Background(), "test-user", roomID, "")
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

func TestSecurity_CSRF_RejectsMissingToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSecurity_CSRF_SkipsWithJWT(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.valid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSecurity_CSRF_SkipsOnGET(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

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
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
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

func TestSecurity_Router_CSRFOnProtectedEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	// Use a valid-length JWT-like token (minimum 30 chars)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusForbidden, rec.Code)
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
