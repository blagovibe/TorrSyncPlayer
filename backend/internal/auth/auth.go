// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth provides functions for JWT authentication and password hashing.
package auth

import (
	"crypto/rand"
	"crypto/tls"
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
	issuer          string   // Optional issuer claim
	audience        []string // Optional audience claims
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

// SetIssuer sets the issuer claim for generated tokens.
// If set, tokens will include this issuer and validation will require it.
func (s *AuthService) SetIssuer(issuer string) {
	s.issuer = issuer
}

// SetAudience sets the audience claim(s) for generated tokens.
// If set, tokens will include these audiences and validation will require at least one to match.
func (s *AuthService) SetAudience(audience ...string) {
	s.audience = audience
}

// Stop stops internal AuthService services.
// Call during graceful shutdown.
func (s *AuthService) Stop() {
	if s.revocationStore != nil {
		s.revocationStore.Stop()
	}
}

// ReloadTLSConfiguration reloads TLS certificates from disk.
// Designed to be called on SIGHUP for certificate rotation.
// Returns an error if certificates cannot be loaded.
func (s *AuthService) ReloadTLSConfiguration(certPath, keyPath string) (*tls.Config, error) {
	// Load new certificates
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("certificate loading error: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}, nil
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
// Optionally includes issuer (iss) and audience (aud) claims if configured.
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

	// Add issuer claim if configured
	if s.issuer != "" {
		claims["iss"] = s.issuer
	}

	// Add audience claim if configured
	if len(s.audience) > 0 {
		claims["aud"] = s.audience
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("token signing error: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns user data.
// Checks signature, expiration, required claims, and optionally issuer/audience.
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

	// Validate issuer claim if configured
	if s.issuer != "" {
		iss, ok := claims["iss"].(string)
		if !ok || iss != s.issuer {
			return nil, fmt.Errorf("%w: invalid issuer", ErrInvalidToken)
		}
	}

	// Validate audience claim if configured
	if len(s.audience) > 0 {
		audClaim, ok := claims["aud"]
		if !ok {
			return nil, fmt.Errorf("%w: missing audience", ErrInvalidToken)
		}

		// Audience can be string or []string
		var tokenAudiences []string
		switch aud := audClaim.(type) {
		case string:
			tokenAudiences = []string{aud}
		case []interface{}:
			for _, a := range aud {
				if s, ok := a.(string); ok {
					tokenAudiences = append(tokenAudiences, s)
				}
			}
		}

		if len(tokenAudiences) == 0 {
			return nil, fmt.Errorf("%w: invalid audience format", ErrInvalidToken)
		}

		// Check if at least one token audience matches configured audiences
		matched := false
		for _, tokenAud := range tokenAudiences {
			for _, configuredAud := range s.audience {
				if tokenAud == configuredAud {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched {
			return nil, fmt.Errorf("%w: audience not allowed", ErrInvalidToken)
		}
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

// GenerateRefreshToken creates a long-lived refresh token.
// Refresh tokens have longer TTL (7 days default) and are used to obtain new access tokens.
// H1: Added for refresh token support.
func (s *AuthService) GenerateRefreshToken(user *models.User) (string, error) {
	jtiBytes := make([]byte, constants.JTIBytes)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("JTI generation error: %w", err)
	}
	jti := hex.EncodeToString(jtiBytes)

	// Refresh tokens live longer (7 days)
	refreshTTL := constants.RefreshTokenTTL
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour
	}

	claims := jwt.MapClaims{
		"userId":    user.ID,
		"username":  user.Username,
		"exp":       time.Now().Add(refreshTTL).Unix(),
		"iat":       time.Now().Unix(),
		"jti":       jti,
		"tokenType": "refresh",
	}

	// Add issuer claim if configured
	if s.issuer != "" {
		claims["iss"] = s.issuer
	}

	// Add audience claim if configured
	if len(s.audience) > 0 {
		claims["aud"] = s.audience
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("refresh token signing error: %w", err)
	}

	return tokenString, nil
}

// ValidateRefreshToken validates a refresh token and returns user data.
// H1: Added for refresh token support.
func (s *AuthService) ValidateRefreshToken(tokenString string) (*models.Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
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
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Check token type
	tokenType, _ := claims["tokenType"].(string)
	if tokenType != "refresh" {
		return nil, fmt.Errorf("%w: not a refresh token", ErrInvalidToken)
	}

	// Validate audience claim if configured
	if len(s.audience) > 0 {
		audClaim, ok := claims["aud"]
		if !ok {
			return nil, fmt.Errorf("%w: missing audience", ErrInvalidToken)
		}

		// Audience can be string or []string
		var tokenAudiences []string
		switch aud := audClaim.(type) {
		case string:
			tokenAudiences = []string{aud}
		case []interface{}:
			for _, a := range aud {
				if s, ok := a.(string); ok {
					tokenAudiences = append(tokenAudiences, s)
				}
			}
		}

		if len(tokenAudiences) == 0 {
			return nil, fmt.Errorf("%w: invalid audience format", ErrInvalidToken)
		}

		// Check if at least one token audience matches configured audiences
		matched := false
		for _, tokenAud := range tokenAudiences {
			for _, configuredAud := range s.audience {
				if tokenAud == configuredAud {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched {
			return nil, fmt.Errorf("%w: audience not allowed", ErrInvalidToken)
		}
	}

	userID, ok := claims["userId"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("%w: missing userId", ErrInvalidToken)
	}

	username, _ := claims["username"].(string)

	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return nil, fmt.Errorf("%w: missing jti", ErrInvalidToken)
	}

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
