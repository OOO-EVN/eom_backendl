// services/geotrack_service.go

package services

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/evn/eom_backendl/models"
	"github.com/evn/eom_backendl/repositories"
	"github.com/redis/go-redis/v9"
)

type GeoTrackService struct {
	posRepo *repositories.PositionRepository
	redis   *redis.Client
}

func NewGeoTrackService(
	posRepo *repositories.PositionRepository,
	redis *redis.Client,
) *GeoTrackService {
	return &GeoTrackService{
		posRepo: posRepo,
		redis:   redis,
	}
}

func (s *GeoTrackService) HandleUpdate(ctx context.Context, update *models.GeoUpdate) error {
	// 1. Сохранить в PostgreSQL
	if err := s.posRepo.Save(ctx, update); err != nil {
		log.Printf("❌ FAILED TO SAVE TO POSTGRESQL: %v", err)
		return err
	}

	// 2. Обновить Redis
	key := "last:" + update.UserID
	data, _ := json.Marshal(map[string]interface{}{
		"lat":     update.Lat,
		"lon":     update.Lon,
		"battery": update.Battery,
		"ts":      update.CreatedAt.Format(time.RFC3339),
	})
	if err := s.redis.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		log.Printf("❌ FAILED TO UPDATE REDIS: %v", err)
		return err
	}

	// 3. Обновить список активных пользователей
	if err := s.redis.SAdd(ctx, "active_users", update.UserID).Err(); err != nil {
		log.Printf("⚠️ Redis SAdd warning: %v", err)
	}
	if err := s.redis.Expire(ctx, "active_users", 5*time.Minute).Err(); err != nil {
		log.Printf("⚠️ Redis Expire warning: %v", err)
	}

	return nil
}

func (s *GeoTrackService) GetLastLocations(ctx context.Context) ([]models.LastLocation, error) {
	locations, err := s.posRepo.GetLastPositions(ctx)
	if err != nil {
		log.Printf("❌ FAILED TO FETCH LAST POSITIONS: %v", err)
		return nil, err
	}
	return locations, nil
}
