// Package api предоставляет HTTP API для сервера.
// Содержит middleware для логирования, CORS, recovery, rate limiting, CSRF и security headers.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// ── CSRF Protection ─────────────────────────────────────────────────────

// csrfTokenKey тип для хранения CSRF токена в контексте
type csrfTokenKey struct{}

// sessionIDKey тип для хранения session ID в контексте
type sessionIDKey struct{}

// CSRFTokenFromContext извлекает CSRF токен из контекста запроса
func CSRFTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(csrfTokenKey{}).(string); ok {
		return token
	}
	return ""
}

// SessionIDFromContext извлекает session ID из контекста запроса
func SessionIDFromContext(ctx context.Context) string {
	if sessionID, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return sessionID
	}
	return ""
}

// csrfTokenInfo хранит информацию о CSRF токене и связанной сессии
type csrfTokenInfo struct {
	Expiry    time.Time
	SessionID string
}

// csrfTokenStore хранит активные CSRF токены с TTL и привязкой к сессии
type csrfTokenStore struct {
	mu      sync.RWMutex
	tokens  map[string]*csrfTokenInfo
	ttl     time.Duration
	maxSize int
	stop    chan struct{}
	wg      sync.WaitGroup
}

// newCSRFTokenStore создаёт новое хранилище CSRF токенов
func newCSRFTokenStore() *csrfTokenStore {
	store := &csrfTokenStore{
		tokens:  make(map[string]*csrfTokenInfo),
		ttl:     constants.CSRFTokenTTL,
		maxSize: constants.CSRFTokenStoreMaxSize,
		stop:    make(chan struct{}),
	}

	// Запускаем периодическую очистку истёкших токенов
	store.wg.Add(1)
	go func() {
		defer store.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("CSRF: горутина cleanup завершилась с паникой", "error", r)
			}
		}()
		store.cleanup()
	}()

	return store
}

// cleanup периодически удаляет истёкшие токены
func (s *csrfTokenStore) cleanup() {
	ticker := time.NewTicker(constants.CSRFCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for token, info := range s.tokens {
				if now.After(info.Expiry) {
					delete(s.tokens, token)
				}
			}
			s.mu.Unlock()
		case <-s.stop:
			return
		}
	}
}

// Stop останавливает горутину cleanup и освобождает ресурсы.
// Блокирует выполнение до завершения горутины или до истечения таймаута.
func (s *csrfTokenStore) Stop() {
	close(s.stop)
	// Ожидаем завершения горутины с таймаутом
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("CSRF: горутина cleanup завершена корректно")
	case <-time.After(constants.CSRFShutdownTimeout):
		logger.Warn("CSRF: таймаут ожидания завершения горутины cleanup")
	}
}

// generateToken создаёт новый CSRF токен и привязывает его к сессии
func (s *csrfTokenStore) generateToken(sessionID string) (string, error) {
	bytes := make([]byte, constants.CSRFTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ограничиваем размер хранилища
	if len(s.tokens) >= s.maxSize {
		// Удаляем самый старый токен (простая эвристика)
		for k := range s.tokens {
			delete(s.tokens, k)
			break
		}
	}

	s.tokens[token] = &csrfTokenInfo{
		Expiry:    time.Now().Add(s.ttl),
		SessionID: sessionID,
	}
	return token, nil
}

// validateToken проверяет CSRF токен и соответствие сессии
func (s *csrfTokenStore) validateToken(token string, sessionID string) bool {
	if token == "" {
		return false
	}

	s.mu.RLock()
	info, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(info.Expiry) {
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
		return false
	}

	// Проверяем привязку к сессии (если sessionID указан)
	if sessionID != "" && info.SessionID != "" && info.SessionID != sessionID {
		logger.Warn("CSRF: несоответствие session ID", "expected", info.SessionID, "got", sessionID)
		return false
	}

	return true
}

// consumeToken использует токен (одноразовый для мутирующих операций)
func (s *csrfTokenStore) consumeToken(token string, sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.tokens[token]
	if !exists || time.Now().After(info.Expiry) {
		return false
	}

	// Проверяем привязку к сессии
	if sessionID != "" && info.SessionID != "" && info.SessionID != sessionID {
		return false
	}

	delete(s.tokens, token)
	return true
}

// Глобальное хранилище CSRF токенов
// Экспортировано для тестирования
var CSRFStore = newCSRFTokenStore()

// csrfStore для обратной совместимости
var csrfStore = CSRFStore

// mutatingMethods HTTP методы, требующие CSRF защиты
var mutatingMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodDelete: true,
	http.MethodPatch:  true,
}

// extractSessionID извлекает session ID из запроса (из cookie или заголовка)
func extractSessionID(r *http.Request) string {
	// Пробуем получить из cookie
	cookie, err := r.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Пробуем получить из заголовка X-Session-ID
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID != "" {
		return sessionID
	}

	return ""
}

