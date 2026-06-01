// Package auth предоставляет хранилище пользователей.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/yourname/torrplayer/backend/internal/models"
)

// UserStore хранилище пользователей в памяти.
// В production следует использовать базу данных.
type UserStore struct {
	mu    sync.RWMutex
	users map[string]*models.User // key: username
}

// NewUserStore создаёт новое хранилище пользователей.
func NewUserStore() *UserStore {
	return &UserStore{
		users: make(map[string]*models.User),
	}
}

// Create создаёт нового пользователя.
// Возвращает ошибку если пользователь уже существует.
func (s *UserStore) Create(username, password string) (*models.User, error) {
	if username == "" {
		return nil, fmt.Errorf("имя пользователя не может быть пустым")
	}
	if len(username) < 3 || len(username) > 32 {
		return nil, fmt.Errorf("имя пользователя должно быть от 3 до 32 символов")
	}
	if password == "" {
		return nil, fmt.Errorf("пароль не может быть пустым")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("пароль должен быть не менее 6 символов")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем существование пользователя
	if _, exists := s.users[username]; exists {
		return nil, fmt.Errorf("пользователь %s уже существует", username)
	}

	// Хешируем пароль
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("ошибка хеширования пароля: %w", err)
	}

	// Создаём пользователя
	user := &models.User{
		ID:           generateID(),
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().Unix(),
	}

	s.users[username] = user
	return user, nil
}

// Authenticate проверяет учётные данные пользователя.
// Возвращает пользователя если данные верны.
func (s *UserStore) Authenticate(username, password string) (*models.User, error) {
	s.mu.RLock()
	user, exists := s.users[username]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrInvalidCredentials
	}

	// Проверяем пароль
	if err := CheckPassword(password, user.PasswordHash); err != nil {
		return nil, err
	}

	return user, nil
}

// GetByUsername возвращает пользователя по имени.
func (s *UserStore) GetByUsername(username string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[username]
	return user, exists
}

// GetByID возвращает пользователя по ID.
func (s *UserStore) GetByID(id string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.ID == id {
			return user, true
		}
	}
	return nil, false
}

// generateID генерирует уникальный идентификатор.
func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
