package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/go-chi/jwtauth/v5"
)

type ProfileHandler struct {
	db *sql.DB
}

func NewProfileHandler(db *sql.DB) *ProfileHandler {
	return &ProfileHandler{db: db}
}

func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	// Проверяем, что user_id есть и является строкой
	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Invalid user ID in token")
		return
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  string
		TelegramID sql.NullInt64
		Role       string
		AvatarURL  sql.NullString
	}

	err = h.db.QueryRow(`
		SELECT id, username, first_name, telegram_id, role, avatar_url 
		FROM users 
		WHERE id = ?`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.FirstName,
		&user.TelegramID,
		&user.Role,
		&user.AvatarURL,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "User not found")
		} else {
			RespondWithError(w, http.StatusInternalServerError, "Database error")
		}
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"firstName":  user.FirstName,
		"telegramId": nullInt64ToInterface(user.TelegramID),
		"role":       user.Role,
		"avatarUrl":  nullStringToInterface(user.AvatarURL),
		"position":   "Скаут", // фиксированное значение
	})
}

// Вспомогательные функции для обработки sql.Null*
func nullInt64ToInterface(n sql.NullInt64) interface{} {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullStringToInterface(s sql.NullString) interface{} {
	if s.Valid {
		return s.String
	}
	return nil
}
