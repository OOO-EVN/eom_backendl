// services/redis_store.go
package services

import (
    "context"
    "database/sql"
    "encoding/json"
    "strings"
    "time"

    "github.com/evn/eom_backendl/models"
    "github.com/redis/go-redis/v9"
)

type RedisStore struct {
    client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
    return &RedisStore{client: client}
}

func (r *RedisStore) Save(key string, value interface{}, expiration time.Duration) error {
    data, err := json.Marshal(value)
    if err != nil {
        return err
    }
    return r.client.Set(context.Background(), key, data, expiration).Err()
}

func (r *RedisStore) Get(key string, dest interface{}) error {
    data, err := r.client.Get(context.Background(), key).Result()
    if err != nil {
        return err
    }
    return json.Unmarshal([]byte(data), dest)
}

func (r *RedisStore) Delete(key string) error {
    return r.client.Del(context.Background(), key).Err()
}

func (r *RedisStore) GetOnlineUsers() ([]string, error) {
    ctx := context.Background()
    
    // Get all keys matching the online user pattern
    keys, err := r.client.Keys(ctx, "online:*").Result()
    if err != nil {
        return nil, err
    }
    
    // Extract usernames from keys
    var onlineUsers []string
    for _, key := range keys {
        if parts := strings.Split(key, ":"); len(parts) > 1 {
            onlineUsers = append(onlineUsers, parts[1])
        }
    }
    
    return onlineUsers, nil
}

func locationKey(userID int) string {
    return "location:user:" + string(rune(userID))
}

func (r *RedisStore) SaveLocation(loc *models.Location) error {
    data, err := json.Marshal(loc)
    if err != nil {
        return err
    }
    return r.client.Set(context.Background(), locationKey(loc.UserID), data, 5*time.Minute).Err()
}

func (r *RedisStore) DeleteLocation(userID int) error {
    return r.client.Del(context.Background(), locationKey(userID)).Err()
}

func (r *RedisStore) GetAllActiveShifts(db *sql.DB) ([]models.ActiveShift, error) {
    rows, err := db.Query(`
        SELECT s.id, s.user_id, u.username, s.start_time, s.slot_time_range, s.position, s.zone, s.selfie_path
        FROM slots s
        JOIN users u ON s.user_id = u.id
        WHERE s.end_time IS NULL
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var shifts []models.ActiveShift
    for rows.Next() {
        var shift models.ActiveShift
        if err := rows.Scan(
            &shift.ID,
            &shift.UserID,
            &shift.Username,
            &shift.StartTime,
            &shift.SlotTimeRange,
            &shift.Position,
            &shift.Zone,
            &shift.SelfiePath,
        ); err != nil {
            continue
        }

        var loc models.Location
        key := locationKey(shift.UserID)
        data, err := r.client.Get(context.Background(), key).Result()
        if err == nil {
            _ = json.Unmarshal([]byte(data), &loc)
            shift.LastLocation = &loc
        }

        shifts = append(shifts, shift)
    }

    return shifts, nil
}

func (r *RedisStore) GetAllLocations() ([]models.Location, error) {
    ctx := context.Background()
    iter := r.client.Scan(ctx, 0, "location:user:*", 0).Iterator()

    var locations []models.Location
    for iter.Next(ctx) {
        key := iter.Val()

        var loc models.Location
        data, err := r.client.Get(ctx, key).Result()
        if err != nil {
            continue
        }

        if err := json.Unmarshal([]byte(data), &loc); err != nil {
            continue
        }

        locations = append(locations, loc)
    }

    if err := iter.Err(); err != nil {
        return nil, err
    }

    return locations, nil
}
