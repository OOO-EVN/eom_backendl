package handlers

import (
	"database/sql"
	"log"
	"net/http"
)

type EndedShift struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	SlotTimeRange string `json:"slot_time_range"`
	Position      string `json:"position"`
	Zone          string `json:"zone"`
	Selfie        string `json:"selfie"`
}

func GetEndedShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := `
			SELECT s.id, s.user_id, u.username, s.start_time, s.end_time, 
			       s.slot_time_range, s.position, s.zone, s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.end_time IS NOT NULL
			ORDER BY s.end_time DESC
		`

		rows, err := db.Query(query)
		if err != nil {
			log.Printf("DB query error (ended shifts): %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var shifts []EndedShift
		for rows.Next() {
			var shift EndedShift
			var endTime sql.NullString
			err := rows.Scan(
				&shift.ID,
				&shift.UserID,
				&shift.Username,
				&shift.StartTime,
				&endTime,
				&shift.SlotTimeRange,
				&shift.Position,
				&shift.Zone,
				&shift.Selfie,
			)
			if err != nil {
				log.Printf("Error scanning ended shift row: %v", err)
				continue
			}
			shift.EndTime = endTime.String
			shifts = append(shifts, shift)
		}

		if err = rows.Err(); err != nil {
			log.Printf("Row iteration error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}
