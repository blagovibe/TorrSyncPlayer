// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── CSRF Store Tests ────────────────────────────────────────────────────

func TestCSRFTokenStore_Stop(t *testing.T) {
	// Создаём новый store для теста
	store := newCSRFTokenStore()
	require.NotNil(t, store)

	// Генерируем токен для проверки (с пустым session ID для обратной совместимости)
	token, err := store.generateToken("")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Проверяем что токен валиден (с пустым session ID)
	assert.True(t, store.validateToken(token, ""))

	// Останавливаем store - горутина должна завершиться
	store.Stop()

	// Даём время на завершение горутины
	time.Sleep(100 * time.Millisecond)
}

func TestCSRFTokenStore_CleanupRemovesExpiredTokens(t *testing.T) {
	store := &csrfTokenStore{
		tokens:  make(map[string]*csrfTokenInfo),
		ttl:     50 * time.Millisecond, // Очень короткий TTL для теста
		maxSize: 100,
		stop:    make(chan struct{}),
	}

	// Добавляем токен
	store.tokens["test-token"] = &csrfTokenInfo{
		Expiry:    time.Now().Add(store.ttl),
		SessionID: "",
	}

	// Ждём истечения TTL
	time.Sleep(100 * time.Millisecond)

	// Вручную запускаем cleanup
	store.mu.Lock()
	now := time.Now()
	for token, info := range store.tokens {
		if now.After(info.Expiry) {
			delete(store.tokens, token)
		}
	}
	store.mu.Unlock()

	// Токен должен быть удалён
	assert.False(t, store.validateToken("test-token", ""))

	// Останавливаем
	store.Stop()
}

func TestCSRFTokenStore_MaxSizeLimit(t *testing.T) {
	store := &csrfTokenStore{
		tokens:  make(map[string]*csrfTokenInfo),
		ttl:     1 * time.Hour,
		maxSize: 5, // Маленький лимит для теста
		stop:    make(chan struct{}),
	}

	// Добавляем токены сверх лимита
	for i := 0; i < 10; i++ {
		token, err := store.generateToken("")
		require.NoError(t, err)
		require.NotEmpty(t, token)
	}

	// Проверяем что количество токенов не превышает лимит значительно
	store.mu.RLock()
	count := len(store.tokens)
	store.mu.RUnlock()

	// Должно быть не более maxSize + 1 (один мог быть добавлен до проверки)
	assert.LessOrEqual(t, count, store.maxSize+1)

	store.Stop()
}

func TestCSRFTokenStore_SessionBinding(t *testing.T) {
	store := newCSRFTokenStore()
	defer store.Stop()

	// Генерируем токен с session ID
	sessionID := "session-123"
	token, err := store.generateToken(sessionID)
	require.NoError(t, err)

	// Токен валиден с правильным session ID
	assert.True(t, store.validateToken(token, sessionID))

	// Токен невалиден с другим session ID
	assert.False(t, store.validateToken(token, "wrong-session"))

	// Токен валиден с пустым session ID (для обратной совместимости)
	token2, err := store.generateToken("")
	require.NoError(t, err)
	assert.True(t, store.validateToken(token2, ""))
	assert.True(t, store.validateToken(token2, "any-session"))
}

// ── SSE Connection Manager Tests ────────────────────────────────────────

func TestSSEConnectionManager_TryAcquire(t *testing.T) {
	manager := newSSEConnectionManager(2) // Лимит 2 соединения

	// Первые два соединения должны быть разрешены для одной комнаты
	assert.True(t, manager.tryAcquire("room1"))
	assert.True(t, manager.tryAcquire("room1"))

	// Третье соединение должно быть заблокировано
	assert.False(t, manager.tryAcquire("room1"))

	// Проверяем счётчик (общее количество соединений)
	assert.Equal(t, 2, manager.Count())
}

func TestSSEConnectionManager_Release(t *testing.T) {
	manager := newSSEConnectionManager(2)

	// Занимаем оба слота для одной комнаты
	manager.tryAcquire("room1")
	manager.tryAcquire("room1")
	assert.Equal(t, 2, manager.Count())

	// Освобождаем один
	manager.release("room1")
	assert.Equal(t, 1, manager.Count())

	// Теперь можно получить новый слот
	assert.True(t, manager.tryAcquire("room1"))
	assert.Equal(t, 2, manager.Count())
}

func TestSSEConnectionManager_ConcurrentAccess(t *testing.T) {
	manager := newSSEConnectionManager(10)

	// Конкурентный доступ
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			if manager.tryAcquire("room1") {
				time.Sleep(10 * time.Millisecond)
				manager.release("room1")
			}
			done <- true
		}()
	}

	// Ждём завершения всех горутин
	for i := 0; i < 20; i++ {
		<-done
	}

	// Все соединения должны быть освобождены
	assert.Equal(t, 0, manager.Count())
}

func TestSSEConnectionManager_MultipleRooms(t *testing.T) {
	manager := newSSEConnectionManager(2) // Лимит 2 соединения на комнату

	// Комната 1: 2 соединения
	assert.True(t, manager.tryAcquire("room1"))
	assert.True(t, manager.tryAcquire("room1"))
	assert.False(t, manager.tryAcquire("room1")) // Лимит исчерпан

	// Комната 2: 2 соединения (независимо от room1)
	assert.True(t, manager.tryAcquire("room2"))
	assert.True(t, manager.tryAcquire("room2"))
	assert.False(t, manager.tryAcquire("room2")) // Лимит исчерпан

	// Общее количество: 4
	assert.Equal(t, 4, manager.Count())
}

// ── Goroutine Leak Tests ────────────────────────────────────────────────

func TestCSRFStore_NoGoroutineLeak(t *testing.T) {
	// Запоминаем количество горутин до теста
	goroutinesBefore := runtime.NumGoroutine()

	// Создаём и останавливаем store несколько раз
	for i := 0; i < 5; i++ {
		store := newCSRFTokenStore()
		time.Sleep(10 * time.Millisecond)
		store.Stop()
		time.Sleep(50 * time.Millisecond)
	}

	// Даём время на сборку мусора
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// Количество горутин не должно значительно вырасти
	goroutinesAfter := runtime.NumGoroutine()
	// Допускаем разницу в 5 горутин (на случай фоновых процессов в CI)
	assert.LessOrEqual(t, goroutinesAfter, goroutinesBefore+5,
		"Обнаружена утечка горутин: было %d, стало %d", goroutinesBefore, goroutinesAfter)
}

func TestSSEManager_ConnectionLimit(t *testing.T) {
	manager := newSSEConnectionManager(5)

	// Заполняем все слоты для одной комнаты
	for i := 0; i < 5; i++ {
		assert.True(t, manager.tryAcquire("room1"))
	}

	// Следующее соединение должно быть заблокировано
	assert.False(t, manager.tryAcquire("room1"))

	// Освобождаем все
	for i := 0; i < 5; i++ {
		manager.release("room1")
	}

	assert.Equal(t, 0, manager.Count())
}

// ── CSRF Middleware Tests ───────────────────────────────────────────────

func TestCSRFMiddleware_GeneratesTokenForGET(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := CSRFMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:8889")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	csrfToken := rr.Header().Get("X-CSRF-Token")
	assert.NotEmpty(t, csrfToken)
}

func TestCSRFMiddleware_RequiresTokenForPOST(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := CSRFMiddleware(testHandler)

	// POST без токена должен быть отклонён
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestCSRFMiddleware_AcceptsValidToken(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := CSRFMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:8889")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csrfToken := rr.Header().Get("X-CSRF-Token")
	require.NotEmpty(t, csrfToken)

	req = httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
