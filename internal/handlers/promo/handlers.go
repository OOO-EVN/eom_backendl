package promo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/evn/eom_backendl/internal/middleware"
	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/evn/eom_backendl/internal/repositories"
	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type PromoHandlers struct {
	repo *repositories.PromoRepository
}

func NewPromoHandlers(db *sql.DB) *PromoHandlers {
	return &PromoHandlers{
		repo: repositories.NewPromoRepository(db),
	}
}

func isAdmin(userID int, db *sql.DB) bool {
	var role string
	err := db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if err != nil {
		return false
	}
	return role == "superadmin" || role == "supervisor" || role == "coordinator"
}

func ClaimPromoByBrandHandler(db *sql.DB) http.HandlerFunc {
	h := NewPromoHandlers(db)
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok {
			response.RespondWithError(w, http.StatusUnauthorized, "Не авторизован")
			return
		}

		brand := strings.ToUpper(chi.URLParam(r, "brand"))
		if brand != "JET" && brand != "YANDEX" && brand != "WHOOSH" && brand != "BOLT" {
			response.RespondWithError(w, http.StatusBadRequest, "Недопустимый бренд")
			return
		}

		var activeBrand string
		err := db.QueryRow(`
			SELECT brand 
			FROM active_promo_brand 
			WHERE expires_at > NOW()
		`).Scan(&activeBrand)
		if err == nil && brand != activeBrand {
			response.RespondWithError(w, http.StatusForbidden, "Промокоды этого бренда временно недоступны")
			return
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка проверки активного бренда")
			return
		}

		var existingPromos map[string][]string
		err = db.QueryRow("SELECT promo_codes FROM users WHERE id = $1", userID).Scan(&existingPromos)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка проверки промокодов")
			return
		}

		if codes, exists := existingPromos[brand]; exists && len(codes) > 0 {
			response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
				"promo_codes":     codes,
				"already_claimed": true,
			})
			return
		}

		var codes []string
		if brand == "YANDEX" {
			codes, err = h.repo.ClaimYandexPairForUser(userID)
		} else {
			codes, err = h.repo.ClaimSinglePromoForUser(brand, userID)
		}

		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		if existingPromos == nil {
			existingPromos = make(map[string][]string)
		}
		existingPromos[brand] = codes

		_, err = db.Exec("UPDATE users SET promo_codes = $1 WHERE id = $2", existingPromos, userID)
		if err != nil {
			log.Printf("Ошибка сохранения промокодов: %v", err)
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"promo_codes":     codes,
			"already_claimed": false,
		})
	}
}

type UploadPromoRequest struct {
	GoogleSheetURL string `json:"google_sheet_url,omitempty"`
}

func UploadPromoCodesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok || !isAdmin(userID, db) {
			response.RespondWithError(w, http.StatusForbidden, "Требуются права администратора")
			return
		}

		var rows [][]string
		var err error

		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			var req UploadPromoRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				response.RespondWithError(w, http.StatusBadRequest, "Неверный JSON")
				return
			}
			if req.GoogleSheetURL == "" {
				response.RespondWithError(w, http.StatusBadRequest, "google_sheet_url обязателен")
				return
			}
			rows, err = readFromGoogleSheet(req.GoogleSheetURL)
			if err != nil {
				response.RespondWithError(w, http.StatusInternalServerError, "Ошибка чтения Google Sheets: "+err.Error())
				return
			}
		} else {
			file, _, err := r.FormFile("file")
			if err != nil {
				response.RespondWithError(w, http.StatusBadRequest, "Файл не найден")
				return
			}
			defer file.Close()

			xlsx, err := excelize.OpenReader(file)
			if err != nil {
				response.RespondWithError(w, http.StatusBadRequest, "Неверный формат Excel")
				return
			}
			rows, err = xlsx.GetRows("Sheet1")
			if err != nil {
				sheets := xlsx.GetSheetList()
				if len(sheets) == 0 {
					response.RespondWithError(w, http.StatusBadRequest, "Пустой Excel")
					return
				}
				rows, err = xlsx.GetRows(sheets[0])
				if err != nil {
					response.RespondWithError(w, http.StatusInternalServerError, "Ошибка чтения листа")
					return
				}
			}
		}

		if len(rows) < 2 {
			response.RespondWithError(w, http.StatusBadRequest, "Файл должен содержать заголовок и хотя бы одну строку")
			return
		}

		if err := validateAndSavePromos(db, rows, userID); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func GetPromoStatsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok || !isAdmin(userID, db) {
			response.RespondWithError(w, http.StatusForbidden, "Требуются права администратора")
			return
		}

		var activeBrand *string
		err := db.QueryRow(`
			SELECT brand 
			FROM active_promo_brand 
			WHERE expires_at > NOW()
		`).Scan(&activeBrand)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка проверки активного бренда")
			return
		}

		whereClause := "assigned_to_user_id IS NULL AND valid_until >= CURRENT_DATE"
		var args []interface{}
		if activeBrand != nil {
			whereClause += " AND brand = $1"
			args = append(args, *activeBrand)
		}

		summary := make(map[string]int)
		query := "SELECT brand, COUNT(*) FROM promo_codes WHERE " + whereClause + " GROUP BY brand"
		rows, err := db.Query(query, args...)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка статистики")
			return
		}
		for rows.Next() {
			var brand string
			var cnt int
			rows.Scan(&brand, &cnt)
			summary[brand] = cnt
		}
		rows.Close()

		type DateStat struct {
			ValidUntil string         `json:"valid_until"`
			Counts     map[string]int `json:"counts"`
		}
		dateMap := make(map[string]map[string]int)
		dateQuery := "SELECT valid_until::text, brand, COUNT(*) FROM promo_codes WHERE " + whereClause + " GROUP BY valid_until, brand ORDER BY valid_until"
		dateRows, err := db.Query(dateQuery, args...)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка статистики по датам")
			return
		}
		for dateRows.Next() {
			var dateStr, brand string
			var cnt int
			dateRows.Scan(&dateStr, &brand, &cnt)
			if _, exists := dateMap[dateStr]; !exists {
				dateMap[dateStr] = make(map[string]int)
			}
			dateMap[dateStr][brand] = cnt
		}
		dateRows.Close()

		var byDate []DateStat
		for date, counts := range dateMap {
			byDate = append(byDate, DateStat{ValidUntil: date, Counts: counts})
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"summary": summary,
			"by_date": byDate,
		})
	}
}

