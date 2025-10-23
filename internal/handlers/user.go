package handlers

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/evn/eom_backendl/internal/pkg/response"
)

// UserProfileResponse - структура для ответа с профилем пользователя
type UserProfileResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}

func (u UserProfileResponse) RespondWithJSON(w http.ResponseWriter, k int, response UserProfileResponse) {
	panic("unimplemented")
}

// GetProfileHandler - обработчик для получения профиля пользователя
func GetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем userID из контекста (должен быть установлен middleware аутентификации)
		userIDVal := r.Context().Value("user_id")
		if userIDVal == nil {
			response.RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		userID, ok := userIDVal.(int)
		if !ok {
			response.RespondWithError(w, http.StatusInternalServerError, "Invalid user ID format")
			return
		}

		// Структура для сканирования из БД с поддержкой NULL
		var user struct {
			ID        int            `json:"id"`
			Username  string         `json:"username"`
			FirstName sql.NullString `json:"first_name"`
			Role      string         `json:"role"`
			IsActive  bool           `json:"is_active"`
		}

		// Выполняем запрос к базе данных
		err := db.QueryRowContext(r.Context(),
			"SELECT id, username, first_name, role, is_active FROM users WHERE id = $1",
			userID).
			Scan(&user.ID, &user.Username, &user.FirstName, &user.Role, &user.IsActive)

		if err != nil {
			if err == sql.ErrNoRows {
				response.RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error fetching user %d: %v", userID, err)
				response.RespondWithError(w, http.StatusInternalServerError, "Failed to fetch user profile")
			}
			return
		}

		// Преобразуем NULL в пустую строку для ответа
		firstName := ""
		if user.FirstName.Valid {
			firstName = user.FirstName.String
		}

		// Формируем ответ
		response := UserProfileResponse{
			ID:        user.ID,
			Username:  user.Username,
			FirstName: firstName,
			Role:      user.Role,
			IsActive:  user.IsActive,
		}

		response.RespondWithJSON(w, http.StatusOK, response)
	}
}

// AlternativeGetProfileHandler - альтернативная реализация с указателями
func AlternativeGetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDVal := r.Context().Value("userID")
		if userIDVal == nil {
			response.RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		userID := userIDVal.(int)

		var user struct {
			ID        int     `json:"id"`
			Username  string  `json:"username"`
			FirstName *string `json:"first_name"`
			Role      string  `json:"role"`
			IsActive  bool    `json:"is_active"`
		}

		err := db.QueryRowContext(r.Context(),
			"SELECT id, username, first_name, role, is_active FROM users WHERE id = $1",
			userID).
			Scan(&user.ID, &user.Username, &user.FirstName, &user.Role, &user.IsActive)

		if err != nil {
			if err == sql.ErrNoRows {
				response.RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error: %v", err)
				response.RespondWithError(w, http.StatusInternalServerError, "Database error")
			}
			return
		}

		// Если FirstName is nil, будет отправлен null в JSON
		response.RespondWithJSON(w, http.StatusOK, user)
	}
}
