// Package auth предоставляет middleware для JWT аутентификации.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yourname/torrplayer/backend/internal/models"
)

// contextKey тип для ключей контекста.
type contextKey string

const (
	// ClaimsKey ключ для хранения claims в контексте запроса.
	ClaimsKey contextKey = "auth_claims"
)

// JWTMiddleware создаёт middleware для проверки JWT токена.
// Проверяет наличие и валидность токена в заголовке Authorization.
// Токен должен быть в формате: Bearer <token>
func JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Получаем заголовок Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAuthError(w, http.StatusUnauthorized, "Отсутствует заголовок Authorization")
			return
		}

		// Проверяем формат заголовка
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeAuthError(w, http.StatusUnauthorized, "Неверный формат заголовка. Используйте: Bearer <token>")
			return
		}

		token := parts[1]
		if token == "" {
			writeAuthError(w, http.StatusUnauthorized, "Пустой токен")
			return
		}

		// Валидируем токен
		claims, err := ValidateToken(token)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "Невалидный или истёкший токен")
			return
		}

		// Добавляем claims в контекст запроса
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims извлекает claims из контекста запроса.
// Возвращает nil если пользователь не аутентифицирован.
func GetClaims(r *http.Request) *models.Claims {
	claims, ok := r.Context().Value(ClaimsKey).(*models.Claims)
	if !ok {
		return nil
	}
	return claims
}

// writeAuthError записывает ошибку аутентификации в формате JSON.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="TorrSyncPlayer"`)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
