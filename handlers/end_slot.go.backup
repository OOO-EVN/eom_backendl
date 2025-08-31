package handlers

import (
    "database/sql"
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5"
)

func ForceEndShiftHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Получаем параметр userID из URL
        userIDStr := chi.URLParam(r, "userID")

        userID, err := strconv.Atoi(userIDStr)
        if err != nil {
            RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
            return
        }

        var slotID int
        var startTime time.Time
        err = db.QueryRow(`
            SELECT id, start_time 
            FROM slots 
            WHERE user_id = ? AND end_time IS NULL
        `, userID).Scan(&slotID, &startTime)
        if err == sql.ErrNoRows {
            RespondWithError(w, http.StatusNotFound, "No active slot found for the user")
            return
        } else if err != nil {
            RespondWithError(w, http.StatusInternalServerError, "Database error")
            return
        }

        endTime := time.Now()
        duration := int(endTime.Sub(startTime).Seconds())

        _, err = db.Exec(`
            UPDATE slots 
            SET end_time = ?, worked_duration = ? 
            WHERE id = ?
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
