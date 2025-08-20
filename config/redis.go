// config/redis.go
package config

import (
	"github.com/go-redis/redis/v8"
	"os"
	"strconv" // ✅ Добавлен импорт

)

func NewRedisClient() *redis.Client {
	addr := getEnv("REDIS_ADDR", "localhost:6379")
	password := getEnv("REDIS_PASSWORD", "")
	db := getEnvInt("REDIS_DB", 0)

	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
	}
	return fallback
}
