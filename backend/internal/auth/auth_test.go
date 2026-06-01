package auth

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/torrplayer/backend/internal/models"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{
			name:        "Валидный пароль",
			password:    "password123",
			expectError: false,
		},
		{
			name:        "Пустой пароль",
			password:    "",
			expectError: true,
		},
		{
			name:        "Длинный пароль (>72 символа)",
			password:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectError: true,
		},
		{
			name:        "Минимальный пароль",
			password:    "123456",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.NotEqual(t, tt.password, hash)
		})
	}
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	tests := []struct {
		name        string
		password    string
		hash        string
		expectError bool
	}{
		{
			name:        "Верный пароль",
			password:    password,
			hash:        hash,
			expectError: false,
		},
		{
			name:        "Неверный пароль",
			password:    "wrongpassword",
			hash:        hash,
			expectError: true,
		},
		{
			name:        "Пустой пароль",
			password:    "",
			hash:        hash,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPassword(tt.password, tt.hash)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	// Устанавливаем фиксированный секрет для тестов
	SetSecret([]byte("test-secret-key-for-testing-32bytes!"))

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := GenerateToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, ".") // Формат: data.signature
}

func TestValidateToken(t *testing.T) {
	// Устанавливаем фиксированный секрет для тестов
	SetSecret([]byte("test-secret-key-for-testing-32bytes!"))

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := GenerateToken(user)
	require.NoError(t, err)

	tests := []struct {
		name        string
		token       string
		expectError bool
	}{
		{
			name:        "Валидный токен",
			token:       token,
			expectError: false,
		},
		{
			name:        "Пустой токен",
			token:       "",
			expectError: true,
		},
		{
			name:        "Невалидный формат",
			token:       "invalid-token",
			expectError: true,
		},
		{
			name:        "Повреждённый токен",
			token:       "abc.def.ghi",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := ValidateToken(tt.token)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, claims)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, claims)
				assert.Equal(t, user.ID, claims.UserID)
				assert.Equal(t, user.Username, claims.Username)
			}
		})
	}
}

func TestValidateTokenExpired(t *testing.T) {
	// Устанавливаем фиксированный секрет для тестов
	SetSecret([]byte("test-secret-key-for-testing-32bytes!"))

	// Создаём токен с истёкшим временем вручную
	expiredTime := time.Now().Add(-48 * time.Hour).Unix() // 48 часов назад
	data := "user123|testuser|" + fmt.Sprintf("%d", expiredTime)
	signature := createSignature(data)
	token := hex.EncodeToString([]byte(data)) + "." + hex.EncodeToString(signature)

	_, err := ValidateToken(token)
	assert.Error(t, err)
	assert.Equal(t, ErrExpiredToken, err)
}

func TestEncodeDecodeHex(t *testing.T) {
	tests := []string{
		"simple string",
		"user123|testuser|1234567890",
		"специальные символы: !@#$%^&*()",
	}

	for _, original := range tests {
		encoded := hex.EncodeToString([]byte(original))
		decoded, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		assert.Equal(t, original, string(decoded))
	}
}
