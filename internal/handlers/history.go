// handlers/history.go
package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/go-chi/chi/v5"
)

func GetHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user_id, err := strconv.Atoi(chi.URLParam(r, "user"))
		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		rows, err := db.Query(`
			SELECT lat, lng, timestamp FROM location_history 
			WHERE user_id = $1 
			ORDER BY timestamp DESC LIMIT 100
		`, user_id)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "DB error")
			return
		}
		defer rows.Close()

		var history []map[string]interface{}
		for rows.Next() {
			var lat, lng float64
			var timestamp time.Time
			if err := rows.Scan(&lat, &lng, &timestamp); err != nil {
				continue
			}
			history = append(history, map[string]interface{}{
				"lat":       lat,
				"lng":       lng,
				"timestamp": timestamp,
			})
		}
		response.RespondWithJSON(w, http.StatusOK, history)
	}
}
