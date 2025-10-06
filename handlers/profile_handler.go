// handlers/profile.go
package handlers

import (
	"database/sql"
	"log"
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

	var userID int
	if rawID, ok := claims["user_id"]; ok {
		switch v := rawID.(type) {
		case float64:
			userID = int(v)
		case int:
			userID = v
		case string:
			if id, err := strconv.Atoi(v); err == nil {
				userID = id
			} else {
				RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
				return
			}
		default:
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID type")
			return
		}
	} else {
		RespondWithError(w, http.StatusBadRequest, "User ID not found in token")
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  sql.NullString
		TelegramID sql.NullInt64
		Role       string
		AvatarURL  sql.NullString
		Zone       sql.NullString
		Status     string
		IsActive   bool
	}

	err = h.db.QueryRow(`
		SELECT id, username, first_name, telegram_id, role, avatar_url, zone, status, is_active
		FROM users
		WHERE id = $1`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.FirstName,
		&user.TelegramID,
		&user.Role,
		&user.AvatarURL,
		&user.Zone,
		&user.Status,
		&user.IsActive,
	)

	if err != nil {
		log.Printf("Database error in GetProfile (user %d): %v", userID, err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	firstName := ""
	if user.FirstName.Valid {
		firstName = user.FirstName.String
	}

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
	case "courier":
		position = "Курьер"
	default:
		position = "Стажер"
	}

	var finalAvatarURL interface{}
	if user.AvatarURL.Valid && user.AvatarURL.String != "" {
		finalAvatarURL = user.AvatarURL.String
	} else {
		finalAvatarURL = nil
	}

	var finalZone interface{}
	if user.Zone.Valid && user.Zone.String != "" {
		finalZone = user.Zone.String
	} else {
		finalZone = "Центр"
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"firstName":  firstName,
		"telegramId": nullInt64ToInterface(user.TelegramID),
		"role":       user.Role,
		"avatarUrl":  finalAvatarURL,
		"position":   position,
		"zone":       finalZone,
		"status":     user.Status,
		"is_active":  user.IsActive,
	})
}

func nullInt64ToInterface(n sql.NullInt64) interface{} {
	if n.Valid {
		return n.Int64
	}
	return nil
}