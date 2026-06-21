package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

func setupTestHandler() *AuthHandler {
	store := NewUserStore()
	authService, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	if err != nil {
		panic(err)
	}
	return NewAuthHandler(store, authService)
}

func TestRegisterHandler(t *testing.T) {
	handler := setupTestHandler()

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "Successful registration",
			body: models.RegisterRequest{
				Username: "testuser",
				Password: "TestPass1!",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var resp models.AuthResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.NotEmpty(t, resp.Token)
				assert.Equal(t, "testuser", resp.User.Username)
			},
		},
		{
			name: "Duplicate user",
			body: models.RegisterRequest{
				Username: "testuser",
				Password: "TestPass1!",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, body []byte) {
				var errResp map[string]string
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)
				assert.Equal(t, "Registration failed", errResp["error"])
			},
		},
		{
			name: "Empty username",
			body: models.RegisterRequest{
				Username: "",
				Password: "TestPass1!",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Short password",
			body: models.RegisterRequest{
				Username: "newuser",
				Password: "123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	// First request to create user
	reqBody, _ := json.Marshal(tests[0].body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.Register(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	// Remaining tests
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			// For duplicate test use the same handler
			testHandler := handler
			if tt.name != "Duplicate user" {
				testHandler = setupTestHandler()
			}
			var reqBody []byte
			if s, ok := tt.body.(string); ok {
				reqBody = []byte(s)
			} else {
				reqBody, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			testHandler.Register(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.Bytes())
			}
		})
	}
}

func TestLoginHandler(t *testing.T) {
	handler := setupTestHandler()

	// Create a user
	_, err := handler.store.Create("testuser", "TestPass1!")
	require.NoError(t, err)

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "Successful login",
			body: models.LoginRequest{
				Username: "testuser",
				Password: "TestPass1!",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp models.AuthResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.NotEmpty(t, resp.Token)
				assert.Equal(t, "testuser", resp.User.Username)
			},
		},
		{
			name: "Wrong password",
			body: models.LoginRequest{
				Username: "testuser",
				Password: "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Non-existent user",
			body: models.LoginRequest{
				Username: "nonexistent",
				Password: "TestPass1!",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody []byte
			if s, ok := tt.body.(string); ok {
				reqBody = []byte(s)
			} else {
				reqBody, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.Login(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.Bytes())
			}
		})
	}
}

func TestJWTMiddleware(t *testing.T) {
	// Create AuthService with fixed secret for tests
	authService, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	require.NoError(t, err)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(claims); err != nil {
			t.Error(err)
		}
	})

	// Wrap in JWT middleware
	handler := authService.JWTMiddleware(testHandler)

	// Create test token
	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid token",
			authHeader:     "Bearer " + token,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid header format",
			authHeader:     "Basic " + token,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Empty token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestGetClaims(t *testing.T) {
	// Test without claims in context
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	claims := GetClaims(req)
	assert.Nil(t, claims)
}

func TestLogoutHandler(t *testing.T) {
	// Create auth service (revocation store is now internal)
	authService, err := NewAuthService([]byte("test-secret-key-for-testing-32bytes!"))
	require.NoError(t, err)

	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}

	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "Successful logout with valid token",
			authHeader:     "Bearer " + token,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp models.SuccessResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "Token revoked successfully", resp.Message)
			},
		},
		{
			name:           "Logout without Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Logout with empty token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			authService.LogoutHandler(rr, req)
			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, rr.Body.Bytes())
			}
		})
	}

}
