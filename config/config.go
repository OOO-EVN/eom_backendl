// config/config.go
package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

// type contextKey string

// const (
// 	UserIDKey contextKey = "user_id"
// )

type Config struct {
	DatabaseDSN      string
	JwtSecret        string
	ServerPort       string
	TelegramBotToken string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
}

func NewConfig() *Config {
	// ✅ Загружаем .env перед всем остальным
	_ = godotenv.Load(".env")

	dsn := getEnv("DATABASE_DSN", "")
	jwtSecret := getEnv("JWT_SECRET", "0hn/a5hwoWLn4nrmogQo+zDCM7h9203J4Iwhkp7b2ns=")
	port := getEnv("SERVER_PORT", "6066")
	telegramBotToken := getEnv("TELEGRAM_BOT_TOKEN", "")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	redisDB := parseInt(getEnv("REDIS_DB", "0"))

	return &Config{
		DatabaseDSN:      dsn,
		JwtSecret:        jwtSecret,
		ServerPort:       port,
		TelegramBotToken: telegramBotToken,
		RedisAddr:        redisAddr,
		RedisPassword:    redisPassword,
		RedisDB:          redisDB,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// NewRedisClient создаёт подключение к Redis
func NewRedisClient() *redis.Client {
	cfg := NewConfig()
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
}

