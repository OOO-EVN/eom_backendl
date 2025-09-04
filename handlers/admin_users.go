package handlers

import (
    "database/sql"
    "encoding/json"
    "net/http"
)

// ListAdminUsersHandler - обработчик для получения списка пользователей (для админов)
func ListAdminUsersHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        rows, err := db.Query(`
            SELECT id, username, first_name, role, status, is_active, created_at 
            FROM users 
            ORDER BY created_at DESC
        `)
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Database error: "+err.Error())
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
                CreatedAt string         `json:"created_at"`
            }

            err := rows.Scan(&user.ID, &user.Username, &user.FirstName, &user.Role, &user.Status, &user.IsActive, &user.CreatedAt)
            if err != nil {
                continue
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
                "created_at": user.CreatedAt,
            })
        }

        RespondWithJSON(w, http.StatusOK, users)
    }
}

func parseJSONBody(r *http.Request, v interface{}) error {
    return json.NewDecoder(r.Body).Decode(v)
}
