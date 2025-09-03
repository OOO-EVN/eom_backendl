package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"
)

func AutoEndShifts(db *sql.DB) (int, error) {
	query := `
		SELECT s.id, s.user_id, s.slot_time_range, s.start_time 
		FROM slots s
		WHERE s.end_time IS NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("DB query error (active slots): %v", err)
		return 0, err
	}
	defer rows.Close()

	var toEnd []struct {
		ID     int
		UserID int
	}

	for rows.Next() {
		var id, userID int
		var slotTimeRange string
		var startTime time.Time

		if err := rows.Scan(&id, &userID, &slotTimeRange, &startTime); err != nil {
			log.Printf("Error scanning active slot: %v", err)
			continue
		}

		slotTimeRange = NormalizeSlot(slotTimeRange)

		endTime, err := getEndTimeFromSlot(slotTimeRange)
		if err != nil {
			log.Printf("Invalid slot time range '%s': %v", slotTimeRange, err)
			continue
		}

		if time.Now().After(endTime) {
			toEnd = append(toEnd, struct{ ID, UserID int }{ID: id, UserID: userID})
		}
	}

	if err = rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
		return 0, err
	}

	endedCount := 0
	for _, slot := range toEnd {
		if err := endSlot(db, slot.ID, slot.UserID); err != nil {
			log.Printf("Failed to auto-end slot ID %d: %v", slot.ID, err)
		} else {
			endedCount++
		}
	}

	return endedCount, nil
}

func AutoEndShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		endedCount, err := AutoEndShifts(db)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to process auto-end shifts")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":      "Auto-end shifts completed",
			"slots_ended":  endedCount,
			"processed_at": time.Now().Format(time.RFC3339),
		})
	}
}

func getEndTimeFromSlot(slotTimeRange string) (time.Time, error) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	switch slotTimeRange {
	case "07:00–15:00", "07:00-15:00":
		return time.Parse("2006-01-02 15:04", dateStr+" 15:00")
	case "15:00–23:00", "15:00-23:00":
		return time.Parse("2006-01-02 15:04", dateStr+" 23:00")
	case "07:00–23:00", "07:00-23:00":
		return time.Parse("2006-01-02 15:04", dateStr+" 23:00")
	default:
		return time.Time{}, fmt.Errorf("invalid slot time range: %s", slotTimeRange)
	}
}
func endSlot(db *sql.DB, slotID, userID int) error {
	var startTime time.Time
	err := db.QueryRow("SELECT start_time FROM slots WHERE id = ? AND end_time IS NULL", slotID).Scan(&startTime)
	if err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}

	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Seconds())

	_, err = db.Exec("UPDATE slots SET end_time = ?, worked_duration = ? WHERE id = ?", endTime, duration, slotID)
	return err
}
