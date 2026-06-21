// SPDX-License-Identifier: MIT

// Package auth provides HTTP handlers for authentication.
package auth

import (
	"encoding/json"
	"net/http"

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
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Registration failed"})
		return
	}

	// Generate token
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Token generation error"})
		return
	}

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
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Invalid username or password"})
		return
	}

	// Generate token
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Token generation error"})
		return
	}

	// Return response
	response.WriteJSON(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User:  user.ToUserResponse(),
	})
}
