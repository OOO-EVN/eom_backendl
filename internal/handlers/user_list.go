package handlers

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/evn/eom_backendl/internal/pkg/response"
)

type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Role      string `json:"role"`
}

func ListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, username, first_name, role FROM users")
		if err != nil {
			log.Printf("Error querying users: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to query users")
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var id int
			var username, firstName, role sql.NullString
			if err := rows.Scan(&id, &username, &firstName, &role); err != nil {
				log.Printf("Error scanning user row: %v", err)
				response.RespondWithError(w, http.StatusInternalServerError, "Failed to process user data")
				return
			}

			user := User{
				ID:        id,
				Username:  username.String,
				FirstName: firstName.String,
				Role:      role.String,
			}
			users = append(users, user)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error after iterating rows: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Error processing rows")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, users)
	}
}
