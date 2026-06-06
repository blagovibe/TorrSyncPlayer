// Package auth предоставляет middleware для JWT аутентификации.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// contextKey тип для ключей контекста.
type contextKey string

const (
	// ClaimsKey ключ для хранения claims в контексте запроса.
	ClaimsKey contextKey = "auth_claims"
	// JTIKey ключ для хранения JTI в контексте запроса.
	JTIKey contextKey = "auth_jti"
)

// JWTMiddleware создаёт middleware для проверки JWT токена.
// Проверяет наличие, валидность токена и его отзыв.
// Токен должен быть в формате: Bearer <token>
func (s *AuthService) JWTMiddleware(next http.Handler) http.Handler {
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

		tokenString := parts[1]
		if tokenString == "" {
			writeAuthError(w, http.StatusUnauthorized, "Пустой токен")
			return
		}

		// Валидируем токен
		claims, err := s.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, ErrExpiredToken) {
				writeAuthError(w, http.StatusUnauthorized, "Токен истёк")
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "Невалидный токен")
			return
		}

		// Проверяем, не отозван ли токен
		if claims.JTI != "" && s.revocationStore.IsRevoked(claims.JTI) {
			writeAuthError(w, http.StatusUnauthorized, "Токен отозван")
			return
		}

		// Добавляем claims и JTI в контекст запроса
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		if claims.JTI != "" {
			ctx = context.WithValue(ctx, JTIKey, claims.JTI)
		}
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

// GetJTI извлекает JTI из контекста запроса.
// Возвращает пустую строку если JTI не найден.
func GetJTI(r *http.Request) string {
	jti, ok := r.Context().Value(JTIKey).(string)
	if !ok {
		return ""
	}
	return jti
}

// LogoutHandler обработчик для отзыва JWT токена.
// POST /api/v1/auth/logout
//
// @Summary      Выход
// @Description  Отзывает JWT токен (добавляет в список отозванных)
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.SuccessResponse
// @Failure      400  {object}  models.ErrorResponse
// @Failure      401  {object}  models.ErrorResponse
// @Router       /api/v1/auth/logout [post]
func (s *AuthService) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Проверяем наличие заголовка Authorization
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Отсутствует заголовок Authorization"})
		return
	}

	// Получаем JTI из контекста (установлен middleware)
	jti := GetJTI(r)
	if jti == "" {
		// Пробуем извлечь JTI из токена напрямую
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			tokenString := strings.TrimSpace(authHeader[len("bearer "):])
			if tokenString == "" {
				response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Пустой токен"})
				return
			}
			jti, _ = s.ExtractJTI(tokenString)
		}
	}

	if jti == "" {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Не удалось идентифицировать токен"})
		return
	}

	// Отзываем токен (используем store из AuthService)
	// Устанавливаем время истечения как текущее время + 24 часа
	// (на случай если токен ещё не истёк)
	s.revocationStore.Revoke(jti, time.Now().Add(constants.RevocationStoreTTL))

	response.WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Токен успешно отозван"})
}

// writeAuthError записывает ошибку аутентификации в формате JSON.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="TorrSyncPlayer"`)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
