package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
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

// resetTrustedCIDRsForTests очищает кэш доверенных прокси, чтобы тест мог
// задать TRUSTED_PROXIES и увидеть эффект в этом же процессе.
func resetTrustedCIDRsForTests() {
	trustedCIDRsOnce = sync.Once{}
	cachedTrustedCIDRs = nil
	hasTrustedCIDRs = false
}

// TestGetClientIPIgnoresForwardedHeadersByDefault — регрессионный тест на B-07.
//
// Раньше при незаданном TRUSTED_PROXIES доверенным считался ЛЮБОЙ приватный,
// loopback и link-local адрес. Сервер рассчитан на доступ извне (LAN,
// проброс портов), поэтому клиент мог подставить X-Forwarded-For с любым
// значением: обойти per-IP лимит и заставить карту лимитеров расти без
// ограничения. Теперь по умолчанию не доверяем никому.
func TestGetClientIPIgnoresForwardedHeadersByDefault(t *testing.T) {
	resetTrustedCIDRsForTests()
	t.Setenv("TRUSTED_PROXIES", "")
	t.Cleanup(resetTrustedCIDRsForTests)

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
	}{
		{"private LAN client", "192.168.1.50:5555", "203.0.113.7"},
		{"10.x client", "10.1.2.3:5555", "198.51.100.9"},
		{"loopback client", "127.0.0.1:5555", "203.0.113.7"},
		{"link-local client", "169.254.10.10:5555", "203.0.113.7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
			r.RemoteAddr = tc.remoteAddr
			r.Header.Set("X-Forwarded-For", tc.xff)
			r.Header.Set("X-Real-IP", "203.0.113.99")

			got := getClientIP(r)
			host, _, err := net.SplitHostPort(tc.remoteAddr)
			if err != nil {
				host = tc.remoteAddr
			}
			if got != host {
				t.Fatalf("getClientIP = %q, want the real peer %q: "+
					"forwarding headers must be ignored unless the proxy is explicitly trusted", got, host)
			}
		})
	}
}

// TestGetClientIPHonoursExplicitlyTrustedProxy проверяет, что явная настройка
// TRUSTED_PROXIES по-прежнему работает — иначе фикс сломал бы реальные
// развёртывания за обратным прокси.
func TestGetClientIPHonoursExplicitlyTrustedProxy(t *testing.T) {
	resetTrustedCIDRsForTests()
	t.Setenv("TRUSTED_PROXIES", "192.168.1.0/24")
	t.Cleanup(resetTrustedCIDRsForTests)

	t.Run("trusted proxy header is used", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
		r.RemoteAddr = "192.168.1.10:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 192.168.1.10")
		if got := getClientIP(r); got != "203.0.113.7" {
			t.Fatalf("getClientIP = %q, want 203.0.113.7 for a configured proxy", got)
		}
	})

	t.Run("untrusted peer header is ignored", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
		r.RemoteAddr = "198.51.100.5:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		if got := getClientIP(r); got != "198.51.100.5" {
			t.Fatalf("getClientIP = %q, want the real peer 198.51.100.5", got)
		}
	})
}
