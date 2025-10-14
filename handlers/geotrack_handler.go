// handlers/geotrack_handler.go

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/models"
	"github.com/evn/eom_backendl/services"
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
		RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	userID, ok := r.Context().Value(config.UserIDKey).(int)
	if !ok {
		RespondWithError(w, http.StatusUnauthorized, "User ID not found in context")
		return
	}
	update.UserID = strconv.Itoa(userID)

	if err := h.service.HandleUpdate(r.Context(), &update); err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to save location")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *GeoTrackHandler) GetLast(w http.ResponseWriter, r *http.Request) {
	locations, err := h.service.GetLastLocations(r.Context())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "DB error")
		return
	}
	RespondWithJSON(w, http.StatusOK, locations)
}
