// services/redis_store.go
package services

import (
	"context"
	"encoding/json"
	"time"
	"github.com/go-redis/redis/v8"
	"github.com/evn/eom_backendl/models"
)

type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{
		client: client,
		ctx:    context.Background(),
	}
}

func (r *RedisStore) SaveLocation(loc *models.Location) error {
	data, _ := json.Marshal(loc)
	key := "user:location:" + string(loc.UserID)
	return r.client.Set(r.ctx, key, data, 24*time.Hour).Err()
}

func (r *RedisStore) GetLocation(userID int) (*models.Location, error) {
	key := "user:location:" + string(userID)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var loc models.Location
	json.Unmarshal(data, &loc)
	return &loc, nil
}

func (r *RedisStore) GetAllLocations() ([]*models.Location, error) {
	keys, err := r.client.Keys(r.ctx, "user:location:*").Result()
	if err != nil {
		return nil, err
	}

	var locations []*models.Location
	for _, key := range keys {
		data, err := r.client.Get(r.ctx, key).Bytes()
		if err != nil {
			continue
		}
		var loc models.Location
		json.Unmarshal(data, &loc)
		locations = append(locations, &loc)
	}
	return locations, nil
}
