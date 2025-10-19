package repositories

import (
	"database/sql"
	"time"
)

type PromoRepository struct {
	db *sql.DB
}

func NewPromoRepository(db *sql.DB) *PromoRepository {
	return &PromoRepository{db: db}
}

// GetDailyPromos — все промокоды (для отображения)
func (r *PromoRepository) GetDailyPromos() ([]map[string]interface{}, error) {
	rows, err := r.db.Query(`
		SELECT id, date, title, description 
		FROM daily_promos 
		ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var promos []map[string]interface{}
	for rows.Next() {
		var id, title, description string
		var date time.Time
		if err := rows.Scan(&id, &date, &title, &description); err != nil {
			return nil, err
		}
		promos = append(promos, map[string]interface{}{
			"id":          id,
			"date":        date.Format("2006-01-02"),
			"title":       title,
			"description": description,
		})
	}
	return promos, nil
}

// GetUserClaimedPromoIDs — какие промокоды уже получены
func (r *PromoRepository) GetUserClaimedPromoIDs(userID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT promo_id FROM promo_claims WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// CreatePromo — админ создаёт
func (r *PromoRepository) CreatePromo(id, dateStr, title, description string) error {
	_, err := r.db.Exec(`
		INSERT INTO daily_promos (id, date, title, description)
		VALUES ($1, $2::date, $3, $4)
		ON CONFLICT (id) DO NOTHING`,
		id, dateStr, title, description)
	return err
}

// AssignPromo — админ выдаёт
func (r *PromoRepository) AssignPromo(promoID string, userID, assignedBy int) error {
	_, err := r.db.Exec(`
		INSERT INTO promo_claims (promo_id, user_id, assigned_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (promo_id, user_id) DO NOTHING`,
		promoID, userID, assignedBy)
	return err
}

// ClaimByUser — пользователь сам активирует
func (r *PromoRepository) ClaimByUser(promoID string, userID int) error {
	// Проверим, что дата промокода <= сегодня
	var promoDate string
	err := r.db.QueryRow(`
		SELECT date FROM daily_promos 
		WHERE id = $1 AND date <= CURRENT_DATE`, promoID).Scan(&promoDate)
	if err != nil {
		return err // либо не существует, либо дата в будущем
	}

	_, err = r.db.Exec(`
		INSERT INTO promo_claims (promo_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (promo_id, user_id) DO NOTHING`,
		promoID, userID)
	return err
}