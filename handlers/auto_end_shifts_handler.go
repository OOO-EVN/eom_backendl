// handlers/auto_end_shifts_handler.go
package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"time"
)

// AutoEndShifts проверяет активные смены и завершает те, что вышли за пределы временного диапазона
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

	var toEnd []struct{ ID, UserID int }

	for rows.Next() {
		var id, userID int
		var slotTimeRange string
		var startTime time.Time

		if err := rows.Scan(&id, &userID, &slotTimeRange, &startTime); err != nil {
			log.Printf("Error scanning active slot: %v", err)
			continue
		}

		// Нормализуем временной слот (убираем лишние пробелы, приводим к стандарту)
		slotTimeRange = NormalizeSlot(slotTimeRange)

		// Получаем текущее время (уже в локальном поясе сервера, например +05:00)
		now := time.Now()

		// Определяем время окончания смены
		var endTime time.Time
		switch slotTimeRange {
		case "07:00-15:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
		case "15:00-23:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
		case "07:00-23:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
		default:
			log.Printf("Invalid slot time range: %s", slotTimeRange)
			continue
		}

		// Если текущее время позже времени окончания — завершаем смену
		if now.After(endTime) {
			toEnd = append(toEnd, struct{ ID, UserID int }{ID: id, UserID: userID})
		}
	}

	if err = rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
		return 0, err
	}

	// Завершаем смены
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

// AutoEndShiftsHandler — HTTP-эндпоинт для ручного вызова (например, для дебага)
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
			"processed_at": time.Now().Format(time.RFC3339), // в локальном времени
		})
	}
}

// endSlot — закрывает одну смену
func endSlot(db *sql.DB, slotID, userID int) error {
	var startTime time.Time
	err := db.QueryRow("SELECT start_time FROM slots WHERE id = ? AND end_time IS NULL", slotID).Scan(&startTime)
	if err == sql.ErrNoRows {
		return nil // смена уже завершена
	} else if err != nil {
		return err
	}

	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Seconds())

	_, err = db.Exec("UPDATE slots SET end_time = ?, worked_duration = ? WHERE id = ?", endTime, duration, slotID)
	return err
}

// NormalizeSlot приводит временной слот к единому формату
/*func NormalizeSlot(slot string) string {
	switch slot {
	case "07:00 - 15:00", "07:00–15:00", "07:00-15:00", "7:00-15:00":
		return "07:00-15:00"
	case "15:00 - 23:00", "15:00–23:00", "15:00-23:00", "15:00-23:00":
		return "15:00-23:00"
	case "07:00 - 23:00", "07:00–23:00", "07:00-23:00":
		return "07:00-23:00"
	default:
		return slot
	}
}
*/
