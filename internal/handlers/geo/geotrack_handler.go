// handlers/geotrack_handler.go

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/internal/models"
	"github.com/evn/eom_backendl/internal/services/geo"
	
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

	userID, ok := r.Context().Value(config.UserIDKey).(int)
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
