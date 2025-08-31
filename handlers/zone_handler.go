package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

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

		result, err := db.Exec("INSERT OR IGNORE INTO zones (name) VALUES (?)", zone.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to create zone")
			return
		}

		id, _ := result.LastInsertId()
		if id == 0 {
			var existingID int
			db.QueryRow("SELECT id FROM zones WHERE name = ?", zone.Name).Scan(&existingID)
			zone.ID = existingID
		} else {
			zone.ID = int(id)
		}

		RespondWithJSON(w, http.StatusCreated, zone)
	}
}

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

func DeleteZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone ID")
			return
		}

		result, err := db.Exec("DELETE FROM zones WHERE id = ?", id)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete zone")
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "Zone not found")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}
