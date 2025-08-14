// handlers/profile_handler.go
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
		// УБРАЛИ PhotoURL, так как столбца в БД нет
	}

	// ВЕРНУЛИ оригинальный запрос БЕЗ photo_url
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
		// УБРАЛИ &user.PhotoURL
	)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "User not found")
		} else {
			// log.Printf("Database error in GetProfile: %v", err) // Опционально для отладки
			RespondWithError(w, http.StatusInternalServerError, "Database error")
		}
		return
	}

	// --- Логика определения должности (position) на основе роли (role) ---
	// Эта часть осталась ИЗМЕНЕННОЙ и исправленной
	var position string
	switch user.Role {
	case "scout":
		position = "Скаут"
	case "supervisor":
		position = "Супервайзер"
	case "coordinator":
		position = "Координатор"
	case "superadmin":
		position = "Суперадмин"
	default:
		// Значение по умолчанию
		position = "Стажер"
	}
	// --- Конец логики определения должности ---

	// --- Логика определения аватара ---
	// Так как photo_url убран, используем только avatar_url
	var finalAvatarURL interface{}
	if user.AvatarURL.Valid && user.AvatarURL.String != "" {
		finalAvatarURL = user.AvatarURL.String
	} else {
		finalAvatarURL = nil // или URL аватара по умолчанию
	}
	// --- Конец логики определения аватара ---

	// Возвращаем ИСПРАВЛЕННЫЙ профиль
	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"firstName":  user.FirstName,
		"telegramId": nullInt64ToInterface(user.TelegramID),
		"role":       user.Role,       // Реальная роль
		"avatarUrl":  finalAvatarURL,  // Выбранный аватар (пока только avatar_url)
		"position":   position,        // Исправленная должность на основе роли
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
