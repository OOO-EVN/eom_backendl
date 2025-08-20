// handlers/zone_handler.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Zone представляет зону
type Zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // теперь это цифра как строка: "1", "2"
}

// GetAvailableZonesHandler возвращает список зон
func GetAvailableZonesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, name FROM zones ORDER BY name")
		if err != nil {
			log.Printf("Database error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var zones []Zone
		for rows.Next() {
			var zone Zone
			if err := rows.Scan(&zone.ID, &zone.Name); err != nil {
				continue
			}
			zones = append(zones, zone)
		}

		RespondWithJSON(w, http.StatusOK, zones)
	}
}

// CreateZoneHandler добавляет новую зону
func CreateZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var zone Zone
		if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if zone.Name == "" {
			RespondWithError(w, http.StatusBadRequest, "Zone name is required")
			return
		}

		result, err := db.Exec("INSERT INTO zones (name) VALUES (?)", zone.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to create zone")
			return
		}

		id, _ := result.LastInsertId()
		zone.ID = int(id)

		RespondWithJSON(w, http.StatusCreated, zone)
	}
}

// UpdateZoneHandler обновляет зону
func UpdateZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))

		var zone Zone
		if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		_, err := db.Exec("UPDATE zones SET name = ? WHERE id = ?", zone.Name, id)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to update zone")
			return
		}

		zone.ID = id
		RespondWithJSON(w, http.StatusOK, zone)
	}
}

// DeleteZoneHandler удаляет зону
func DeleteZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))

		_, err := db.Exec("DELETE FROM zones WHERE id = ?", id)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete zone")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
	}
}
