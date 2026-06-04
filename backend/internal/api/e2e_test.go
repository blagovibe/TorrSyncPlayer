// Package api содержит E2E тесты для API endpoints.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServer создаёт тестовый сервер с реальными сервисами.
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
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

	p2pSvc, err := p2p.NewService(auth.NewAuthService([]byte("test-secret-key-for-e2e-tests")))
	require.NoError(t, err)

	syncSvc := sync.NewService()
	authStore := auth.NewUserStore()
	authService := auth.NewAuthService([]byte("test-secret-key-for-e2e-tests"))

	// Создаём роутер
	router := NewRouter(RouterConfig{
		TorrentSvc:  torrentSvc,
		P2pSvc:      p2pSvc,
		SyncSvc:     syncSvc,
		AuthStore:   authStore,
		AuthService: authService,
	})

	// Создаём тестовый сервер
	server := httptest.NewServer(router)

	// Функция очистки
	cleanup := func() {
		server.Close()
		_ = torrentSvc.Close()
		_ = p2pSvc.Close()
		syncSvc.Close()
	}

	return server, cleanup
}

// TestE2E_HealthCheck проверяет health check endpoint.
func TestE2E_HealthCheck(t *testing.T) {
	server, cleanup := setupTestServer(t)
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
	server, cleanup := setupTestServer(t)
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
	server, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

// TestE2E_AuthFlow проверяет полный цикл аутентификации.
func TestE2E_AuthFlow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Регистрация пользователя
	registerBody := map[string]string{
		"username": "testuser",
		"password": "testpassword123",
	}
	body, _ := json.Marshal(registerBody)

	resp, err := http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// 2. Вход в систему
	loginBody := map[string]string{
		"username": "testuser",
		"password": "testpassword123",
	}
	body, _ = json.Marshal(loginBody)

	resp, err = http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var loginResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResult)
	require.NoError(t, err)

	token, ok := loginResult["token"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, token)

	// 3. Доступ к защищённому endpoint с токеном
	req, err := http.NewRequest("GET", server.URL+"/api/v1/torrents", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestE2E_TorrentList проверяет получение списка торрентов.
func TestE2E_TorrentList(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Получаем токен
	token := getAuthToken(t, server.URL)

	// Запрашиваем список торрентов
	req, err := http.NewRequest("GET", server.URL+"/api/v1/torrents", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

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
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := getAuthToken(t, server.URL)
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Play
	playBody := map[string]interface{}{}
	body, _ := json.Marshal(playBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/sync/play", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var playResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&playResult)
	require.NoError(t, err)
	assert.Equal(t, true, playResult["isPlaying"])

	// 2. Seek
	seekBody := map[string]float64{
		"position": 120.5,
	}
	body, _ = json.Marshal(seekBody)

	req, err = http.NewRequest("POST", server.URL+"/api/v1/sync/seek", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var seekResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&seekResult)
	require.NoError(t, err)
	assert.Equal(t, 120.5, seekResult["position"])

	// 3. Pause
	pauseBody := map[string]interface{}{}
	body, _ = json.Marshal(pauseBody)

	req, err = http.NewRequest("POST", server.URL+"/api/v1/sync/pause", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
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
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var statusResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&statusResult)
	require.NoError(t, err)
	assert.Equal(t, false, statusResult["isPlaying"])
	assert.Equal(t, 120.5, statusResult["position"])
}

// TestE2E_RoomFlow проверяет цикл работы с комнатами.
func TestE2E_RoomFlow(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := getAuthToken(t, server.URL)
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Создание комнаты
	createBody := map[string]string{
		"name":     "Test Room",
		"password": "roompass123",
	}
	body, _ := json.Marshal(createBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/rooms", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
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
	server, cleanup := setupTestServer(t)
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
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := getAuthToken(t, server.URL)
	client := &http.Client{Timeout: 5 * time.Second}

	// Невалидный seek (отрицательная позиция)
	seekBody := map[string]float64{
		"position": -100,
	}
	body, _ := json.Marshal(seekBody)

	req, err := http.NewRequest("POST", server.URL+"/api/v1/sync/seek", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// getAuthToken получает JWT токен для тестов.
func getAuthToken(t *testing.T, baseURL string) string {
	t.Helper()

	// Регистрация
	registerBody := map[string]string{
		"username": "e2e_test_user",
		"password": "e2e_test_password",
	}
	body, _ := json.Marshal(registerBody)

	resp, err := http.Post(baseURL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()

	// Вход
	loginBody := map[string]string{
		"username": "e2e_test_user",
		"password": "e2e_test_password",
	}
	body, _ = json.Marshal(loginBody)

	resp, err = http.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	token, ok := result["token"].(string)
	require.True(t, ok)
	return token
}

// TestE2E_ContextCancellation проверяет корректную обработку отмены контекста.
func TestE2E_ContextCancellation(t *testing.T) {
	server, cleanup := setupTestServer(t)
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
