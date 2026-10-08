// Package api содержит тесты потоковой доставки событий (SSE).
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
)

// openSSE создаёт комнату текущим клиентом и открывает SSE-поток.
//
// Комната обязательна: /rooms/{id}/events отвечает 403, если клиент в неё не
// вступил, поэтому подписаться «просто так» нельзя.
//
// Возвращает функцию закрытия, которую ОБЯЗАТЕЛЬНО вызывать до закрытия
// тестового сервера: httptest.Server.Close() блокируется на живом
// SSE-соединении, поэтому порядок defer'ов в тестах критичен.
func openSSE(t *testing.T, server *httptest.Server, token string) (*http.Response, func()) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"name": "SSE Test Room"})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/rooms", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-Access-Token", token)
	req.Header.Set("X-Client-Id", "e2e-client")
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "room creation failed")
	var room map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&room))
	require.NoError(t, resp.Body.Close())

	roomID, _ := room["id"].(string)
	require.NotEmpty(t, roomID, "created room must expose its id")

	ctx, cancel := context.WithCancel(context.Background())

	evReq, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/rooms/"+roomID+"/events", nil)
	require.NoError(t, err)
	evReq.Header.Set("X-Access-Token", token)
	evReq.Header.Set("X-Client-Id", "e2e-client")

	evResp, err := server.Client().Do(evReq.WithContext(ctx))
	require.NoError(t, err)

	var once bool
	closeSSE := func() {
		if once {
			return
		}
		once = true
		_ = evResp.Body.Close()
		cancel()
	}
	return evResp, closeSSE
}

// TestSSEStreamSurvivesLoggerWrapper — регрессионный тест на B-03.
//
// Logger оборачивает ResponseWriter в middleware.responseWriter, чтобы
// записать код ответа. Такой обёртке не хватало метода Flush(), поэтому
// w.(http.Flusher) в SSE-хендлере давал false и обработчик отвечал
// 500 "Streaming is not supported" на КАЖДОЕ подключение к событиям комнаты.
// Вся синхронизация комнат была мертва при полностью зелёной сборке.
//
// Тест идёт через настоящий роутер, поэтому ловит регрессию независимо от
// того, какой именно middleware добавили или переставили.
func TestSSEStreamSurvivesLoggerWrapper(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	resp, closeSSE := openSSE(t, server, token)
	defer closeSSE() // LIFO: выполняется до cleanup, сервер не ждёт живой поток

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"SSE endpoint must not answer 500 through the middleware chain")
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"),
		"SSE endpoint must advertise an event stream")

	// Первое событие ("event: connected") приходит сразу, до того как соединение
	// перейдёт в режим ожидания. Его получение доказывает, что Flush()
	// действительно работает: без него байты остались бы в буфере net/http,
	// и клиент не увидел бы их до первого ping.
	buf := make([]byte, 256)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err, "reading the first SSE event failed")
	assert.Contains(t, string(buf[:n]), "event: connected",
		"the initial SSE event should be flushed immediately")
}

// TestSSEContextCancellationReleasesConnection проверяет, что отмена контекста
// клиента действительно закрывает поток, а не оставляет горутину висеть.
func TestSSEContextCancellationReleasesConnection(t *testing.T) {
	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	resp, closeSSE := openSSE(t, server, token)
	defer closeSSE()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	closeSSE() // отмена контекста запроса должна разорвать чтение

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SSE read did not terminate after the client cancelled the request")
	}
}

// TestSSEOutlivesServerWriteTimeout — регрессионный тест на B-02.
//
// ServerWriteTimeout в net/http — это абсолютный дедлайн на ВЕСЬ ответ, а не
// таймер простоя между записими. При значении 30s соединение рвалось на
// тридцатой секунде: и видеопоток обрывался с unexpected EOF, и SSE молчал.
//
// Тест держит SSE-соединение дольше ServerWriteTimeout и убеждается, что
// пришли heartbeat-сообщения. Запускается только в полном режиме: по умолчанию
// занимает больше 30 секунд.
func TestSSEOutlivesServerWriteTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short mode: holds a connection longer than ServerWriteTimeout")
	}

	server, token, cleanup := setupTestServer(t)
	defer cleanup()

	resp, closeSSE := openSSE(t, server, token)
	defer closeSSE()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// ssePingInterval — 30s; ждём два пинга, чтобы заведомо перевалить
	// прежний 30-секундный WriteTimeout.
	deadline := time.Now().Add(75 * time.Second)
	buf := make([]byte, 512)
	var seen strings.Builder
	pings := 0
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		seen.Write(buf[:n])
		if strings.Contains(seen.String(), "ping") {
			pings++
			if pings >= 2 {
				return // пережили рубеж — успех
			}
		}
		if err != nil {
			t.Fatalf("SSE stream ended early after %q: %v", seen.String(), err)
		}
	}
	t.Fatalf("no heartbeats within the window; the stream was not kept alive")
}
