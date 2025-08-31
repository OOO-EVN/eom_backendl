// handlers/user.go
package handlers

import (
	"database/sql"
	"net/http"
	"log"
	"time"

	"github.com/evn/eom_backendl/services"
)

type UserAPIResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}

func ListAdminUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, username, first_name, role, is_active
			FROM users
			ORDER BY id
		`)
		if err != nil {
			log.Printf("Failed to fetch users: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to fetch users")
			return
		}
		defer rows.Close()

		var users []UserAPIResponse
		for rows.Next() {
			var u struct {
				ID        int
				Username  string
				FirstName sql.NullString
				Role      string
				IsActive  bool
			}
			if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.Role, &u.IsActive); err != nil {
				log.Printf("Scan error: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Scan error")
				return
			}
			users = append(users, UserAPIResponse{
				ID:        u.ID,
				Username:  u.Username,
				FirstName: nullStringOrEmpty(u.FirstName),
				Role:      u.Role,
				IsActive:  u.IsActive,
			})
		}

		if err := rows.Err(); err != nil {
			log.Printf("Rows error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Rows error")
			return
		}

		RespondWithJSON(w, http.StatusOK, users)
	}
}

func nullStringOrEmpty(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

type LocationResponse struct {
	UserID    int     `json:"user_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Timestamp string  `json:"timestamp"`
}

// ✅ Экспортируем как функцию пакета handlers
func GetOnlineUsersHandler(store *services.RedisStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locations, err := store.GetAllLocations()
		if err != nil {
			log.Printf("Redis error in GetAllLocations: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to get online users")
			return
		}

		var responses []LocationResponse
		for _, loc := range locations {
			responses = append(responses, LocationResponse{
				UserID:    loc.UserID,
				Lat:       loc.Lat,
				Lng:       loc.Lng,
				Timestamp: loc.Timestamp.UTC().Format(time.RFC3339),
			})
		}

		RespondWithJSON(w, http.StatusOK, responses)
	}
}
