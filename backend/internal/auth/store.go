// Package auth предоставляет хранилище пользователей.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
)

// dummyHash is a real bcrypt hash used for timing attack mitigation
// when a user is not found. Generated once at startup.
var dummyHash []byte

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing-mitigation"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("failed to generate dummy bcrypt hash: %v", err))
	}
	dummyHash = hash
}

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
	// Валидация имени пользователя
	if err := validation.ValidateUsername(username); err != nil {
		return nil, err
	}

	username = strings.ToLower(username)

	// Валидация пароля
	if err := validation.ValidatePassword(password); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем существование пользователя
	if _, exists := s.users[username]; exists {
		return nil, fmt.Errorf("%w: %s", ErrUserExists, username)
	}

	// Хешируем пароль
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("ошибка хеширования пароля: %w", err)
	}

	// Генерируем уникальный ID
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации ID: %w", err)
	}

	// Создаём пользователя
	user := &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().Unix(),
	}

	s.users[username] = user
	return user, nil
}

// Authenticate проверяет учётные данные пользователя.
// Возвращает пользователя если данные верны.
// Использует фиктивный хеш при отсутствии пользователя для защиты от timing-атак.
func (s *UserStore) Authenticate(username, password string) (*models.User, error) {
	username = strings.ToLower(username)
	s.mu.RLock()
	user, exists := s.users[username]
	s.mu.RUnlock()

	if !exists {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}

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
// Возвращает ошибку если не удалось получить случайные байты.
func generateID() (string, error) {
	bytes := make([]byte, constants.JTIBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("ошибка чтения случайных байт: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
