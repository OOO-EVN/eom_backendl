// handlers/shift_generator.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
				log.Printf("Failed to create morning shift: %v", err)
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
				log.Printf("Failed to create evening shift: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create evening shift")
				return
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Shift generation commit error: %v", err)
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
	args := make([]interface{}, len(scoutIDs)+1)
	for i, id := range scoutIDs {
		placeholders[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}
	args[len(scoutIDs)] = date.Format("2006-01-02")

	query := fmt.Sprintf(`
		SELECT user_id FROM slots 
		WHERE user_id IN (%s) 
		AND DATE(start_time) = $%d
		AND end_time IS NULL
	`, strings.Join(placeholders, ","), len(scoutIDs)+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	busy := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
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
		VALUES ($1, $2, $3, 'Скаут', 'Центр', '')
	`, userID, date.Format("2006-01-02 07:00:00"), slotTime)
	return err
}

func GetShiftsByDateHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dateStr := chi.URLParam(r, "date")
		if dateStr == "" {
			RespondWithError(w, http.StatusBadRequest, "Date is required")
			return
		}

		_, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD")
			return
		}

		query := `
			SELECT 
				s.id,
				s.user_id,
				u.username,
				u.first_name,
				s.start_time,
				s.slot_time_range,
				s.position,
				s.zone,
				s.selfie_path,
				s.end_time
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE DATE(s.start_time) = $1
			ORDER BY s.start_time
		`

		rows, err := db.Query(query, dateStr)
		if err != nil {
			log.Printf("Error querying shifts for date %s: %v", dateStr, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var id, userID int
			var username, firstName, startTime, slotTimeRange, position, zone, selfie, endTime sql.NullString
			if err := rows.Scan(&id, &userID, &username, &firstName, &startTime, &slotTimeRange, &position, &zone, &selfie, &endTime); err != nil {
				log.Printf("Error scanning shift row: %v", err)
				continue
			}

			shift := map[string]interface{}{
				"id":              id,
				"user_id":         userID,
				"username":        username.String,
				"first_name":      firstName.String,
				"start_time":      startTime.String,
				"shift_type":      getShiftTypeFromTimeRange(slotTimeRange.String),
				"position":        position.String,
				"zone":            zone.String,
				"selfie":          selfie.String,
				"end_time":        endTime.String,
			}
			shifts = append(shifts, shift)
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}

func getShiftTypeFromTimeRange(timeRange string) string {
	if strings.Contains(timeRange, "07:00") {
		return "morning"
	} else if strings.Contains(timeRange, "15:00") {
		return "evening"
	}
	return "unknown"
}