// handlers/force_end_shift.go
package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/evn/eom_backendl/internal/pkg/response"
)

func ForceEndShiftHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		var slotID int
		var startTime time.Time
		err = db.QueryRow(`
            SELECT id, start_time 
            FROM slots 
            WHERE user_id = $1 AND end_time IS NULL
        `, userID).Scan(&slotID, &startTime)
		if err == sql.ErrNoRows {
			response.RespondWithError(w, http.StatusNotFound, "No active slot found for the user")
			return
		} else if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		endTime := time.Now()
		duration := int(endTime.Sub(startTime).Seconds())

		_, err = db.Exec(`
            UPDATE slots 
            SET end_time = $1, worked_duration = $2 
            WHERE id = $3
        `, endTime, duration, slotID)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":     "Slot ended",
			"worked_time": response.FormatDuration(duration),
		})
	}
}
