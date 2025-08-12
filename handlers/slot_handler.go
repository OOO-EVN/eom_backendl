// handlers/slot_handler.go
package handlers

import (
	"database/sql"
//	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/evn/eom_backendl/config"
)

// StartSlotHandler — начинает новый слот
func StartSlotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		// Проверка активного слота
		var activeCount int
		err := db.QueryRow("SELECT COUNT(*) FROM slots WHERE user_id = ? AND end_time IS NULL", userID).Scan(&activeCount)
		if err != nil {
			log.Printf("DB error checking active slot: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if activeCount > 0 {
			RespondWithError(w, http.StatusBadRequest, "Slot already active")
			return
		}

		// ✅ ИСПРАВЛЕНИЕ: Парсим multipart form data.
		// Лимит памяти 10 МБ.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
			RespondWithError(w, http.StatusBadRequest, "Failed to parse form data")
			return
		}

		// ✅ ИСПРАВЛЕНИЕ: Получаем значения полей из multipart-формы.
		slotTimeRange := r.FormValue("slot_time_range")
		position := r.FormValue("position")
		zone := r.FormValue("zone")

		// Проверяем, что поля не пустые
		if slotTimeRange == "" || position == "" || zone == "" {
			RespondWithError(w, http.StatusBadRequest, "Missing slot details")
			return
		}

		// Получаем файл "selfie"
		file, _, err := r.FormFile("selfie")
		if err != nil {
			log.Printf("Failed to get selfie file from form: %v", err)
			RespondWithError(w, http.StatusBadRequest, "Selfie image is required")
			return
		}
		defer file.Close()

		// Проверка MIME-типа
		buff := make([]byte, 512)
		_, err = file.Read(buff)
		if err != nil {
			log.Printf("Error reading file header: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Error reading file")
			return
		}
		file.Seek(0, 0) // Возвращаем указатель файла в начало

		contentType := http.DetectContentType(buff)
		if contentType != "image/jpeg" && contentType != "image/png" {
			RespondWithError(w, http.StatusBadRequest, "Only JPEG and PNG images are allowed")
			return
		}

		// Генерируем имя файла
		ext := ".jpg"
		if contentType == "image/png" {
			ext = ".png"
		}
		filename := fmt.Sprintf("selfie_%d_%d%s", userID, time.Now().Unix(), ext)
		filepath := filepath.Join("./uploads/selfies", filename)

		out, err := os.Create(filepath)
		if err != nil {
			log.Printf("Failed to create file: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Server error")
			return
		}
		defer out.Close()

		_, err = io.Copy(out, file)
		if err != nil {
			log.Printf("Failed to save file: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Server error")
			return
		}

		// Сохраняем в БД
		_, err = db.Exec(`
			INSERT INTO slots (
				user_id, start_time, slot_time_range, position, zone, selfie_path
			) VALUES (?, ?, ?, ?, ?, ?)
		`, userID, time.Now(), slotTimeRange, position, zone, "/uploads/selfies/"+filename)

		if err != nil {
			log.Printf("Failed to insert slot: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]string{
			"message": "Slot started successfully",
		})
	}
}

// EndSlotHandler — завершает активный слот
func EndSlotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var slotID int
		var startTime time.Time
		err := db.QueryRow(`
			SELECT id, start_time FROM slots WHERE user_id = ? AND end_time IS NULL
		`, userID).Scan(&slotID, &startTime)
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusBadRequest, "No active slot found")
			return
		} else if err != nil {
			log.Printf("DB error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		endTime := time.Now()
		duration := int(endTime.Sub(startTime).Seconds())

		_, err = db.Exec(`
			UPDATE slots SET end_time = ?, worked_duration = ? WHERE id = ?
		`, endTime, duration, slotID)

		if err != nil {
			log.Printf("Failed to update slot: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":     "Slot ended",
			"worked_time": formatDuration(int(duration)),
		})
	}
}

// GetActiveSlotHandler — проверяет, активен ли слот
func GetActiveSlotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM slots WHERE user_id = ? AND end_time IS NULL", userID).Scan(&count)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]bool{
			"is_active": count > 0,
		})
	}
}

// GetShiftsHandler — возвращает историю смен
func GetShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		rows, err := db.Query(`
			SELECT 
				start_time, end_time, slot_time_range, position, zone, worked_duration
			FROM slots 
			WHERE user_id = ? AND end_time IS NOT NULL
			ORDER BY start_time DESC
		`, userID)
		if err != nil {
			log.Printf("Error querying shifts: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to query shifts")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var startTime, endTime time.Time
			var slotTimeRange, position, zone sql.NullString
			var workedDuration sql.NullInt64

			if err := rows.Scan(&startTime, &endTime, &slotTimeRange, &position, &zone, &workedDuration); err != nil {
				log.Printf("Error scanning shift: %v", err)
				continue
			}

			shift := map[string]interface{}{
				"date":             startTime.Format("2006-01-02"),
				"selected_slot":    slotTimeRange.String,
				"worked_time":      formatDuration(int(workedDuration.Int64)),
				"work_period":      fmt.Sprintf("%s–%s", startTime.Format("15:04"), endTime.Format("15:04")),
				"transport_status": "Транспорт не указан",
				"new_tasks":        0,
			}
			shifts = append(shifts, shift)
		}

		// ✅ ИСПРАВЛЕНИЕ: Убедитесь, что пустой массив правильно сериализуется
		if shifts == nil {
			shifts = []map[string]interface{}{}
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}

func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "0 мин"
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%d ч %d мин", hours, mins)
	}
	return fmt.Sprintf("%d мин", mins)
}