type SetActiveBrandRequest struct {
	Brand string `json:"brand"`
	Days  int    `json:"days"`
}

func SetActivePromoBrandHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok || !isAdmin(userID, db) {
			response.RespondWithError(w, http.StatusForbidden, "Требуются права администратора")
			return
		}

		var req SetActiveBrandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Неверный JSON")
			return
		}

		brand := strings.ToUpper(req.Brand)
		if brand != "JET" && brand != "YANDEX" && brand != "WHOOSH" && brand != "BOLT" {
			response.RespondWithError(w, http.StatusBadRequest, "Недопустимый бренд")
			return
		}

		days := req.Days
		if days <= 0 {
			days = 10
		}

		_, err := db.Exec(`
			INSERT INTO active_promo_brand (brand, expires_at)
			VALUES ($1, NOW() + $2 * INTERVAL '1 day')
			ON CONFLICT ((brand IS NOT NULL)) DO UPDATE
			SET brand = EXCLUDED.brand, expires_at = EXCLUDED.expires_at;
		`, brand, days)
		if err != nil {
			log.Printf("Ошибка установки активного бренда: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка сервера")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
		})
	}
}

func ClearActivePromoBrandHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok || !isAdmin(userID, db) {
			response.RespondWithError(w, http.StatusForbidden, "Требуются права администратора")
			return
		}

		_, err := db.Exec("DELETE FROM active_promo_brand")
		if err != nil {
			log.Printf("Ошибка очистки активного бренда: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка сервера")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
	}
}

func GetActivePromoBrandHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromContext(r.Context())
		if !ok || !isAdmin(userID, db) {
			response.RespondWithError(w, http.StatusForbidden, "Требуются права администратора")
			return
		}

		var brand string
		var expiresAt time.Time
		err := db.QueryRow(`
			SELECT brand, expires_at 
			FROM active_promo_brand 
			WHERE expires_at > NOW()
		`).Scan(&brand, &expiresAt)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				response.RespondWithJSON(w, http.StatusOK, nil)
				return
			}
			response.RespondWithError(w, http.StatusInternalServerError, "Ошибка")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"brand":      brand,
			"expires_at": expiresAt.Format("2006-01-02"),
		})
	}
}

func validateAndSavePromos(db *sql.DB, rows [][]string, adminID int) error {
	if len(rows) < 1 {
		return fmt.Errorf("нет данных")
	}

	dataRows := rows[1:]
	groups := make(map[string][]string)

	for _, row := range dataRows {
		if len(row) < 3 {
			continue
		}
		brand := strings.ToUpper(strings.TrimSpace(row[0]))
		code := strings.TrimSpace(row[1])
		validStr := strings.TrimSpace(row[2])

		if brand == "" || code == "" || validStr == "" {
			continue
		}

		if brand != "JET" && brand != "YANDEX" && brand != "WHOOSH" && brand != "BOLT" {
			return fmt.Errorf("неизвестный бренд: %s", brand)
		}

		_, err := time.Parse("2006-01-02", validStr)
		if err != nil {
			return fmt.Errorf("неверный формат даты (ожидается ГГГГ-ММ-ДД): %s", validStr)
		}

		key := brand + "|" + validStr
		groups[key] = append(groups[key], code)
	}

	for key, codes := range groups {
		parts := strings.Split(key, "|")
		brand := parts[0]
		if brand == "YANDEX" && len(codes)%2 != 0 {
			return fmt.Errorf("у YANDEX должно быть чётное количество промокодов на дату %s", parts[1])
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, codes := range groups {
		parts := strings.Split(key, "|")
		brand := parts[0]
		validUntil := parts[1]

		for _, code := range codes {
			_, err := tx.Exec(`
				INSERT INTO promo_codes (brand, promo_code, valid_until, created_by_admin_id)
				VALUES ($1, $2, $3, $4)
			`, brand, code, validUntil, adminID)
			if err != nil {
				return fmt.Errorf("ошибка вставки: %w", err)
			}
		}
	}

	return tx.Commit()
}

func readFromGoogleSheet(url string) ([][]string, error) {
	re := regexp.MustCompile(`\/d\/([a-zA-Z0-9-_]+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return nil, fmt.Errorf("неверный URL Google Sheets")
	}
	spreadsheetID := matches[1]

	ctx := context.Background()
	srv, err := sheets.NewService(ctx, option.WithCredentialsFile("credentials.json"))
	if err != nil {
		return nil, fmt.Errorf("ошибка инициализации Google API: %w", err)
	}

	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, "A1:C1000").Do()
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения таблицы: %w", err)
	}

	if len(resp.Values) == 0 {
		return nil, fmt.Errorf("таблица пуста")
	}

	var rows [][]string
	for _, row := range resp.Values {
		var strRow []string
		for _, cell := range row {
			strRow = append(strRow, fmt.Sprintf("%v", cell))
		}
		rows = append(rows, strRow)
	}

	return rows, nil
}
