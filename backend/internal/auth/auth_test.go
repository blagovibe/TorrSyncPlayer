package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// testAuthService creates an AuthService with a fixed secret for tests
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
			name:        "Valid password",
			password:    "TestPass1!",
			expectError: false,
		},
		{
			name:        "Empty password",
			password:    "",
			expectError: true,
		},
		{
			name:        "Long password (>72 characters)",
			password:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectError: true,
		},
		{
			name:        "Minimal password",
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
	password := "TestPass1!"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	tests := []struct {
		name        string
		password    string
		hash        string
		expectError bool
	}{
		{
			name:        "Correct password",
			password:    password,
			hash:        hash,
			expectError: false,
		},
		{
			name:        "Wrong password",
			password:    "wrongpassword",
			hash:        hash,
			expectError: true,
		},
		{
			name:        "Empty password",
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

	// JWT token must contain two dots (three parts: header.payload.signature)
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "JWT token must contain 3 parts")
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
			name:        "Valid token",
			token:       token,
			expectError: false,
		},
		{
			name:        "Empty token",
			token:       "",
			expectError: true,
		},
		{
			name:        "Invalid format",
			token:       "invalid-token",
			expectError: true,
		},
		{
			name:        "Corrupted token",
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

	// Create an expired token directly via jwt.NewWithClaims
	claims := jwt.MapClaims{
		"userId":   user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
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

	// Verify JTI is unique
	token2, err := authService.GenerateToken(user)
	require.NoError(t, err)

	jti2, err := authService.ExtractJTI(token2)
	require.NoError(t, err)
	assert.NotEqual(t, jti, jti2, "JTI must be unique for each token")
}

func TestTokenRevocation(t *testing.T) {
	authService := testAuthService()

	// Get revocation store from auth service
	store := authService.GetRevocationStore()

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	// Extract JTI
	jti, err := authService.ExtractJTI(token)
	require.NoError(t, err)

	// Token should not be revoked
	assert.False(t, store.IsRevoked(jti))

	// Revoke token
	store.Revoke(jti, time.Now().Add(24*time.Hour))

	// Token should be revoked
	assert.True(t, store.IsRevoked(jti))

	// Verify that a valid token still passes validation
	claims, err := authService.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
}

func TestTokenRevocationStoreCleanup(t *testing.T) {
	store := &TokenRevocationStore{
		revokedTokens: make(map[string]time.Time),
		ttl:           100 * time.Millisecond,
	}

	// Add a token with short TTL
	store.Revoke("jti1", time.Now().Add(100*time.Millisecond))
	assert.Equal(t, 1, store.Count())

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Verify token is automatically removed on check
	assert.False(t, store.IsRevoked("jti1"))
}

func TestValidateTokenWrongSecret(t *testing.T) {
	// Create first service
	authService1, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	require.NoError(t, err)

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService1.GenerateToken(user)
	require.NoError(t, err)

	// Create second service with different secret
	authService2, err := NewAuthService([]byte("different-secret-key-for-testing-32bytes!"))
	require.NoError(t, err)

	// Token should not validate with different secret
	_, err = authService2.ValidateToken(token)
	assert.Error(t, err)
}
