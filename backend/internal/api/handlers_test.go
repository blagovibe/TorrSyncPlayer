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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	syncsvc "github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Р“Р»РѕР±Р°Р»СЊРЅС‹Рµ СЃРµСЂРІРёСЃС‹ РґР»СЏ API-С‚РµСЃС‚РѕРІ
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
		tmpDir, err := os.MkdirTemp("", "api-test-torrent-*")
		if err != nil {
			torrentInitErr = err
			return
		}
		svc, err := torrent.NewService(tmpDir)
		if err != nil {
			torrentInitErr = err
			os.RemoveAll(tmpDir)
			return
		}
		apiTorrentSvc = svc
	})
}

// getTestToken РІРѕР·РІСЂР°С‰Р°РµС‚ С‚РѕРєРµРЅ РґР»СЏ С‚РµСЃС‚РѕРІРѕРіРѕ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ
func getTestToken(t *testing.T) string {
	t.Helper()
	// РЎРѕР·РґР°С‘Рј РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ Рё РїРѕР»СѓС‡Р°РµРј С‚РѕРєРµРЅ
	authHandler := auth.NewAuthHandler(apiAuthStore, auth.NewAuthService([]byte("test-secret-for-api-tests-32bytes!")))

	body := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	authHandler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		// Р•СЃР»Рё РїРѕР»СЊР·РѕРІР°С‚РµР»СЊ СѓР¶Рµ СЃСѓС‰РµСЃС‚РІСѓРµС‚, РїСЂРѕР±СѓРµРј Р·Р°Р»РѕРіРёРЅРёС‚СЊСЃСЏ
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		authHandler.Login(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	var resp models.AuthResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp.Token
}

// TestMain СЃРѕР·РґР°С‘С‚ РѕР±С‰РёРµ СЃРµСЂРІРёСЃС‹ РґР»СЏ РІСЃРµС… API-С‚РµСЃС‚РѕРІ
func TestMain(m *testing.M) {
	// РРЅРёС†РёР°Р»РёР·РёСЂСѓРµРј Р»РѕРіРіРµСЂ РґРѕ СЃРѕР·РґР°РЅРёСЏ СЃРµСЂРІРёСЃРѕРІ
	logger.Init("error", "text")

	p2pSvc, err := p2p.NewService(auth.NewAuthService([]byte("test-secret-for-api-tests-32bytes!")))
	if err != nil {
		panic(err)
	}
	apiP2pSvc = p2pSvc

	apiSyncSvc = syncsvc.NewService()
	initTorrentService()
	if torrentInitErr != nil {
		panic(torrentInitErr)
	}

	// РЎРѕР·РґР°С‘Рј auth store РґР»СЏ С‚РµСЃС‚РѕРІ
	apiAuthStore = auth.NewUserStore()

	apiRouter = NewRouter(RouterConfig{
		TorrentSvc: apiTorrentSvc,
		P2pSvc:     apiP2pSvc,
		SyncSvc:    apiSyncSvc,
		AuthStore:  apiAuthStore,
	})

	code := m.Run()

	apiTorrentSvc.Close()
	p2pSvc.Close()
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

// TestAddTorrent_InvalidJSON РїСЂРѕРІРµСЂСЏРµС‚ РѕР±СЂР°Р±РѕС‚РєСѓ РЅРµРІР°Р»РёРґРЅРѕРіРѕ JSON
func TestAddTorrent_InvalidJSON(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var apiErr APIError
	parseJSON(t, rec.Body, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.Code)
}

// TestAddTorrent_InvalidMagnetURI РїСЂРѕРІРµСЂСЏРµС‚ РІР°Р»РёРґР°С†РёСЋ magnet URI
func TestAddTorrent_InvalidMagnetURI(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	tests := []struct {
		name      string
		magnetURI string
	}{
		{"empty", ""},
		{"plain text", "not-a-magnet-link"},
		{"partial magnet", "magnet:?xt=urn:btih:abc"},
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

// TestAddTorrent_Timeout РїСЂРѕРІРµСЂСЏРµС‚ С‚Р°Р№РјР°СѓС‚ РїСЂРё РїРѕР»СѓС‡РµРЅРёРё РјРµС‚Р°РґР°РЅРЅС‹С…
func TestAddTorrent_Timeout(t *testing.T) {
	handler := AddTorrent(apiTorrentSvc)

	body := `{"magnetUri":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	// РЈСЃС‚Р°РЅР°РІР»РёРІР°РµРј РєРѕСЂРѕС‚РєРёР№ С‚Р°Р№РјР°СѓС‚ РЅР° РєРѕРЅС‚РµРєСЃС‚ Р·Р°РїСЂРѕСЃР°
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// РћР¶РёРґР°РµРј 500 (С‚Р°Р№РјР°СѓС‚ РїРѕР»СѓС‡РµРЅРёСЏ РјРµС‚Р°РґР°РЅРЅС‹С…) РёР»Рё 201 (РµСЃР»Рё СѓСЃРїРµР»Рё)
	assert.Contains(t, []int{http.StatusCreated, http.StatusInternalServerError}, rec.Code)
}

func TestListTorrents(t *testing.T) {
	handler := ListTorrents(apiTorrentSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// РўРµРїРµСЂСЊ РѕС‚РІРµС‚ СЃРѕРґРµСЂР¶РёС‚ РїР°РіРёРЅРёСЂРѕРІР°РЅРЅСѓСЋ СЃС‚СЂСѓРєС‚СѓСЂСѓ
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

	// РСЃРїРѕР»СЊР·СѓРµРј РІР°Р»РёРґРЅС‹Р№ С„РѕСЂРјР°С‚ torrentID (40 hex СЃРёРјРІРѕР»РѕРІ)
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

	// РСЃРїРѕР»СЊР·СѓРµРј РІР°Р»РёРґРЅС‹Р№ С„РѕСЂРјР°С‚ torrentID (40 hex СЃРёРјРІРѕР»РѕРІ)
	req := httptest.NewRequest(http.MethodGet, "/torrents/0123456789abcdef0123456789abcdef01234567/files", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSelectFile_InvalidJSON(t *testing.T) {
	handler := SelectFile(apiTorrentSvc)

	r := chi.NewRouter()
	r.Post("/torrents/{id}/select", handler)

	// РСЃРїРѕР»СЊР·СѓРµРј РІР°Р»РёРґРЅС‹Р№ С„РѕСЂРјР°С‚ torrentID (40 hex СЃРёРјРІРѕР»РѕРІ)
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
	room, err := apiP2pSvc.CreateRoom("Test Room", "")
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
	assert.Equal(t, "РџСЂРёСЃРѕРµРґРёРЅРёР»РёСЃСЊ Рє РєРѕРјРЅР°С‚Рµ", response.Message)
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
	room, err := apiP2pSvc.CreateRoom("Test Room", "")
	require.NoError(t, err)
	err = apiP2pSvc.JoinRoom(room.ID, "")
	require.NoError(t, err)

	handler := LeaveRoom(apiP2pSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/leave", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.SuccessResponse
	parseJSON(t, rec.Body, &response)
	assert.Equal(t, "Р’С‹С€Р»Рё РёР· РєРѕРјРЅР°С‚С‹", response.Message)
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
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
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
	apiSyncSvc.Play()

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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
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
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var result map[string]string
	parseJSON(t, rec.Body, &result)
	assert.Equal(t, "value", result["key"])
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "test error")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var apiErr APIError
	parseJSON(t, rec.Body, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.Code)
	assert.Equal(t, "test error", apiErr.Message)
}

func TestWriteErrorResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusNotFound, models.ErrorResponse{Error: "not found"})

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp models.ErrorResponse
	parseJSON(t, rec.Body, &errResp)
	assert.Equal(t, "not found", errResp.Error)
}

func TestAPIError_Structure(t *testing.T) {
	apiErr := APIError{Code: 400, Message: "bad request"}

	data, jsonErr := json.Marshal(apiErr)
	assert.NoError(t, jsonErr)

	var parsed APIError
	jsonErr = json.Unmarshal(data, &parsed)
	assert.NoError(t, jsonErr)
	assert.Equal(t, apiErr.Code, parsed.Code)
	assert.Equal(t, apiErr.Message, parsed.Message)
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

	// 1. РџРѕР»СѓС‡Р°РµРј СЃРїРёСЃРѕРє С‚РѕСЂСЂРµРЅС‚РѕРІ (СЃ С‚РѕРєРµРЅРѕРј) - С‚РµРїРµСЂСЊ СЃ РїР°РіРёРЅР°С†РёРµР№
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response models.TorrentListResponse
	parseJSON(t, rec.Body, &response)
	assert.NotNil(t, response.Torrents)

	// 2. РџС‹С‚Р°РµРјСЃСЏ РґРѕР±Р°РІРёС‚СЊ РЅРµРІР°Р»РёРґРЅС‹Р№ magnet URI
	body := `{"magnetUri":"invalid"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/torrents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. РџС‹С‚Р°РµРјСЃСЏ СѓРґР°Р»РёС‚СЊ РЅРµСЃСѓС‰РµСЃС‚РІСѓСЋС‰РёР№ С‚РѕСЂСЂРµРЅС‚ (РёСЃРїРѕР»СЊР·СѓРµРј РІР°Р»РёРґРЅС‹Р№ С„РѕСЂРјР°С‚ ID)
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567", nil)
	req.Header.Set("Authorization", "Bearer "+token)
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

	// РќРµРІР°Р»РёРґРЅС‹Р№ ID (СЃР»РёС€РєРѕРј РєРѕСЂРѕС‚РєРёР№)
	req := httptest.NewRequest(http.MethodDelete, "/torrents/invalid", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	// РћР¶РёРґР°РµРј 400 (РЅРµРІР°Р»РёРґРЅС‹Р№ ID) РёР»Рё 404 (РЅРµ РЅР°Р№РґРµРЅ)
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, rec.Code)
}

func TestGetFiles_InvalidID(t *testing.T) {
	handler := GetFiles(apiTorrentSvc)

	r := chi.NewRouter()
	r.Get("/torrents/{id}/files", handler)

	// РќРµРІР°Р»РёРґРЅС‹Р№ ID
	req := httptest.NewRequest(http.MethodGet, "/torrents/invalid-id/files", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, rec.Code)
}

// TestRoomEvents_WithRoomID проверяет SSE endpoint для событий комнаты с параметром roomID в URL
func TestRoomEvents_WithRoomID(t *testing.T) {
handler := RoomEvents(apiP2pSvc)

r := chi.NewRouter()
r.Get("/rooms/{roomID}/events", handler)

// Используем валидный roomID (hex строка длиной 32 символа)
roomID := "0123456789abcdef0123456789abcdef"

// Создаём контекст с коротким таймаутом чтобы прервать SSE соединение
ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
defer cancel()

req := httptest.NewRequest(http.MethodGet, "/rooms/"+roomID+"/events", nil)
req = req.WithContext(ctx)
rec := httptest.NewRecorder()

handler.ServeHTTP(rec, req)

// Проверяем что ответ начался с SSE заголовков
assert.Equal(t, http.StatusOK, rec.Code)
assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))

// Проверяем что начальное событие connected было отправлено
body := rec.Body.String()
assert.Contains(t, body, "event: connected")
assert.Contains(t, body, "status")
}

