// Package auth предоставляет HTTP обработчики для аутентификации.
package auth

import (
	"encoding/json"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/response"
)

// AuthHandler обработчик аутентификации.
type AuthHandler struct {
	store       *UserStore
	authService *AuthService
}

// NewAuthHandler создаёт новый обработчик аутентификации.
func NewAuthHandler(store *UserStore, authService *AuthService) *AuthHandler {
	return &AuthHandler{
		store:       store,
		authService: authService,
	}
}

// Register обработчик регистрации нового пользователя.
// Принимает JSON с полями username и password.
// Возвращает токен аутентификации при успешной регистрации.
//
// @Summary      Регистрация
// @Description  Регистрирует нового пользователя и возвращает JWT токен
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.RegisterRequest  true  "Данные для регистрации"
// @Success      201      {object}  models.AuthResponse
// @Failure      400      {object}  models.ErrorResponse
// @Router       /api/v1/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер тела запроса
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestSize) // 1MB

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Неверный формат запроса"})
		return
	}

	// Создаём пользователя
	user, err := h.store.Create(req.Username, req.Password)
	if err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	// Генерируем токен
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Ошибка создания токена"})
		return
	}

	// Возвращаем ответ
	response.WriteJSON(w, http.StatusCreated, models.AuthResponse{
		Token: token,
		User:  *user,
	})
}

// Login обработчик входа в систему.
// Принимает JSON с полями username и password.
// Возвращает токен аутентификации при успешном входе.
//
// @Summary      Вход
// @Description  Аутентифицирует пользователя и возвращает JWT токен
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.LoginRequest  true  "Данные для входа"
// @Success      200      {object}  models.AuthResponse
// @Failure      401      {object}  models.ErrorResponse
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер тела запроса
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestSize)

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, models.ErrorResponse{Error: "Неверный формат запроса"})
		return
	}

	// Аутентифицируем пользователя
	user, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		response.WriteJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Неверное имя пользователя или пароль"})
		return
	}

	// Генерируем токен
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Ошибка создания токена"})
		return
	}

	// Возвращаем ответ
	response.WriteJSON(w, http.StatusOK, models.AuthResponse{
		Token: token,
		User:  *user,
	})
}
