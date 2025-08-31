// handlers/shift_generator.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type GenerateShiftsRequest struct {
	Date         string `json:"date"`
	MorningCount int    `json:"morning_count"`
	EveningCount int    `json:"evening_count"`
	ScoutIDs     []int  `json:"scout_ids"`
}

func GenerateShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GenerateShiftsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD")
			return
		}

		validScouts, err := filterAvailableScouts(db, req.ScoutIDs, date)
		if err != nil {
			log.Printf("Error checking scout availability: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		if len(validScouts) < req.MorningCount+req.EveningCount {
			RespondWithError(w, http.StatusBadRequest, "Недостаточно доступных скаутов")
			return
		}

		tx, err := db.Begin()
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Transaction error")
			return
		}
		defer tx.Rollback()

		slotTime := "07:00 - 15:00"
		for i := 0; i < req.MorningCount; i++ {
			if i >= len(validScouts) {
				break
			}
			if err := createSlot(tx, validScouts[i], date, slotTime); err != nil {
				RespondWithError(w, http.StatusInternalServerError, "Failed to create morning shift")
				return
			}
		}

		slotTime = "15:00 - 23:00"
		for i := req.MorningCount; i < req.MorningCount+req.EveningCount; i++ {
			if i >= len(validScouts) {
				break
			}
			if err := createSlot(tx, validScouts[i], date, slotTime); err != nil {
				RespondWithError(w, http.StatusInternalServerError, "Failed to create evening shift")
				return
			}
		}

		if err := tx.Commit(); err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Commit error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "Смены сгенерированы",
		})
	}
}

func filterAvailableScouts(db *sql.DB, scoutIDs []int, date time.Time) ([]int, error) {
	if len(scoutIDs) == 0 {
		return []int{}, nil
	}

	placeholders := make([]string, len(scoutIDs))
	args := make([]interface{}, len(scoutIDs))
	for i, id := range scoutIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	args = append(args, date.Format("2006-01-02"))

	query := fmt.Sprintf(`
		SELECT user_id FROM slots 
		WHERE user_id IN (%s) 
		AND DATE(start_time) = ?
		AND end_time IS NULL
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	busy := make(map[int]bool)
	for rows.Next() {
		var id int
		rows.Scan(&id)
		busy[id] = true
	}

	var available []int
	for _, id := range scoutIDs {
		if !busy[id] {
			available = append(available, id)
		}
	}
	return available, nil
}

func createSlot(tx *sql.Tx, userID int, date time.Time, slotTime string) error {
	_, err := tx.Exec(`
		INSERT INTO slots (user_id, start_time, slot_time_range, position, zone, selfie_path) 
		VALUES (?, ?, ?, 'Скаут', 'Центр', '')
	`, userID, date.Format("2006-01-02 07:00:00"), slotTime)
	return err
}
