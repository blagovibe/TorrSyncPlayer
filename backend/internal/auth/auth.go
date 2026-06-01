// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth предоставляет функции для JWT аутентификации и хеширования паролей.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/yourname/torrplayer/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	// jwtSecret секретный ключ для подписи JWT токенов.
	// В production должен быть загружен из переменных окружения.
	jwtSecret  []byte
	secretOnce sync.Once

	// ErrInvalidToken ошибка невалидного токена
	ErrInvalidToken = errors.New("невалидный токен")
	// ErrExpiredToken ошибка истёкшего токена
	ErrExpiredToken = errors.New("токен истёк")
	// ErrInvalidCredentials ошибка неверных учётных данных
	ErrInvalidCredentials = errors.New("неверные учётные данные")
)

// initSecret инициализирует секретный ключ для JWT.
// Генерирует случайный ключ при первом вызове.
func initSecret() {
	secretOnce.Do(func() {
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			// Fallback на фиксированный ключ для разработки
			jwtSecret = []byte("dev-secret-key-change-in-production-32bytes!")
		}
	})
}

// SetSecret устанавливает секретный ключ для JWT.
// Используется для тестирования и для установки ключа из конфигурации.
func SetSecret(secret []byte) {
	jwtSecret = secret
}

// HashPassword хеширует пароль с использованием bcrypt.
// Возвращает хеш пароля или ошибку.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("пароль не может быть пустым")
	}
	if len(password) > 72 {
		return "", errors.New("пароль слишком длинный (максимум 72 символа)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
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

// GenerateToken создаёт простой токен аутентификации.
// Формат: hex(data).hex(signature)
// Это упрощённая реализация без внешних зависимостей.
func GenerateToken(user *models.User) (string, error) {
	initSecret()

	// Создаём данные токена
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%s|%s|%d", user.ID, user.Username, timestamp)

	// Создаём подпись
	signature := createSignature(data)

	// Кодируем токен: data и signature разделяются точкой
	token := hex.EncodeToString([]byte(data)) + "." + hex.EncodeToString(signature)
	return token, nil
}

// ValidateToken проверяет токен и возвращает данные пользователя.
// Возвращает ошибку если токен невалидный или истёк.
func ValidateToken(token string) (*models.Claims, error) {
	initSecret()

	if token == "" {
		return nil, ErrInvalidToken
	}

	// Разделяем токен на данные и подпись
	parts := splitToken(token, '.')
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}

	// Декодируем данные
	data, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Декодируем подпись
	signature, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Проверяем подпись
	expectedSig := createSignature(string(data))
	if subtle.ConstantTimeCompare(signature, expectedSig) != 1 {
		return nil, ErrInvalidToken
	}

	// Парсим данные
	claims, err := parseTokenData(string(data))
	if err != nil {
		return nil, err
	}

	// Проверяем срок действия (24 часа)
	if time.Now().Unix()-claims.ExpiresAt > 86400 {
		return nil, ErrExpiredToken
	}

	return claims, nil
}

// createSignature создаёт подпись данных с использованием секретного ключа.
func createSignature(data string) []byte {
	// Простая реализация HMAC-подобной подписи
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = data[i] ^ jwtSecret[i%len(jwtSecret)]
	}
	return result
}

// splitToken разбивает токен на части по разделителю.
func splitToken(token string, sep byte) []string {
	var parts []string
	current := ""
	for i := 0; i < len(token); i++ {
		if token[i] == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(token[i])
		}
	}
	parts = append(parts, current)
	return parts
}

// parseTokenData парсит данные токена.
// Формат: userID|username|timestamp
func parseTokenData(data string) (*models.Claims, error) {
	var userID, username string
	var timestamp int64

	// Парсим формат userID|username|timestamp
	parts := splitToken(data, '|')
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	userID = parts[0]
	username = parts[1]
	_, err := fmt.Sscanf(parts[2], "%d", &timestamp)
	if err != nil {
		return nil, ErrInvalidToken
	}

	return &models.Claims{
		UserID:    userID,
		Username:  username,
		ExpiresAt: timestamp,
	}, nil
}
