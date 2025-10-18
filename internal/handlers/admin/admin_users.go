// handlers/admin_users.go
package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"time"
	"github.com/evn/eom_backendl/internal/pkg/response"

)

// ListAdminUsersHandler возвращает список всех пользователей для админов
func ListAdminUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, username, first_name, role, status, is_active, created_at 
			FROM users 
			ORDER BY created_at DESC
		`)
		if err != nil {
			log.Printf("Database query error: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to fetch users")
			return
		}
		defer rows.Close()

		var users []map[string]interface{}

		for rows.Next() {
			var user struct {
				ID        int            `json:"id"`
				Username  string         `json:"username"`
				FirstName sql.NullString `json:"first_name"`
				Role      string         `json:"role"`
				Status    string         `json:"status"`
				IsActive  bool           `json:"is_active"`
				CreatedAt time.Time      `json:"created_at"`
			}

			err := rows.Scan(
				&user.ID,
				&user.Username,
				&user.FirstName,
				&user.Role,
				&user.Status,
				&user.IsActive,
				&user.CreatedAt,
			)
			if err != nil {
				log.Printf("Error scanning user row: %v", err)
				response.RespondWithError(w, http.StatusInternalServerError, "Failed to read user data")
				return
			}

			firstName := ""
			if user.FirstName.Valid {
				firstName = user.FirstName.String
			}

			users = append(users, map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"first_name": firstName,
				"role":       user.Role,
				"status":     user.Status,
				"is_active":  user.IsActive,
				"created_at": user.CreatedAt.Format(time.RFC3339), // или "2006-01-02 15:04:05"
			})
		}

		// Проверяем ошибки после итерации по rows
		if err = rows.Err(); err != nil {
			log.Printf("Row iteration error: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Data read error")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, users)
	}
}

// Вспомогательная функция для парсинга JSON (может использоваться в других обработчиках)
// func parseJSONBody(r *http.Request, v interface{}) error {
// 	return json.NewDecoder(r.Body).Decode(v)
// }
