package handlers

import (
	"database/sql"
	"net/http"
	"log"
)

// ✅ Исправлено: firstName → first_name
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
