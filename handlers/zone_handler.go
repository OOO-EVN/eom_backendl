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

		var newID int
		err := db.QueryRow(`
			INSERT INTO zones (name) 
			VALUES ($1) 
			ON CONFLICT (name) DO NOTHING 
			RETURNING id
		`, zone.Name).Scan(&newID)

		if err != nil {
			if err == sql.ErrNoRows {
				// Запись уже существует — получаем её ID
				err = db.QueryRow("SELECT id FROM zones WHERE name = $1", zone.Name).Scan(&newID)
				if err != nil {
					log.Printf("Failed to fetch existing zone ID: %v", err)
					RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve zone")
					return
				}
			} else {
				log.Printf("Database insert error: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create zone")
				return
			}
		}

		zone.ID = newID
		RespondWithJSON(w, http.StatusCreated, zone)
	}
}

func UpdateZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone ID")
			return
		}

		var zone Zone
		if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		result, err := db.Exec("UPDATE zones SET name = $1 WHERE id = $2", zone.Name, id)
		if err != nil {
			log.Printf("Database update error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to update zone")
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "Zone not found")
			return
		}

		zone.ID = id
		RespondWithJSON(w, http.StatusOK, zone)
	}
}

func DeleteZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone ID")
			return
		}

		result, err := db.Exec("DELETE FROM zones WHERE id = $1", id)
		if err != nil {
			log.Printf("Database delete error: %v", err)
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
