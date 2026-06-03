package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStoreCreate(t *testing.T) {
	store := NewUserStore()

	tests := []struct {
		name        string
		username    string
		password    string
		expectError bool
	}{
		{
			name:        "Валидный пользователь",
			username:    "testuser",
			password:    "password123",
			expectError: false,
		},
		{
			name:        "Пустое имя пользователя",
			username:    "",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "Короткое имя пользователя",
			username:    "ab",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "Длинное имя пользователя",
			username:    "abcdefghijklmnopqrstuvwxyz1234567890",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "Пустой пароль",
			username:    "testuser2",
			password:    "",
			expectError: true,
		},
		{
			name:        "Короткий пароль (менее 8 символов)",
			username:    "testuser3",
			password:    "12345",
			expectError: true,
		},
		{
			name:        "Пароль без букв",
			username:    "testuser4",
			password:    "12345678",
			expectError: true,
		},
		{
			name:        "Пароль без цифр",
			username:    "testuser5",
			password:    "password",
			expectError: true,
		},
		{
			name:        "Имя с недопустимыми символами",
			username:    "test@user!",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "Валидный пользователь с дефисом",
			username:    "test-user",
			password:    "password123",
			expectError: false,
		},
		{
			name:        "Валидный пользователь с подчёркиванием",
			username:    "test_user",
			password:    "password123",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.Create(tt.username, tt.password)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, user)
			assert.Equal(t, tt.username, user.Username)
			assert.NotEmpty(t, user.ID)
			assert.NotEmpty(t, user.PasswordHash)
			assert.NotEqual(t, tt.password, user.PasswordHash) // Пароль должен быть хеширован
		})
	}
}

func TestUserStoreCreateDuplicate(t *testing.T) {
	store := NewUserStore()

	// Создаём первого пользователя
	_, err := store.Create("testuser", "password123")
	require.NoError(t, err)

	// Пытаемся создать дубликат
	_, err = store.Create("testuser", "password456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "уже существует")
}

func TestUserStoreAuthenticate(t *testing.T) {
	store := NewUserStore()

	// Создаём пользователя
	_, err := store.Create("testuser", "password123")
	require.NoError(t, err)

	tests := []struct {
		name        string
		username    string
		password    string
		expectError bool
	}{
		{
			name:        "Верные учётные данные",
			username:    "testuser",
			password:    "password123",
			expectError: false,
		},
		{
			name:        "Неверный пароль",
			username:    "testuser",
			password:    "wrongpassword",
			expectError: true,
		},
		{
			name:        "Несуществующий пользователь",
			username:    "nonexistent",
			password:    "password123",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := store.Authenticate(tt.username, tt.password)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
			}
		})
	}
}

func TestUserStoreGetByUsername(t *testing.T) {
	store := NewUserStore()

	// Создаём пользователя
	created, err := store.Create("testuser", "password123")
	require.NoError(t, err)

	// Получаем по имени
	user, exists := store.GetByUsername("testuser")
	assert.True(t, exists)
	assert.Equal(t, created.ID, user.ID)

	// Несуществующий пользователь
	_, exists = store.GetByUsername("nonexistent")
	assert.False(t, exists)
}

func TestUserStoreGetByID(t *testing.T) {
	store := NewUserStore()

	// Создаём пользователя
	created, err := store.Create("testuser", "password123")
	require.NoError(t, err)

	// Получаем по ID
	user, exists := store.GetByID(created.ID)
	assert.True(t, exists)
	assert.Equal(t, "testuser", user.Username)

	// Несуществующий ID
	_, exists = store.GetByID("nonexistent-id")
	assert.False(t, exists)
}

func TestGenerateID(t *testing.T) {
	// Генерируем несколько ID и проверяем уникальность
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := generateID()
		assert.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "ID должен быть уникальным")
		ids[id] = true
	}
}
