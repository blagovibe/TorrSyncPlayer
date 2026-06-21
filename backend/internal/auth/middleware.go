// Package auth provides middleware for JWT authentication.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// contextKey type for context keys.
type contextKey string

const (
	// ClaimsKey key for storing claims in request context.
	ClaimsKey contextKey = "auth_claims"
	// JTIKey key for storing JTI in request context.
	JTIKey contextKey = "auth_jti"
)

// JWTMiddleware creates middleware for JWT token validation.
// Checks presence, validity and revocation of the token.
// Token must be in the format: Bearer <token>
func (s *AuthService) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAuthError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}

		// Check header format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeAuthError(w, http.StatusUnauthorized, "Invalid Authorization header format. Use: Bearer <token>")
			return
		}

		tokenString := parts[1]
		if tokenString == "" {
			writeAuthError(w, http.StatusUnauthorized, "Empty token")
			return
		}

		// Validate token
		claims, err := s.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, ErrExpiredToken) {
				writeAuthError(w, http.StatusUnauthorized, "Token expired")
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Check if token is revoked
		if claims.JTI != "" && s.revocationStore.IsRevoked(claims.JTI) {
			writeAuthError(w, http.StatusUnauthorized, "Token revoked")
			return
		}

		// Add claims and JTI to request context
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		if claims.JTI != "" {
			ctx = context.WithValue(ctx, JTIKey, claims.JTI)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims extracts claims from the request context.
// Returns nil if the user is not authenticated.
func GetClaims(r *http.Request) *models.Claims {
	claims, ok := r.Context().Value(ClaimsKey).(*models.Claims)
	if !ok {
		return nil
	}
	return claims
}

// GetJTI extracts JTI from the request context.
// Returns an empty string if JTI is not found.
func GetJTI(r *http.Request) string {
	jti, ok := r.Context().Value(JTIKey).(string)
	if !ok {
		return ""
	}
	return jti
}

// LogoutHandler handler for revoking a JWT token.
// POST /api/v1/auth/logout
//
// @Summary      Logout
// @Description  Revokes the JWT token (adds to revoked list)
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.SuccessResponse
// @Failure      400  {object}  models.ErrorResponse
// @Failure      401  {object}  models.ErrorResponse
// @Router       /api/v1/auth/logout [post]
func (s *AuthService) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Get JTI from context first (set by JWTMiddleware).
	// This avoids re-parsing the JWT token after middleware already validated it.
	jti := GetJTI(r)
	if jti == "" {
		// Fallback 1: extract from claims in context
		if claims := GetClaims(r); claims != nil && claims.JTI != "" {
			jti = claims.JTI
		} else {
			// Fallback 2: parse token directly (handler called without middleware)
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				tokenString := strings.TrimSpace(authHeader[len("bearer "):])
				if tokenString == "" {
					response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Empty token"})
					return
				}
				jti, _ = s.ExtractJTI(tokenString)
			}
		}
	}

	if jti == "" {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Could not identify token"})
		return
	}

	// Revoke token (use store from AuthService)
	// Set expiration as current time + 24 hours
	// (in case the token has not expired yet)
	s.revocationStore.Revoke(jti, time.Now().Add(constants.RevocationStoreTTL))

	response.WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Token revoked successfully"})
}

// writeAuthError writes an authentication error in JSON format.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="TorrSyncPlayer"`)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
