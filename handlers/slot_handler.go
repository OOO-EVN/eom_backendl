package handlers

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
    "crypto/rand"
    _ "image/jpeg"
    _ "image/png"
    "github.com/evn/eom_backendl/config"
)

func generateSafeFilename(userID int, ext string) string {
    randomBytes := make([]byte, 8)
    if _, err := rand.Read(randomBytes); err != nil {
        return fmt.Sprintf("selfie_%d_%d%s", userID, time.Now().UnixNano(), ext)
    }
    hash := fmt.Sprintf("%x", randomBytes)
    return fmt.Sprintf("selfie_%d_%s%s", userID, hash, ext)
}

func StartSlotHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID, ok := r.Context().Value(config.UserIDKey).(int)
        if !ok {
            RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
            return
        }

        var activeCount int
        err := db.QueryRow("SELECT COUNT(*) FROM slots WHERE user_id = ? AND end_time IS NULL", userID).Scan(&activeCount)
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Database error")
            return
        }
        if activeCount > 0 {
            RespondWithError(w, http.StatusBadRequest, "Slot already active")
            return
        }

        // Получаем роль пользователя как позицию
        var position string
        err = db.QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&position)
        if err != nil {
            // Если роль не найдена, используем значение по умолчанию
            position = "user"
        }
        
        // Преобразуем роль в читаемую позицию
        positionMap := map[string]string{
            "superadmin":   "Суперадмин",
            "admin":        "Администратор",
            "coordinator":  "Координатор",
            "scout":        "Скаут",
            "user":         "Пользователь",
        }
        
        if readablePosition, exists := positionMap[position]; exists {
            position = readablePosition
        } else {
            position = "Сотрудник"
        }

        if err := r.ParseMultipartForm(5 << 20); err != nil {
            RespondWithError(w, http.StatusBadRequest, "File too large or malformed")
            return
        }

        slotTimeRange := r.FormValue("slot_time_range")
        zone := r.FormValue("zone")
        if slotTimeRange == "" || zone == "" {
            RespondWithError(w, http.StatusBadRequest, "Missing required fields")
            return
        }

        // Обработка зоны - если это JSON, извлекаем только имя
        if len(zone) > 0 && zone[0] == '{' {
            // Извлекаем имя зоны из JSON
            nameStart := strings.Index(zone, "name:")
            if nameStart != -1 {
                nameStart += 5
                for nameStart < len(zone) && (zone[nameStart] == ' ' || zone[nameStart] == '\t') {
                    nameStart++
                }
                var zoneName string
                if nameStart < len(zone) {
                    if zone[nameStart] == '"' {
                        nameStart++
                        nameEnd := strings.Index(zone[nameStart:], "\"")
                        if nameEnd != -1 {
                            zoneName = zone[nameStart : nameStart+nameEnd]
                        }
                    } else {
                    nameEnd := nameStart
                        for nameEnd < len(zone) && zone[nameEnd] != '}' && zone[nameEnd] != ',' {
                            nameEnd++
                        }
                        zoneName = strings.TrimSpace(zone[nameStart:nameEnd])
                    }
                }
                if zoneName != "" {
                    zone = zoneName
                }
            }
        }

        file, _, err := r.FormFile("selfie")
        if err != nil {
            RespondWithError(w, http.StatusBadRequest, "Selfie image is required")
            return
        }
        defer file.Close()

        buff := make([]byte, 512)
        _, err = file.Read(buff)
        if err != nil && err != io.EOF {
            RespondWithError(w, http.StatusInternalServerError, "Error reading file")
            return
        }

        contentType := http.DetectContentType(buff)
        if contentType != "image/jpeg" && contentType != "image/png" {
            RespondWithError(w, http.StatusBadRequest, "Only JPEG and PNG images allowed")
            return
        }

        file.Seek(0, 0)

        ext := ".jpg"
        if contentType == "image/png" {
            ext = ".png"
        }

        filename := generateSafeFilename(userID, ext)
        filepath := filepath.Join("./uploads/selfies", filename)

        if err := os.MkdirAll("./uploads/selfies", 0755); err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Server error")
            return
        }

        out, err := os.Create(filepath)
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Server error")
            return
        }
        defer out.Close()

        _, err = io.Copy(out, file)
        if err != nil {
            os.Remove(filepath)
            RespondWithError(w, http.StatusInternalServerError, "Failed to save image")
            return
        }

        result, err := db.Exec(`
            INSERT INTO slots (user_id, start_time, slot_time_range, position, zone, selfie_path)
            VALUES (?, ?, ?, ?, ?, ?)
        `, userID, time.Now(), slotTimeRange, position, zone, "/uploads/selfies/"+filename)

        if err != nil {
            os.Remove(filepath)
            RespondWithError(w, http.StatusInternalServerError, "Database error: "+err.Error())
            return
        }

        slotID, err := result.LastInsertId()
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Failed to get slot ID")
            return
        }

        RespondWithJSON(w, http.StatusCreated, map[string]interface{}{
            "message":           "Slot started successfully",
            "selfie":            "/uploads/selfies/" + filename,
            "id":                slotID,
            "user_id":           userID,
            "slot_time_range":   slotTimeRange,
            "position":          position,
            "zone":              zone,
            "start_time":        time.Now().Format(time.RFC3339),
        })
    }
}

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
            RespondWithError(w, http.StatusInternalServerError, "Database error")
            return
        }

        endTime := time.Now()
        duration := int(endTime.Sub(startTime).Seconds())

        _, err = db.Exec(`
            UPDATE slots SET end_time = ?, worked_duration = ? WHERE id = ?
        `, endTime, duration, slotID)

        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Database error")
            return
        }

        RespondWithJSON(w, http.StatusOK, map[string]interface{}{
            "message":     "Slot ended",
            "worked_time": formatDuration(duration),
        })
    }
}

func GetActiveShiftsHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")

        rows, err := db.Query(`
            SELECT 
                s.id,
                s.user_id,
                u.username,
                s.slot_time_range,
                s.position,
                s.zone,
                s.start_time,
                s.selfie_path
            FROM slots s
            JOIN users u ON s.user_id = u.id
            WHERE s.end_time IS NULL
        `)
        if err != nil {
            http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        var shifts []map[string]interface{}

        for rows.Next() {
            var id, userID int
            var username, slotTimeRange, position, zone, selfiePath string
            var startTime time.Time

            if err := rows.Scan(&id, &userID, &username, &slotTimeRange, &position, &zone, &startTime, &selfiePath); err != nil {
                http.Error(w, `{"error":"Error processing data"}`, http.StatusInternalServerError)
                return
            }

            shifts = append(shifts, map[string]interface{}{
                "id":              id,
                "user_id":         userID,
                "username":        username,
                "slot_time_range": slotTimeRange,
                "position":        position,
                "zone":            zone,
                "start_time":      startTime,
                "is_active":       true,
                "selfie":          selfiePath,
            })
        }

        if shifts == nil {
            shifts = []map[string]interface{}{}
        }

        json.NewEncoder(w).Encode(shifts)
    }
}

func GetUserActiveShiftHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID, ok := r.Context().Value(config.UserIDKey).(int)
        if !ok {
            RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
            return
        }

        w.Header().Set("Content-Type", "application/json")

        var id int
        var username, slotTimeRange, position, zone, selfiePath string
        var startTime time.Time

        err := db.QueryRow(`
            SELECT 
                s.id,
                u.username,
                s.slot_time_range,
                s.position,
                s.zone,
                s.start_time,
                s.selfie_path
            FROM slots s
            JOIN users u ON s.user_id = u.id
            WHERE s.user_id = ? AND s.end_time IS NULL
        `, userID).Scan(&id, &username, &slotTimeRange, &position, &zone, &startTime, &selfiePath)

        if err == sql.ErrNoRows {
            w.Write([]byte("null"))
            return
        } else if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Database error")
            return
        }

        activeShift := map[string]interface{}{
            "id":              id,
            "user_id":         userID,
            "username":        username,
            "slot_time_range": slotTimeRange,
            "position":        position,
            "zone":            zone,
            "start_time":      startTime.Format(time.RFC3339),
            "is_active":       true,
            "selfie":          selfiePath,
        }

        json.NewEncoder(w).Encode(activeShift)
    }
}

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

        if shifts == nil {
            shifts = []map[string]interface{}{}
        }

        RespondWithJSON(w, http.StatusOK, shifts)
    }
}

func GetAvailablePositionsHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID, ok := r.Context().Value(config.UserIDKey).(int)
        if !ok {
            RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
            return
        }

        var role string
        err := db.QueryRow("SELECT role FROM users WHERE id = ?", userID).Scan(&role)
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Failed to load user role")
            return
        }

        // Преобразуем роль в читаемую позицию
        positionMap := map[string]string{
            "superadmin":   "Суперадмин",
            "admin":        "Администратор",
            "coordinator":  "Координатор",
            "scout":        "Скаут",
            "user":         "Пользователь",
        }
        
        position := "Сотрудник"
        if readablePosition, exists := positionMap[role]; exists {
            position = readablePosition
        }

        RespondWithJSON(w, http.StatusOK, []string{position})
    }
}

func GetAvailableTimeSlotsHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var timeSlots []string
        rows, err := db.Query("SELECT slot_time_range FROM available_time_slots")
        if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Failed to load time slots")
            return
        }
        defer rows.Close()

        for rows.Next() {
            var timeSlot string
            if err := rows.Scan(&timeSlot); err != nil {
                continue
            }
            timeSlots = append(timeSlots, timeSlot)
        }

        RespondWithJSON(w, http.StatusOK, timeSlots)
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

/*func RespondWithError(w http.ResponseWriter, code int, message string) {
  //  w.Header().Set("Content-Type", "application/json")
   // w.WriteHeader(code)
    ///json.NewEncoder(w).Encode(map[string]string{"error": message})
//}

/func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(payload)
}*/
