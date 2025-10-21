package repositories

import (
	"database/sql"
	"errors"
	"fmt"
)

type PromoRepository struct {
	db *sql.DB
}

func NewPromoRepository(db *sql.DB) *PromoRepository {
	return &PromoRepository{db: db}
}

// ClaimSinglePromoForUser выдаёт 1 промокод указанного бренда
func (r *PromoRepository) ClaimSinglePromoForUser(brand string, userID int) ([]string, error) {
	const query = `
		UPDATE promo_codes 
		SET assigned_to_user_id = $1, claimed_at = NOW()
		WHERE id = (
			SELECT id 
			FROM promo_codes
			WHERE brand = $2 
			  AND assigned_to_user_id IS NULL
			  AND valid_until >= CURRENT_DATE
			ORDER BY valid_until, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING promo_code;
	`

	var code string
	err := r.db.QueryRow(query, userID, brand).Scan(&code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("нет доступных промокодов для %s", brand)
		}
		return nil, fmt.Errorf("ошибка выдачи промокода: %w", err)
	}

	return []string{code}, nil
}

// ClaimYandexPairForUser выдаёт 2 промокода YANDEX из одной даты
func (r *PromoRepository) ClaimYandexPairForUser(userID int) ([]string, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("не удалось начать транзакцию: %w", err)
	}
	defer tx.Rollback()

	// Шаг 1: найти дату, у которой есть хотя бы 2 свободных промокода
	var validUntil string
	err = tx.QueryRow(`
		SELECT valid_until::text
		FROM promo_codes
		WHERE brand = 'YANDEX'
		  AND assigned_to_user_id IS NULL
		  AND valid_until >= CURRENT_DATE
		GROUP BY valid_until
		HAVING COUNT(*) >= 2
		ORDER BY valid_until
		LIMIT 1
		FOR UPDATE OF promo_codes SKIP LOCKED
	`).Scan(&validUntil)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("нет доступных пар промокодов YANDEX")
		}
		return nil, fmt.Errorf("ошибка поиска даты для YANDEX: %w", err)
	}

	// Шаг 2: заблокировать и выдать 2 промокода из этой даты
	rows, err := tx.Query(`
		UPDATE promo_codes 
		SET assigned_to_user_id = $1, claimed_at = NOW()
		WHERE id IN (
			SELECT id
			FROM promo_codes
			WHERE brand = 'YANDEX'
			  AND valid_until = $2::date
			  AND assigned_to_user_id IS NULL
			ORDER BY id
			LIMIT 2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING promo_code;
	`, userID, validUntil)

	if err != nil {
		return nil, fmt.Errorf("ошибка выдачи пары YANDEX: %w", err)
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("ошибка чтения промокода: %w", err)
		}
		codes = append(codes, code)
	}

	if len(codes) != 2 {
		return nil, fmt.Errorf("не удалось получить 2 промокода YANDEX (получено: %d)", len(codes))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return codes, nil
}