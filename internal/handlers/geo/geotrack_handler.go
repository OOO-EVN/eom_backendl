// handlers/geotrack_handler.go

package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/evn/eom_backendl/internal/middleware"
	"github.com/evn/eom_backendl/internal/models"
	services "github.com/evn/eom_backendl/internal/services/geo"

	"github.com/evn/eom_backendl/internal/pkg/response"
)

type GeoTrackHandler struct {
	service *services.GeoTrackService
}

func NewGeoTrackHandler(service *services.GeoTrackService) *GeoTrackHandler {
	return &GeoTrackHandler{service: service}
}

func (h *GeoTrackHandler) PostGeo(w http.ResponseWriter, r *http.Request) {
	var update models.GeoUpdate

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		response.RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDContextKey).(int)
	if !ok {
		response.RespondWithError(w, http.StatusUnauthorized, "User ID not found in context")
		return
	}
	update.UserID = strconv.Itoa(userID)

	if err := h.service.HandleUpdate(r.Context(), &update); err != nil {
		response.RespondWithError(w, http.StatusInternalServerError, "Failed to save location")
		return
	}

	response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *GeoTrackHandler) GetLast(w http.ResponseWriter, r *http.Request) {
	locations, err := h.service.GetLastLocations(r.Context())
	if err != nil {
		response.RespondWithError(w, http.StatusInternalServerError, "DB error")
		return
	}
	response.RespondWithJSON(w, http.StatusOK, locations)
}

func (h *GeoTrackHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if userID == "" || fromStr == "" || toStr == "" {
		response.RespondWithError(w, http.StatusBadRequest, "Missing required query params: user_id, from, to")
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		response.RespondWithError(w, http.StatusBadRequest, "Invalid 'from' timestamp (use RFC3339)")
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		response.RespondWithError(w, http.StatusBadRequest, "Invalid 'to' timestamp (use RFC3339)")
		return
	}

	if from.After(to) {
		response.RespondWithError(w, http.StatusBadRequest, "'from' must be before 'to'")
		return
	}

	history, err := h.service.GetHistory(r.Context(), userID, from, to)
	if err != nil {
		log.Printf("❌ Failed to fetch history for user %s: %v", userID, err)
		response.RespondWithError(w, http.StatusInternalServerError, "DB error")
		return
	}

	// Преобразуем в формат для фронтенда (если нужно)
	// GeoUpdate уже содержит всё нужное: Lat, Lon, Battery, CreatedAt → ts
	response.RespondWithJSON(w, http.StatusOK, history)
}
