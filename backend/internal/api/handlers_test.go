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
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Глобальные сервисы для API-тестов
var (
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
	if err := os.Setenv(constants.EnvAPIRateLimit, "100"); err != nil {
		panic(err)
	}
	if err := os.Setenv(constants.EnvAPIRateBurst, "1000"); err != nil {
		panic(err)
	}

	// Инициализируем логгер до создания сервисов
	logger.Init("error", "text")

	// Recreate global rate limiter with test values
	ResetGlobalRateLimiter()

	authService, err := auth.NewAuthService()
	if err != nil {
		panic(err)
	}
	apiAuthSvc = authService
	initTorrentService()
	if torrentInitErr != nil {
		panic(torrentInitErr)
	}

	apiRouter = NewRouter(RouterConfig{
		TorrentSvc:  apiTorrentSvc,
		AuthService: authService,
	})

	code := m.Run()

	_ = apiTorrentSvc.Close()
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

	// Даже валидно выглядящий тикет в URL больше не авторизует: тикета нет,
	// и это именно то, чего мы добились — передаваемый bearer в адресной строке.
	req = httptest.NewRequest(http.MethodGet,
		"/torrents/"+torrentID+"/stream?ticket=anything", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

}

// ============ P2P Handler Tests ============

// Rooms and sync used to be brokered here; playback commands travel peer to
// peer now, and these flows have no server endpoint to test against (idea-0064).

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
