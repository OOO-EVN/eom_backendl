package promo

import (
	"database/sql"
	"encoding/json"
	"net/http"

	// "strconv"

	"github.com/evn/eom_backendl/internal/middleware"
	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/evn/eom_backendl/internal/repositories"
	"github.com/go-chi/chi/v5"
)

type PromoHandlers struct {
	repo *repositories.PromoRepository
}

func NewPromoHandlers(db *sql.DB) *PromoHandlers {
	return &PromoHandlers{
		repo: repositories.NewPromoRepository(db),
	}
}

// GetDailyPromoCodesHandler — для пользовательского экрана
func GetDailyPromoCodesHandler(db *sql.DB) http.HandlerFunc {
	h := NewPromoHandlers(db)
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok {
			response.RespondWithError(w, http.StatusUnauthorized, "Не авторизован")
			return
		}

		promos, err := h.repo.GetDailyPromos()
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка загрузки промокодов")
			return
		}

		claimed, err := h.repo.GetUserClaimedPromoIDs(userID)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка загрузки статусов")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"promos":  promos,
			"claimed": claimed,
		})
	}
}

// ClaimDailyPromoHandler — пользователь сам получает
func ClaimDailyPromoHandler(db *sql.DB) http.HandlerFunc {
	h := NewPromoHandlers(db)
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok {
			response.RespondWithError(w, http.StatusUnauthorized, "Не авторизован")
			return
		}

		var req struct {
			PromoID string `json:"promo_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Неверный JSON")
			return
		}

		if err := h.repo.ClaimByUser(req.PromoID, userID); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Нельзя получить промокод (дата в будущем или уже получен)")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// CreatePromoCodeHandler — админ создаёт
func CreatePromoCodeHandler(db *sql.DB) http.HandlerFunc {
	h := NewPromoHandlers(db)
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID          string `json:"id"`
			Date        string `json:"date"`
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Неверный JSON")
			return
		}

		if err := h.repo.CreatePromo(req.ID, req.Date, req.Title, req.Description); err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка создания промокода")
			return
		}

		response.RespondWithJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
	}
}

// AssignPromoCodeHandler — админ выдаёт
func AssignPromoCodeHandler(db *sql.DB) http.HandlerFunc {
	h := NewPromoHandlers(db)
	return func(w http.ResponseWriter, r *http.Request) {
		promoID := chi.URLParam(r, "promoID")
		if promoID == "" {
			response.RespondWithError(w, http.StatusBadRequest, "promoID обязателен")
			return
		}

		adminID, _ := middleware.GetUserIDFromContext(r.Context())

		var req struct {
			UserID int `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Неверный JSON")
			return
		}

		if err := h.repo.AssignPromo(promoID, req.UserID, adminID); err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка выдачи промокода")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
