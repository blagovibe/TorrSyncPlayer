package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	// Создаём rate limiter с высоким лимитом
	limiter := NewRateLimiter(rate.Limit(100), 10)

	// Создаём тестовый handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Оборачиваем в middleware
	handler := limiter(testHandler)

	// Отправляем несколько запросов
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	}
}

func TestNewRateLimiterExceedLimit(t *testing.T) {
	// Создаём rate limiter с очень низким лимитом
	limiter := NewRateLimiter(rate.Limit(0.01), 1) // 1 запрос в 100 секунд

	// Создаём тестовый handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Оборачиваем в middleware
	handler := limiter(testHandler)

	// Первый запрос должен пройти
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Второй запрос должен быть заблокирован
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
}

func TestCORS(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := CORS(testHandler)

	tests := []struct {
		name           string
		origin         string
		method         string
		expectedStatus int
	}{
		{
			name:           "Разрешённый origin (localhost)",
			origin:         "http://localhost:8889",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Разрешённый origin (127.0.0.1)",
			origin:         "http://127.0.0.1:8889",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "HTTPS origin",
			origin:         "https://localhost:8889",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Неразрешённый origin",
			origin:         "http://evil.com",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK, // CORS не блокирует, просто не заголовки
		},
		{
			name:           "Preflight запрос",
			origin:         "http://localhost:8889",
			method:         http.MethodOptions,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestRecovery(t *testing.T) {
	// Handler который вызывает панику
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := Recovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Не должно быть паники
	assert.NotPanics(t, func() {
		handler.ServeHTTP(rr, req)
	})

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestLogger(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := Logger(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Не должно быть паники
	assert.NotPanics(t, func() {
		handler.ServeHTTP(rr, req)
	})

	assert.Equal(t, http.StatusOK, rr.Code)
}
