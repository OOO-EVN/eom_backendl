package handlers

import (
	"database/sql"
	"net/http"
	"log"
)

// UserProfileResponse - структура для ответа с профилем пользователя
type UserProfileResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}
// GetProfileHandler - обработчик для получения профиля пользователя
func GetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем userID из контекста (должен быть установлен middleware аутентификации)
		userIDVal := r.Context().Value("userID")
		if userIDVal == nil {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		userID, ok := userIDVal.(int)
		if !ok {
			RespondWithError(w, http.StatusInternalServerError, "Invalid user ID format")
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
				RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error fetching user %d: %v", userID, err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to fetch user profile")
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

		RespondWithJSON(w, http.StatusOK, response)
	}
}

// AlternativeGetProfileHandler - альтернативная реализация с указателями
func AlternativeGetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDVal := r.Context().Value("userID")
		if userIDVal == nil {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
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
				RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Database error")
			}
			return
		}

		// Если FirstName is nil, будет отправлен null в JSON
		RespondWithJSON(w, http.StatusOK, user)
	}
}
