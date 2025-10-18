package handlers

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/evn/eom_backendl/internal/pkg/response"
)

// GetActiveShiftsForAllHandler возвращает активные смены всех пользователей.
func GetActiveShiftsForAllHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT s.id, s.user_id, u.username, s.start_time, s.slot_time_range, s.position, s.zone, s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.end_time IS NULL
		`)
		if err != nil {
			log.Printf("DB query error: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var id, userID int
			var username, startTime, slotTimeRange, position, zone, selfie string
			if err := rows.Scan(&id, &userID, &username, &startTime, &slotTimeRange, &position, &zone, &selfie); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			shifts = append(shifts, map[string]interface{}{
				"id":              id,
				"user_id":         userID,
				"username":        username,
				"start_time":      startTime,
				"slot_time_range": slotTimeRange,
				"position":        position,
				"zone":            zone,
				"selfie":          selfie,
			})
		}
		response.RespondWithJSON(w, http.StatusOK, shifts)
	}
}
