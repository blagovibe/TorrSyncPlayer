package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAuthService создаёт AuthService с фиксированным секретом для тестов
func testAuthService() *AuthService {
	svc, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	if err != nil {
		panic(err)
	}
	return svc
}

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
	authService := testAuthService()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// JWT токен должен содержать две точки (три части: header.payload.signature)
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "JWT токен должен содержать 3 части")
}

func TestValidateToken(t *testing.T) {
	authService := testAuthService()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
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
			claims, err := authService.ValidateToken(tt.token)
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
	authService := testAuthService()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	// Создаём истёкший токен напрямую через jwt.NewWithClaims
	claims := jwt.MapClaims{
		"userId":   user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(-1 * time.Hour).Unix(), // Истёк час назад
		"iat":      time.Now().Add(-2 * time.Hour).Unix(),
		"jti":      "test-expired-jti",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	expiredToken, err := token.SignedString(authService.jwtSecret)
	require.NoError(t, err)

	_, err = authService.ValidateToken(expiredToken)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestExtractJTI(t *testing.T) {
	authService := testAuthService()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	jti, err := authService.ExtractJTI(token)
	require.NoError(t, err)
	assert.NotEmpty(t, jti)

	// Проверяем что JTI уникален
	token2, err := authService.GenerateToken(user)
	require.NoError(t, err)

	jti2, err := authService.ExtractJTI(token2)
	require.NoError(t, err)
	assert.NotEqual(t, jti, jti2, "JTI должен быть уникальным для каждого токена")
}

func TestTokenRevocation(t *testing.T) {
	authService := testAuthService()

	// Получаем revocation store из auth service
	store := authService.GetRevocationStore()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	// Извлекаем JTI
	jti, err := authService.ExtractJTI(token)
	require.NoError(t, err)

	// Токен не должен быть отозван
	assert.False(t, store.IsRevoked(jti))

	// Отзываем токен
	store.Revoke(jti, time.Now().Add(24*time.Hour))

	// Токен должен быть отозван
	assert.True(t, store.IsRevoked(jti))

	// Проверяем что валидный токен всё ещё проходит валидацию
	claims, err := authService.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
}

func TestTokenRevocationStoreCleanup(t *testing.T) {
	store := &TokenRevocationStore{
		revokedTokens: make(map[string]time.Time),
		ttl:           100 * time.Millisecond,
	}

	// Добавляем токен с коротким TTL
	store.Revoke("jti1", time.Now().Add(100*time.Millisecond))
	assert.Equal(t, 1, store.Count())

	// Ждём истечения
	time.Sleep(200 * time.Millisecond)

	// Проверяем что токен автоматически удалён при проверке
	assert.False(t, store.IsRevoked("jti1"))
}

func TestValidateTokenWrongSecret(t *testing.T) {
	// Создаём первый сервис
	authService1, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	require.NoError(t, err)

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService1.GenerateToken(user)
	require.NoError(t, err)

	// Создаём второй сервис с другим секретом
	authService2, err := NewAuthService([]byte("different-secret-key-for-testing!"))
	require.NoError(t, err)

	// Токен не должен пройти валидацию с другим секретом
	_, err = authService2.ValidateToken(token)
	assert.Error(t, err)
}
