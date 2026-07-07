// SPDX-License-Identifier: MIT

// Package auth provides HTTP handlers for authentication.
package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// AuthHandler authentication handler.
type AuthHandler struct {
	store       *UserStore
	authService *AuthService
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(store *UserStore, authService *AuthService) *AuthHandler {
	return &AuthHandler{
		store:       store,
		authService: authService,
	}
}

// auditLog logs authentication events for security audit trail
func auditLog(event, username, remoteAddr, userAgent string, success bool) {
	slog.Info("AUDIT",
		"event", event,
		"username", username,
		"remote_addr", remoteAddr,
		"user_agent", userAgent,
		"success", success,
		"timestamp", time.Now().Unix(),
	)
}

// Register handler for new user registration.
// Accepts JSON with username and password fields.
// Returns an authentication token on successful registration.
//
// @Summary      Registration
// @Description  Registers a new user and returns a JWT token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.RegisterRequest  true  "Registration data"
// @Success      201      {object}  models.AuthResponse
// @Failure      400      {object}  models.ErrorResponse
// @Router       /api/v1/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > constants.MaxRequestSize {
		response.WriteJSON(w, http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "Request body too large"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestSize)

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Invalid request format"})
		return
	}

	// Create user
	user, err := h.store.Create(req.Username, req.Password)
	if err != nil {
		auditLog("register", req.Username, r.RemoteAddr, r.UserAgent(), false)
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Registration failed"})
		return
	}

	// Generate token
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		auditLog("register", req.Username, r.RemoteAddr, r.UserAgent(), false)
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Token generation error"})
		return
	}

	auditLog("register", req.Username, r.RemoteAddr, r.UserAgent(), true)

	// Return response
	response.WriteJSON(w, http.StatusCreated, models.AuthResponse{
		Token: token,
		User:  user.ToUserResponse(),
	})
}

// Login handler for user login.
// Accepts JSON with username and password fields.
// Returns an authentication token on successful login.
//
// @Summary      Login
// @Description  Authenticates the user and returns a JWT token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.LoginRequest  true  "Login data"
// @Success      200      {object}  models.AuthResponse
// @Failure      401      {object}  models.ErrorResponse
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > constants.MaxRequestSize {
		response.WriteJSON(w, http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "Request body too large"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestSize)

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Invalid request format"})
		return
	}

	// Authenticate user
	user, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		auditLog("login", req.Username, r.RemoteAddr, r.UserAgent(), false)
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Invalid username or password"})
		return
	}

	// Generate token
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		auditLog("login", req.Username, r.RemoteAddr, r.UserAgent(), false)
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Token generation error"})
		return
	}

	auditLog("login", req.Username, r.RemoteAddr, r.UserAgent(), true)

	// Return response
	response.WriteJSON(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User:  user.ToUserResponse(),
	})
}

// ChangePassword handler for changing user password.
// Accepts JSON with currentPassword and newPassword fields.
// Requires JWT authentication.
//
// @Summary      Change password
// @Description  Changes the current user's password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.ChangePasswordRequest  true  "Password change data"
// @Success      200      {object}  models.SuccessResponse
// @Failure      400      {object}  models.ErrorResponse
// @Failure      401      {object}  models.ErrorResponse
// @Router       /api/v1/auth/change-password [post]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > constants.MaxRequestSize {
		response.WriteJSON(w, http.StatusRequestEntityTooLarge, models.ErrorResponse{Error: "Request body too large"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestSize)

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Invalid request format"})
		return
	}

	// Get user ID from context (set by JWT middleware)
	claims := GetClaims(r)
	if claims == nil || claims.UserID == "" {
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Authentication required"})
		return
	}

	// Get user to get username
	user, exists := h.store.GetByID(claims.UserID)
	if !exists {
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "User not found"})
		return
	}

	// Change password
	err := h.store.ChangePassword(user.Username, req.CurrentPassword, req.NewPassword)
	if err != nil {
		auditLog("change_password", user.Username, r.RemoteAddr, r.UserAgent(), false)
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Password change failed"})
		return
	}

	auditLog("change_password", user.Username, r.RemoteAddr, r.UserAgent(), true)
	response.WriteJSON(w, http.StatusOK, models.SuccessResponse{Message: "Password changed successfully"})
}
