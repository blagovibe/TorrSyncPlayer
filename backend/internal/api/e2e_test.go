// Package api содержит E2E тесты для API endpoints.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
)

// setupTestServer создаёт тестовый сервер с реальными сервисами.
func setupTestServer(t *testing.T) (*httptest.Server, string, func()) {
	t.Helper()

	// Создаём сервис буферизации для тестов
	bufferSvc := buffer.NewService(64 * 1024 * 1024) // 64 МБ для тестов

	// Инициализация сервисов (всегда in-memory)
	// Используем ListenPort: 0 для динамического выбора свободного порта
	torrentSvc, err := torrent.NewServiceWithOptions(bufferSvc, torrent.ServiceOptions{
		NoDHT:      true,
		DisableUTP: true,
		DisableTCP: true,
		ListenPort: 0,
	})
	require.NoError(t, err)

	authService, err := auth.NewAuthService()
	require.NoError(t, err)
	p2pSvc, err := p2p.NewService(authService)
	require.NoError(t, err)

	syncSvc := sync.NewService()

	// Создаём роутер
	router := NewRouter(RouterConfig{
		TorrentSvc:  torrentSvc,
		P2pSvc:      p2pSvc,
		SyncSvc:     syncSvc,
		AuthService: authService,
	})

	// Создаём тестовый сервер
	// Each test gets a full request budget; see clientRateLimiter.resetAll.
	globalClientRateLimiter.resetAll()

	server := httptest.NewServer(router)

	// Функция очистки
	cleanup := func() {
		server.Close()
		_ = torrentSvc.Close()
		_ = p2pSvc.Close()
		syncSvc.Close()
	}

	// The access token is printed at startup in production; tests get it here so
	// they can send it the way a real client does.
	return server, authService.AccessToken(), cleanup
}

// authRequest builds a request carrying the headers TokenMiddleware expects.
func authRequest(t *testing.T, server *httptest.Server, token, method, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	return req
}

// TestE2E_HealthCheck проверяет health check endpoint.
func TestE2E_HealthCheck(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "ok", result["status"])
}

// TestE2E_Version проверяет version endpoint.
func TestE2E_Version(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/v1/version")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotNil(t, result["version"])
	assert.NotNil(t, result["commit"])
	assert.NotNil(t, result["build"])
	assert.NotNil(t, result["runtime"])
}

// TestE2E_Metrics проверяет metrics endpoint.
func TestE2E_Metrics(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

// TestE2E_AccessToken проверяет схему доступа: токен выдаётся при старте
// сервера, защищённые endpoint его требуют, а чужие и отсутствующие — нет.
func TestE2E_AccessToken(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	require.NotEmpty(t, token, "сервер должен выдать токен при старте")

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("с корректным токеном — доступ разрешён", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/torrents", nil)
		require.NoError(t, err)
		req.Header.Set(auth.HeaderAccessToken, token)
		req.Header.Set(auth.HeaderClientID, "e2e-client")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("без токена — доступ запрещён", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/api/v1/torrents")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("с чужим токеном — доступ запрещён", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/torrents", nil)
		require.NoError(t, err)
		req.Header.Set(auth.HeaderAccessToken, strings.Repeat("ab", 32))
		req.Header.Set(auth.HeaderClientID, "e2e-client")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("старый JWT-токен больше не принимается", func(t *testing.T) {
		// Раньше здесь был структурированный JWT; теперь приведения вида
		// "Bearer ..." нет вообще, и такой заголовок не должен работать.
		req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/torrents", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.valid")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("эндпоинты регистрации и входа удалены", func(t *testing.T) {
		for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/login",
			"/api/v1/auth/logout", "/api/v1/auth/change-password"} {
			// Each check gets its own budget; see clientRateLimiter.resetAll.
			globalClientRateLimiter.resetAll()

			// Without a token the request is refused before routing, which also
			// means the endpoint is unreachable; send a valid one to prove the
			// route itself no longer exists.
			resp, err := client.Post(server.URL+path, "application/json",
				bytes.NewBufferString(`{"username":"x","password":"y"}`))
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"без токена %s не должен быть доступен", path)

			req := authRequest(t, server, token, http.MethodPost, path)
			req.Header.Set("Content-Type", "application/json")
			resp, err = client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode,
				"маршрут %s должен быть удалён", path)
		}
	})
}

// TestE2E_TorrentList проверяет получение списка торрентов.
func TestE2E_TorrentList(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	// Запрашиваем список торрентов
	req, err := http.NewRequest("GET", server.URL+"/api/v1/torrents", nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotNil(t, result["torrents"])
	assert.NotNil(t, result["totalCount"])
}

// TestE2E_SyncFlow проверяет цикл синхронизации.
func TestE2E_SyncFlow(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Play
	playBody := map[string]interface{}{}
	body, _ := json.Marshal(playBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/sync/play", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var playResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&playResult)
	require.NoError(t, err)
	assert.Equal(t, true, playResult["isPlaying"])

	// 2. Seek (small jump, applied directly under latency compensation)
	seekBody := map[string]float64{
		"position": 1.5,
	}
	body, _ = json.Marshal(seekBody)

	req, err = http.NewRequest("POST", server.URL+"/api/v1/sync/seek", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var seekResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&seekResult)
	require.NoError(t, err)
	assert.Equal(t, 1.5, seekResult["position"])

	// 3. Pause
	pauseBody := map[string]interface{}{}
	body, _ = json.Marshal(pauseBody)

	req, err = http.NewRequest("POST", server.URL+"/api/v1/sync/pause", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var pauseResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&pauseResult)
	require.NoError(t, err)
	assert.Equal(t, false, pauseResult["isPlaying"])

	// 4. Get Status
	req, err = http.NewRequest("GET", server.URL+"/api/v1/sync/status", nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var statusResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&statusResult)
	require.NoError(t, err)
	assert.Equal(t, false, statusResult["isPlaying"])
	assert.Equal(t, 1.5, statusResult["position"])
}

// TestE2E_RoomFlow проверяет цикл работы с комнатами.
func TestE2E_RoomFlow(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Создание комнаты
	createBody := map[string]string{
		"name":     "Test Room",
		"password": "roompass123",
	}
	body, _ := json.Marshal(createBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/rooms", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var roomResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&roomResult)
	require.NoError(t, err)
	assert.NotNil(t, roomResult["id"])
	assert.Equal(t, "Test Room", roomResult["name"])
}

// TestE2E_UnauthorizedAccess проверяет защиту от несанкционированного доступа.
func TestE2E_UnauthorizedAccess(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	// Запрос без токена
	resp, err := client.Get(server.URL + "/api/v1/torrents")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Запрос с невалидным токеном
	req, err := http.NewRequest("GET", server.URL+"/api/v1/torrents", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestE2E_InvalidInput проверяет обработку невалидного ввода.
func TestE2E_InvalidInput(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	// Невалидный seek (отрицательная позиция)
	seekBody := map[string]float64{
		"position": -100,
	}
	body, _ := json.Marshal(seekBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/sync/seek", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, token)
	req.Header.Set(auth.HeaderClientID, "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestE2E_ContextCancellation(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/health", nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
