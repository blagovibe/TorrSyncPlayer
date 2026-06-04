// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth предоставляет функции для JWT аутентификации и хеширования паролей.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

var (
	// ErrInvalidToken ошибка невалидного токена
	ErrInvalidToken = errors.New("невалидный токен")
	// ErrExpiredToken ошибка истёкшего токена
	ErrExpiredToken = errors.New("токен истёк")
	// ErrInvalidCredentials ошибка неверных учётных данных
	ErrInvalidCredentials = errors.New("неверные учётные данные")
)

// AuthService сервис аутентификации с JWT.
// Хранит секретный ключ как поле структуры вместо глобальной переменной.
type AuthService struct {
	jwtSecret []byte
}

// NewAuthService создаёт новый сервис аутентификации.
// Если secret пустой, проверяет переменную окружения JWT_SECRET,
// затем генерирует случайный ключ.
func NewAuthService(secret []byte) *AuthService {
	svc := &AuthService{}

	if len(secret) > 0 {
		svc.jwtSecret = secret
		return svc
	}

	// Проверяем переменную окружения JWT_SECRET
	if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
		svc.jwtSecret = []byte(envSecret)
		return svc
	}

	// Генерируем случайный ключ
	svc.jwtSecret = make([]byte, constants.JWTSecretLength)
	if _, err := rand.Read(svc.jwtSecret); err != nil {
		// При ошибке генерации ключа паникуем
		// так как безопасность критически важна
		panic(fmt.Sprintf("не удалось сгенерировать безопасный JWT ключ: %v", err))
	}

	return svc
}

// SetSecret устанавливает секретный ключ для JWT.
// Используется для тестирования и для установки ключа из конфигурации.
func (s *AuthService) SetSecret(secret []byte) {
	s.jwtSecret = secret
}

// HashPassword хеширует пароль с использованием bcrypt.
// Возвращает хеш пароля или ошибку.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("пароль не может быть пустым")
	}
	if len(password) > constants.MaxPasswordLength {
		return "", errors.New("пароль слишком длинный (максимум 72 символа)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), constants.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("ошибка хеширования пароля: %w", err)
	}
	return string(hash), nil
}

// CheckPassword проверяет пароль по хешу с использованием bcrypt.
// Возвращает nil если пароль верный, иначе ошибку.
func CheckPassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// GenerateToken создаёт JWT токен аутентификации с использованием HMAC-SHA256.
// Токен содержит claims: userId, username, exp, iat, jti (JWT ID для revocation).
// Срок действия токена: 24 часа.
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	// Генерируем уникальный ID токена для возможности отзыва
	jtiBytes := make([]byte, constants.JTIBytes)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("ошибка генерации JTI: %w", err)
	}
	jti := hex.EncodeToString(jtiBytes)

	claims := jwt.MapClaims{
		"userId":   user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(constants.JWTTokenTTL).Unix(),
		"iat":      time.Now().Unix(),
		"jti":      jti,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("ошибка подписи токена: %w", err)
	}

	return tokenString, nil
}

// ValidateToken проверяет JWT токен и возвращает данные пользователя.
// Проверяет подпись, срок действия и наличие обязательных claims.
// Возвращает ошибку если токен невалидный или истёк.
func (s *AuthService) ValidateToken(tokenString string) (*models.Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Проверяем метод подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный метод подписи: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		// Различаем ошибки истечения и другие ошибки
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Извлекаем данные пользователя
	userID, ok := claims["userId"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("%w: отсутствует userId", ErrInvalidToken)
	}

	username, _ := claims["username"].(string)
	// username может быть пустым, это не критично

	// Извлекаем время истечения
	var expiresAt int64
	switch exp := claims["exp"].(type) {
	case float64:
		expiresAt = int64(exp)
	case int64:
		expiresAt = exp
	case json.Number:
		expiresAt, _ = exp.Int64()
	}

	return &models.Claims{
		UserID:    userID,
		Username:  username,
		ExpiresAt: expiresAt,
	}, nil
}

// ExtractJTI извлекает JWT ID (jti) из токена.
// Используется для проверки отзыва токена.
func (s *AuthService) ExtractJTI(tokenString string) (string, error) {
	if tokenString == "" {
		return "", ErrInvalidToken
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный метод подписи: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
	)

	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ErrInvalidToken
	}

	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return "", fmt.Errorf("%w: отсутствует jti", ErrInvalidToken)
	}

	return jti, nil
}
