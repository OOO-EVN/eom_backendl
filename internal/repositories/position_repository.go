// repositories/position_repository.go

package repositories

import (
	"context"
	"database/sql"
	"time"

	"github.com/evn/eom_backendl/internal/models"
)

type PositionRepository struct {
	db *sql.DB
}

func NewPositionRepository(db *sql.DB) *PositionRepository {
	return &PositionRepository{db: db}
}

func (r *PositionRepository) Save(ctx context.Context, pos *models.GeoUpdate) error {
	query := `
		INSERT INTO positions (user_id, lat, lon, speed, accuracy, battery, event, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		pos.UserID,
		pos.Lat,
		pos.Lon,
		pos.Speed,
		pos.Accuracy,
		pos.Battery,
		pos.Event,
		time.Now(),
	).Scan(&pos.ID, &pos.CreatedAt)
	return err
}

func (r *PositionRepository) GetLastPositions(ctx context.Context) ([]models.LastLocation, error) {
	query := `
		SELECT DISTINCT ON (user_id) user_id, lat, lon, battery, created_at AS ts
		FROM positions
		WHERE created_at > NOW() - INTERVAL '5 minutes'
		ORDER BY user_id, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.LastLocation
	for rows.Next() {
		var loc models.LastLocation
		if err := rows.Scan(&loc.UserID, &loc.Lat, &loc.Lon, &loc.Battery, &loc.Ts); err != nil {
			return nil, err
		}
		result = append(result, loc)
	}
	return result, rows.Err()
}
func (r *PositionRepository) GetHistoryByUser(ctx context.Context, userID string, from, to time.Time) ([]models.GeoUpdate, error) {
	query := `
		SELECT user_id, lat, lon, speed, accuracy, battery, created_at
		FROM geo_updates
		WHERE user_id = $1 AND created_at BETWEEN $2 AND $3
		ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var updates []models.GeoUpdate
	for rows.Next() {
		var u models.GeoUpdate
		err := rows.Scan(&u.UserID, &u.Lat, &u.Lon, &u.Speed, &u.Accuracy, &u.Battery, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		updates = append(updates, u)
	}

	return updates, rows.Err()
}
	