// CSRFMiddleware создаёт middleware для защиты от CSRF атак.
// Проверяет CSRF токен для мутирующих запросов (POST, PUT, DELETE, PATCH).
// Исключает проверку для запросов с валидным JWT токеном (Authorization header).
// Токен может передаваться через заголовок X-CSRF-Token или параметр _csrf.
// Токены привязаны к сессии пользователя для предотвращения атак межсессионного CSRF.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем session ID из запроса
		sessionID := extractSessionID(r)

		// GET и OPTIONS не требуют CSRF проверки
		if !mutatingMethods[r.Method] {
			// Но генерируем токен для будущих запросов
			token, err := csrfStore.generateToken(sessionID)
			if err == nil {
				// Устанавливаем токен в заголовок ответа
				w.Header().Set("X-CSRF-Token", token)
				// Добавляем в контекст
				ctx := context.WithValue(r.Context(), csrfTokenKey{}, token)
				// Добавляем session ID в контекст
				if sessionID != "" {
					ctx = context.WithValue(ctx, sessionIDKey{}, sessionID)
				}
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
			return
		}

		// Проверяем наличие JWT токена - если есть, CSRF не требуется
		// JWT сам по себе защищает от CSRF при правильной реализации
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimPrefix(token, "bearer ")
			if len(token) > constants.MinJWTTokenLength { // Минимальная длина токена
				// JWT токен присутствуем, пропускаем CSRF проверку
				next.ServeHTTP(w, r)
				return
			}
		}

		// Для запросов без JWT проверяем CSRF токен
		csrfToken := r.Header.Get("X-CSRF-Token")
		if csrfToken == "" {
			// Пробуем получить из параметра запроса
			csrfToken = r.FormValue("_csrf")
		}

		if csrfToken == "" {
			logger.Warn("CSRF: отсутствует токен", "path", r.URL.Path, "method", r.Method)
			WriteError(w, http.StatusForbidden, "Отсутствует CSRF токен")
			return
		}

		// Валидируем токен через хранилище с проверкой сессии
		if !csrfStore.validateToken(csrfToken, sessionID) {
			logger.Warn("CSRF: невалидный токен или несоответствие сессии", "path", r.URL.Path, "method", r.Method, "sessionID", sessionID)
			WriteError(w, http.StatusForbidden, "Невалидный CSRF токен")
			return
		}

		// Токен валиден, пропускаем запрос
		next.ServeHTTP(w, r)
	})
}

// GetCSRFToken возвращает текущий CSRF токен из контекста или генерирует новый
func GetCSRFToken(ctx context.Context) string {
	token := CSRFTokenFromContext(ctx)
	if token == "" {
		sessionID := SessionIDFromContext(ctx)
		newToken, err := csrfStore.generateToken(sessionID)
		if err == nil {
			token = newToken
		}
	}
	return token
}

// ── Security Headers ────────────────────────────────────────────────────

// SecurityHeadersMiddleware добавляет заголовки безопасности ко всем ответам.
// Защищает от XSS, clickjacking, MIME-sniffing и других атак.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Предотвращает MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Защита от clickjacking - запрещает встраивание в iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS Protection для старых браузеров
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Контроль информации о referrer
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy - базовый набор правил
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self' ws: wss:; "+
				"media-src 'self' blob:; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")

		// Strict Transport Security (HSTS) - принудительное HTTPS соединение
		// max-age=31536000 - 1 год, includeSubDomains - применять ко всем поддоменам
		w.Header().Set("Strict-Transport-Security", constants.HSTSMaxAge)

		// Permissions Policy - ограничение API браузера
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=()")

		next.ServeHTTP(w, r)
	})
}

// ── Existing Middleware ─────────────────────────────────────────────────

// Logger middleware для логирования HTTP запросов.
// Записывает метод, путь, статус, длительность и удалённый адрес.
// Использует slog для структурированного логирования.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Оборачиваем ResponseWriter для получения статуса
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		slog.Info("HTTP запрос",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// getCORSOrigins возвращает список разрешённых CORS origins.
// Читает из переменной окружения CORS_ORIGINS (comma-separated).
// Если переменная не задана — использует defaults для разработки.
func getCORSOrigins() map[string]bool {
	origins := make(map[string]bool)

	// Пробуем прочитать из переменной окружения
	if corsOrigins := os.Getenv("CORS_ORIGINS"); corsOrigins != "" {
		for _, origin := range strings.Split(corsOrigins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				origins[origin] = true
			}
		}
	}

	// Если переменная не задана — используем defaults для разработки
	if len(origins) == 0 {
		origins["http://localhost:8889"] = true
		origins["https://localhost:8889"] = true
		origins["http://127.0.0.1:8889"] = true
		origins["https://127.0.0.1:8889"] = true
	}

	return origins
}

// CORS middleware для поддержки Cross-Origin Resource Sharing.
// Разрешает запросы с origins, указанных в переменной окружения CORS_ORIGINS.
// Формат переменной: "http://localhost:8889,http://127.0.0.1:8889"
// Если переменная не задана — использует defaults для разработки.
// Обрабатывает preflight запросы (OPTIONS).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Получаем список разрешённых origins
		allowedOrigins := getCORSOrigins()

		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", constants.CORSAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", constants.CORSAllowHeaders)
		w.Header().Set("Access-Control-Max-Age", constants.CORSMaxAge)
		w.Header().Set("Access-Control-Expose-Headers", constants.CORSExposeHeaders)

		// Обработка preflight запросов
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recovery middleware для обработки паник в обработчиках.
// Перехватывает паники и возвращает 500 ошибку вместо падения сервера.
// Логирует информацию о панике для отладки.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Паника в обработчике", "error", err, "path", r.URL.Path)
				WriteError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// NewRateLimiter создаёт middleware для ограничения частоты запросов.
// Параметр r - лимит запросов в секунду (rate.Limit).
// Параметр b - максимальный burst (размер ведра токенов).
// Возвращает 429 Too Many Requests при превышении лимита.
func NewRateLimiter(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !limiter.Allow() {
				WriteError(w, http.StatusTooManyRequests, "Слишком много запросов. Попробуйте позже.")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// responseWriter обёртка для перехвата статуса ответа.
// Позволяет middleware получить HTTP код ответа после обработки.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader перехватывает статус ответа.
// Сохраняет код для последующего использования в middleware.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
