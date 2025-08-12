package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// UserAPIResponse - структура для ответа API, чтобы избежать конфликта с user_list.go
type UserAPIResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}

// ListAdminUsersHandler возвращает список всех пользователей
func ListAdminUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, username, first_name, role, is_active
			FROM users
			ORDER BY id
		`)
		if err != nil {
			log.Printf("Failed to fetch users: %v", err)
			http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
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
				http.Error(w, "Scan error", http.StatusInternalServerError)
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
			http.Error(w, "Rows error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

// nullStringOrEmpty - вспомогательная функция
func nullStringOrEmpty(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
