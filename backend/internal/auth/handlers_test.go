package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/torrplayer/backend/internal/models"
)

func setupTestHandler() *AuthHandler {
	store := NewUserStore()
	return NewAuthHandler(store)
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
			name: "Успешная регистрация",
			body: models.RegisterRequest{
				Username: "testuser",
				Password: "password123",
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
			name: "Дубликат пользователя",
			body: models.RegisterRequest{
				Username: "testuser",
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Пустое имя пользователя",
			body: models.RegisterRequest{
				Username: "",
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Короткий пароль",
			body: models.RegisterRequest{
				Username: "newuser",
				Password: "123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Невалидный JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	// Первый запрос для создания пользователя
	reqBody, _ := json.Marshal(tests[0].body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.Register(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	// Остальные тесты
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			// Для теста дубликата используем тот же handler
			testHandler := handler
			if tt.name != "Дубликат пользователя" {
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

	// Создаём пользователя
	_, err := handler.store.Create("testuser", "password123")
	require.NoError(t, err)

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "Успешный вход",
			body: models.LoginRequest{
				Username: "testuser",
				Password: "password123",
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
			name: "Неверный пароль",
			body: models.LoginRequest{
				Username: "testuser",
				Password: "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Несуществующий пользователь",
			body: models.LoginRequest{
				Username: "nonexistent",
				Password: "password123",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Невалидный JSON",
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
	// Устанавливаем фиксированный секрет для тестов
	SetSecret([]byte("test-secret-key-for-testing-32bytes!"))

	// Создаём тестовый handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(claims)
	})

	// Оборачиваем в JWT middleware
	handler := JWTMiddleware(testHandler)

	// Создаём тестовый токен
	user := &models.User{
		ID:       "user123",
		Username: "testuser",
	}
	token, err := GenerateToken(user)
	require.NoError(t, err)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Валидный токен",
			authHeader:     "Bearer " + token,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Без заголовка Authorization",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Неверный формат заголовка",
			authHeader:     "Basic " + token,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Пустой токен",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Невалидный токен",
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
	// Тест без claims в контексте
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	claims := GetClaims(req)
	assert.Nil(t, claims)
}
