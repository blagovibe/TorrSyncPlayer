// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth provides functions for JWT authentication and password hashing.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	apperrors "github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

var (
	// ErrInvalidToken invalid token error
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpiredToken expired token error
	ErrExpiredToken = errors.New("token expired")
	// ErrInvalidCredentials invalid credentials error
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidCredentialsAppError AppError variant for HTTP handling
	ErrInvalidCredentialsAppError = apperrors.Unauthorized("invalid credentials")
	// ErrUserExists user already exists error
	ErrUserExists = errors.New("user already exists")
)

// AuthService authentication service with JWT.
// Stores the secret key and revocation store as struct fields.
type AuthService struct {
	jwtSecret       []byte
	revocationStore *TokenRevocationStore
	tokenTTL        time.Duration
}

// NewAuthService creates a new authentication service.
// secret must be non-empty (minimum 32 bytes).
// Creates an internal TokenRevocationStore for managing token revocation.
// Returns an error if secret is empty or too short.
func NewAuthService(secret []byte) (*AuthService, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("JWT secret is required — set via JWT_SECRET environment variable (minimum %d bytes)", constants.JWTSecretLength)
	}
	if len(secret) < constants.JWTSecretLength {
		return nil, fmt.Errorf("JWT secret too short: got %d bytes, minimum %d", len(secret), constants.JWTSecretLength)
	}

	svc := &AuthService{
		jwtSecret:       secret,
		revocationStore: NewTokenRevocationStore(),
		tokenTTL:        constants.JWTTokenTTL,
	}

	return svc, nil
}

// GetRevocationStore returns the token revocation store.
func (s *AuthService) GetRevocationStore() *TokenRevocationStore {
	return s.revocationStore
}

// SetPersistence enables JSON-file persistence for the user store and token store.
func (s *AuthService) SetPersistence(dataDir string) error {
	if s.revocationStore != nil {
		if err := s.revocationStore.SetPersistence(dataDir); err != nil {
			return fmt.Errorf("revocation store: %w", err)
		}
	}
	return nil
}

// SetTokenTTL sets the JWT token TTL.
func (s *AuthService) SetTokenTTL(ttl time.Duration) {
	s.tokenTTL = ttl
}

// Stop stops internal AuthService services.
// Call during graceful shutdown.
func (s *AuthService) Stop() {
	if s.revocationStore != nil {
		s.revocationStore.Stop()
	}
}

// HashPassword hashes a password using bcrypt.
// Returns the password hash or an error.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("password cannot be empty")
	}
	if len(password) > constants.MaxPasswordLength {
		return "", errors.New("password too long (maximum 72 characters)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), constants.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("password hashing error: %w", err)
	}
	return string(hash), nil
}

// CheckPassword checks a password against a hash using bcrypt.
// Returns nil if the password is correct, otherwise an error.
func CheckPassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentialsAppError
	}
	return nil
}

// GenerateToken creates a JWT authentication token using HMAC-SHA256.
// Token contains claims: userId, username, exp, iat, jti (JWT ID for revocation).
// Token expiration: 24 hours.
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	// Generate unique token ID for revocation support
	jtiBytes := make([]byte, constants.JTIBytes)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("JTI generation error: %w", err)
	}
	jti := hex.EncodeToString(jtiBytes)

	claims := jwt.MapClaims{
		"userId":   user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(s.tokenTTL).Unix(),
		"iat":      time.Now().Unix(),
		"jti":      jti,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("token signing error: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns user data.
// Checks signature, expiration and required claims.
// Returns an error if the token is invalid or expired.
func (s *AuthService) ValidateToken(tokenString string) (*models.Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Check signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		// Distinguish between expiration and other errors
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Extract user data
	userID, ok := claims["userId"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("%w: missing userId", ErrInvalidToken)
	}

	username, _ := claims["username"].(string)
	// username may be empty, this is not critical

	// Extract JTI for token revocation check.
	// JTI is required — without it token revocation is impossible.
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return nil, fmt.Errorf("%w: missing or invalid jti", ErrInvalidToken)
	}

	// Extract expiration time
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
		JTI:       jti,
	}, nil
}

// ValidateTokenWithRevocation validates a JWT token and checks revocation.
// Checks signature, expiration, required claims, and token revocation status.
func (s *AuthService) ValidateTokenWithRevocation(tokenString string) (*models.Claims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.JTI != "" && s.revocationStore.IsRevoked(claims.JTI) {
		return nil, fmt.Errorf("%w: token has been revoked", ErrInvalidToken)
	}
	return claims, nil
}

// ExtractJTI extracts JWT ID (jti) from the token.
// Used for token revocation check.
func (s *AuthService) ExtractJTI(tokenString string) (string, error) {
	if tokenString == "" {
		return "", ErrInvalidToken
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
	)

	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ErrInvalidToken
	}

	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return "", fmt.Errorf("%w: missing jti", ErrInvalidToken)
	}

	return jti, nil
}
