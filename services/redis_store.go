package services

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    "github.com/evn/eom_backendl/models"
    "github.com/go-redis/redis/v8"
)

type RedisStore struct {
    client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
    return &RedisStore{
        client: client,
    }
}

func (s *RedisStore) SaveLocation(loc *models.Location) error {
    ctx := context.Background()
    key := fmt.Sprintf("location:%d", loc.UserID)
    data, err := json.Marshal(loc)
    if err != nil {
        return err
    }
    return s.client.Set(ctx, key, data, 5*time.Minute).Err()
}

func (s *RedisStore) GetLocation(userID int) (*models.Location, error) {
    ctx := context.Background()
    key := fmt.Sprintf("location:%d", userID)
    data, err := s.client.Get(ctx, key).Bytes()
    if err != nil {
        return nil, err
    }
    var loc models.Location
    if err := json.Unmarshal(data, &loc); err != nil {
        return nil, err
    }
    return &loc, nil
}

func (s *RedisStore) GetAllLocations() ([]models.Location, error) {
    ctx := context.Background()
    pattern := "location:*"
    keys, err := s.client.Keys(ctx, pattern).Result()
    if err != nil {
        return nil, err
    }
    var locations []models.Location
    for _, key := range keys {
        data, err := s.client.Get(ctx, key).Bytes()
        if err != nil {
            continue
        }
        var loc models.Location
        if err := json.Unmarshal(data, &loc); err != nil {
            continue
        }
        locations = append(locations, loc)
    }
    return locations, nil
}

func (s *RedisStore) DeleteLocation(userID int) error {
    ctx := context.Background()
    key := fmt.Sprintf("location:%d", userID)
    return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) GetAllActiveShifts(db *sql.DB) ([]models.UserShiftLocation, error) {
    ctx := context.Background()
    rows, err := db.Query(`
        SELECT s.user_id, u.username, s.position, s.zone, s.start_time
        FROM slots s
        JOIN users u ON s.user_id = u.id
        WHERE s.end_time IS NULL
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var activeShifts []models.UserShiftLocation
    for rows.Next() {
        var userID int
        var username, position, zone string
        var startTime time.Time
        if err := rows.Scan(&userID, &username, &position, &zone, &startTime); err != nil {
            continue
        }
        shift := models.UserShiftLocation{
            UserID:    userID,
            Username:  username,
            Position:  position,
            Zone:      zone,
            StartTime: startTime,
        }
        locationKey := fmt.Sprintf("location:%d", userID)
        locationData, err := s.client.Get(ctx, locationKey).Bytes()
        if err == nil {
            var loc models.Location
            if err := json.Unmarshal(locationData, &loc); err == nil {
                shift.Lat = &loc.Lat
                shift.Lng = &loc.Lng
                shift.Timestamp = &loc.Timestamp
                shift.HasLocation = true
            }
        }
        activeShifts = append(activeShifts, shift)
    }
    return activeShifts, nil
}
