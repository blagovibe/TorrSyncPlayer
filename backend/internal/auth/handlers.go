// Package auth предоставляет HTTP обработчики для аутентификации.
package auth

import (
	"encoding/json"
	"net/http"

	"github.com/yourname/torrplayer/backend/internal/models"
)

// AuthHandler обработчик аутентификации.
type AuthHandler struct {
	store *UserStore
}

// NewAuthHandler создаёт новый обработчик аутентификации.
func NewAuthHandler(store *UserStore) *AuthHandler {
	return &AuthHandler{store: store}
}

// Register обработчик регистрации нового пользователя.
// Принимает JSON с полями username и password.
// Возвращает токен аутентификации при успешной регистрации.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер тела запроса
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Неверный формат запроса"})
		return
	}

	// Создаём пользователя
	user, err := h.store.Create(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	// Генерируем токен
	token, err := GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Ошибка создания токена"})
		return
	}

	// Возвращаем ответ
	writeJSON(w, http.StatusCreated, models.AuthResponse{
		Token: token,
		User:  *user,
	})
}

// Login обработчик входа в систему.
// Принимает JSON с полями username и password.
// Возвращает токен аутентификации при успешном входе.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер тела запроса
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Неверный формат запроса"})
		return
	}

	// Аутентифицируем пользователя
	user, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Неверное имя пользователя или пароль"})
		return
	}

	// Генерируем токен
	token, err := GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Ошибка создания токена"})
		return
	}

	// Возвращаем ответ
	writeJSON(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User:  *user,
	})
}

// writeJSON записывает JSON ответ с указанным статусом.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
