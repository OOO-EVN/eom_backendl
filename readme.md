package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/db"
	"github.com/evn/eom_backendl/handlers"
	"github.com/evn/eom_backendl/repositories"
	"github.com/evn/eom_backendl/services"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
)

func main() {
	cfg := config.NewConfig()
	database := db.InitDB(cfg.DatabaseDSN)
	defer database.Close()

	redisClient := config.NewRedisClient()
	defer redisClient.Close()

	jwtAuth := jwtauth.New("HS256", []byte(cfg.JwtSecret), nil)
	jwtService := services.NewJWTService(cfg.JwtSecret, redisClient)
	telegramAuthService := services.NewTelegramAuthService(cfg.TelegramBotToken)

	posRepo := repositories.NewPositionRepository(database)
	geoService := services.NewGeoTrackService(posRepo, redisClient)
	geoHandler := handlers.NewGeoTrackHandler(geoService)

	authHandler := handlers.NewAuthHandler(database, jwtService, telegramAuthService)
	profileHandler := handlers.NewProfileHandler(database)
	mapHandler := handlers.NewMapHandler(database)
	scooterStatsHandler := handlers.NewScooterStatsHandler("/root/tg_bot/Sharing/scooters.db")
	appVersionHandler := handlers.NewAppVersionHandler(database)

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(jwtauth.Verifier(jwtAuth))
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil || token == nil {
				next.ServeHTTP(w, r)
				return
			}
			claims := token.PrivateClaims()
			var userID int
			if rawID, ok := claims["user_id"]; ok {
				switch v := rawID.(type) {
				case float64:
					userID = int(v)
				case int:
					userID = v
				case string:
					if id, err := strconv.Atoi(v); err == nil {
						userID = id
					}
				}
			}
			if userID != 0 {
				ctx := context.WithValue(r.Context(), config.UserIDKey, userID)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	})

	router.Post("/api/geo", geoHandler.PostGeo)

	router.Post("/api/auth/register", authHandler.RegisterHandler)
	router.Post("/api/auth/login", authHandler.LoginHandler)
	router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)
	router.Get("/auth_callback", authHandler.TelegramAuthCallbackHandler)

	router.Get("/api/users", handlers.ListUsersHandler(database))
	router.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir("./uploads"))))
	router.Get("/api/active-slots", handlers.GetActiveShiftsHandler(database))
	router.Post("/api/auth/refresh", authHandler.RefreshTokenHandler)
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Group(func(r chi.Router) {
		r.Use(jwtauth.Authenticator(jwtAuth))

		r.Get("/api/profile", profileHandler.GetProfile)
		r.Post("/api/logout", authHandler.LogoutHandler)
		r.Post("/api/auth/complete-registration", authHandler.CompleteRegistrationHandler)
		r.Get("/api/admin/active-shifts", GetActiveShiftsForAll(database))
		r.Get("/api/admin/ended-shifts", handlers.GetEndedShiftsHandler(database))
		r.Post("/api/slot/start", handlers.StartSlotHandler(database))
		r.Post("/api/slot/end", handlers.EndSlotHandler(database))
		r.Get("/api/shifts/active", handlers.GetUserActiveShiftHandler(database))
		r.Get("/api/shifts", handlers.GetShiftsHandler(database))
		r.Get("/api/shifts/date/{date}", handlers.GetShiftsByDateHandler(database))
		r.Get("/api/users/{userID}/shifts", handlers.GetUserShiftsByIDHandler(database))

		r.Get("/api/slots/positions", handlers.GetAvailablePositionsHandler(database))
		r.Get("/api/slots/times", handlers.GetAvailableTimeSlotsHandler(database))
		r.Get("/api/slots/zones", handlers.GetAvailableZonesHandler(database))
		r.Post("/api/admin/generate-shifts", handlers.GenerateShiftsHandler(database))

		r.Get("/api/scooter-stats/shift", scooterStatsHandler.GetShiftStatsHandler)

		r.Get("/api/admin/maps", mapHandler.GetMapsHandler)
		r.Get("/api/admin/maps/{mapID}", mapHandler.GetMapByIDHandler)
		r.Get("/api/admin/maps/files/{filename}", mapHandler.ServeMapFileHandler)

		r.Post("/api/app/version/check", appVersionHandler.CheckVersionHandler)
		r.Get("/api/app/version/latest", appVersionHandler.GetLatestVersionHandler)

		r.Group(func(r chi.Router) {
			r.Use(superadminOnlyMiddleware(jwtService))

			r.Get("/api/admin/users", handlers.ListAdminUsersHandler(database))
			r.Patch("/api/admin/users/{userID}/role", handlers.UpdateUserRoleHandler(database))
			r.Post("/api/admin/roles", handlers.CreateRoleHandler(database))
			r.Delete("/api/admin/roles", handlers.DeleteRoleHandler(database))
			r.Post("/api/admin/users", handlers.CreateUserHandler(database))
			r.Patch("/api/admin/users/{userID}/status", handlers.UpdateUserStatusHandler(database))
			r.Delete("/api/admin/users/{userID}", handlers.DeleteUserHandler(database))
			r.Post("/api/admin/users/{userID}/end-shift", handlers.ForceEndShiftHandler(database))

			r.Post("/api/admin/maps/upload", mapHandler.UploadMapHandler)
			r.Delete("/api/admin/maps/{mapID}", mapHandler.DeleteMapHandler)

			r.Get("/api/admin/zones", handlers.GetAvailableZonesHandler(database))
			r.Post("/api/admin/zones", handlers.CreateZoneHandler(database))
			r.Put("/api/admin/zones/{id}", handlers.UpdateZoneHandler(database))
			r.Delete("/api/admin/zones/{id}", handlers.DeleteZoneHandler(database))

			r.Get("/api/admin/app/versions", appVersionHandler.ListVersionsHandler)
			r.Post("/api/admin/app/versions", appVersionHandler.CreateVersionHandler)
			r.Put("/api/admin/app/versions/{id}", appVersionHandler.UpdateVersionHandler)
			r.Delete("/api/admin/app/versions/{id}", appVersionHandler.DeleteVersionHandler)

			r.Get("/api/admin/auto-end-shifts", handlers.AutoEndShiftsHandler(database))

			r.Get("/last", geoHandler.GetLast)
		})
	})

	if err := ensureUploadDirs(); err != nil {
		log.Fatalf("Failed to create upload directories: %v", err)
	}

	go func() {
		log.Println("✅ Auto-end shifts job started")
		if count, err := handlers.AutoEndShifts(database); err != nil {
			log.Printf("❌ Startup failed: %v", err)
		} else {
			log.Printf("✅ Startup: ended %d slots", count)
		}

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if count, err := handlers.AutoEndShifts(database); err != nil {
				log.Printf("❌ AutoEndShifts failed: %v", err)
			} else if count > 0 {
				log.Printf("✅ AutoEndShifts: ended %d expired slots", count)
			}
		}
	}()

	serverAddress := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("🚀 Server starting on %s", serverAddress)
	log.Fatal(http.ListenAndServe(serverAddress, router))
}

func superadminOnlyMiddleware(jwtService *services.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil {
				handlers.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}
			claims, err := token.AsMap(r.Context())
			if err != nil {
				handlers.RespondWithError(w, http.StatusUnauthorized, "Invalid claims")
				return
			}
			if claims["role"] != "superadmin" {
				handlers.RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ensureUploadDirs() error {
	dirs := []string{
		"./uploads/selfies",
		"./uploads/maps",
		"./uploads/app",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func GetActiveShiftsForAll(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT s.id, s.user_id, u.username, s.start_time, s.slot_time_range, s.position, s.zone, s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.end_time IS NULL
		`)
		if err != nil {
			log.Printf("DB query error: %v", err)
			handlers.RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var shifts []map[string]interface{}
		for rows.Next() {
			var id, userID int
			var username, startTime, slotTimeRange, position, zone, selfie string
			if err := rows.Scan(&id, &userID, &username, &startTime, &slotTimeRange, &position, &zone, &selfie); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			shifts = append(shifts, map[string]interface{}{
				"id":              id,
				"user_id":         userID,
				"username":        username,
				"start_time":      startTime,
				"slot_time_range": slotTimeRange,
				"position":        position,
				"zone":            zone,
				"selfie":          selfie,
			})
		}
		handlers.RespondWithJSON(w, http.StatusOK, shifts)
	}
}
// config/config.go
package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
)

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

	dsn := getEnv("DATABASE_DSN", "./data.db")
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

// db/database.go
package db

import (
    "database/sql"
    _ "github.com/lib/pq" // ← важно: нижнее подчёркивание!
    "log"
    "os"
)

// InitDB инициализирует соединение с базой данных и создаёт таблицы
func InitDB(dsn string) *sql.DB {
    log.Println("Попытка подключения к PostgreSQL по DSN:", dsn)
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatalf("Ошибка при открытии подключения к PostgreSQL: %v", err)
    }

    if err = db.Ping(); err != nil {
        log.Fatalf("Ошибка при пинге PostgreSQL: %v", err)
    }
    log.Println("Успешное подключение к PostgreSQL.")

    createTable(db)
    log.Println("База данных успешно инициализирована.")
    return db
}

// createTable читает schema.sql и применяет его
func createTable(db *sql.DB) {
    log.Println("Чтение файла схемы db/schema.sql...")
    schema, err := os.ReadFile("db/schema.sql")
    if err != nil {
        log.Fatalf("Не удалось прочитать файл схемы БД: %v", err)
    }

    log.Println("Попытка создания таблиц...")
    _, err = db.Exec(string(schema))
    if err != nil {
        log.Fatalf("Не удалось создать таблицы: %v", err)
    }
    log.Println("Таблицы успешно созданы.")
}package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type CreateUserRequest struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func CreateUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input CreateUserRequest

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if input.Username == "" {
			RespondWithError(w, http.StatusBadRequest, "Username is required")
			return
		}

		// Проверяем, существует ли пользователь с таким именем
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", input.Username).Scan(&count)
		if err != nil {
			log.Printf("DB error checking for existing user: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "DB error")
			return
		}
		if count > 0 {
			RespondWithError(w, http.StatusConflict, "Username already exists")
			return
		}

		_, err = db.Exec(
			"INSERT INTO users (username, first_name, role) VALUES ($1, $2, $3)",
			input.Username,
			input.FirstName,
			"scout",
		)
		if err != nil {
			log.Printf("DB error creating user: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "DB error creating user")
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "User created successfully"})
	}
}
func UpdateUserRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		var update struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		var roleExists int
		err = db.QueryRow("SELECT COUNT(*) FROM roles WHERE name = $1", update.Role).Scan(&roleExists)
		if err != nil || roleExists == 0 {
			RespondWithError(w, http.StatusBadRequest, "Role does not exist")
			return
		}

		_, err = db.Exec("UPDATE users SET role = $1 WHERE id = $2", update.Role, userID)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to update user role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User role updated successfully"})
	}
}

func UpdateUserStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем userID из URL
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		// Декодируем тело запроса
		var req struct {
			Status string `json:"status"` // Ожидаем "active" или "pending"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Проверяем, что статус допустимый
		if req.Status != "active" && req.Status != "pending" {
			RespondWithError(w, http.StatusBadRequest, "Invalid status value. Must be 'active' or 'pending'")
			return
		}

		// Подготавливаем значения для БД
		isActive := 0
		if req.Status == "active" {
			isActive = 1
		}

		// Обновляем запись в БД
		_, err = db.Exec("UPDATE users SET status = $1, is_active = $2 WHERE id = $3", req.Status, isActive, userID)
		if err != nil {
			log.Printf("Failed to update user %d status: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to update user status")
			return
		}

		// Отправляем успешный ответ
		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User status updated successfully"})
	}
}
func DeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		result, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
		if err != nil {
			log.Printf("Failed to delete user: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete user")
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Failed to get rows affected: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to check deletion status")
			return
		}

		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "User not found")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
	}
}

func CreateRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var newRole struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&newRole); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		_, err := db.Exec("INSERT INTO roles (name) VALUES ($1)", newRole.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to create new role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "Role created successfully"})
	}
}

func DeleteRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var roleToDelete struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&roleToDelete); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if roleToDelete.Name == "user" || roleToDelete.Name == "superadmin" {
			RespondWithError(w, http.StatusBadRequest, "Cannot delete this role")
			return
		}

		_, err := db.Exec("DELETE FROM roles WHERE name = $1", roleToDelete.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Role deleted successfully"})
	}
}
// handlers/admin_users.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// ListAdminUsersHandler возвращает список всех пользователей для админов
func ListAdminUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, username, first_name, role, status, is_active, created_at 
			FROM users 
			ORDER BY created_at DESC
		`)
		if err != nil {
			log.Printf("Database query error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to fetch users")
			return
		}
		defer rows.Close()

		var users []map[string]interface{}

		for rows.Next() {
			var user struct {
				ID        int            `json:"id"`
				Username  string         `json:"username"`
				FirstName sql.NullString `json:"first_name"`
				Role      string         `json:"role"`
				Status    string         `json:"status"`
				IsActive  bool           `json:"is_active"`
				CreatedAt time.Time      `json:"created_at"`
			}

			err := rows.Scan(
				&user.ID,
				&user.Username,
				&user.FirstName,
				&user.Role,
				&user.Status,
				&user.IsActive,
				&user.CreatedAt,
			)
			if err != nil {
				log.Printf("Error scanning user row: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to read user data")
				return
			}

			firstName := ""
			if user.FirstName.Valid {
				firstName = user.FirstName.String
			}

			users = append(users, map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"first_name": firstName,
				"role":       user.Role,
				"status":     user.Status,
				"is_active":  user.IsActive,
				"created_at": user.CreatedAt.Format(time.RFC3339), // или "2006-01-02 15:04:05"
			})
		}

		// Проверяем ошибки после итерации по rows
		if err = rows.Err(); err != nil {
			log.Printf("Row iteration error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Data read error")
			return
		}

		RespondWithJSON(w, http.StatusOK, users)
	}
}

// Вспомогательная функция для парсинга JSON (может использоваться в других обработчиках)
func parseJSONBody(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
package handlers

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    
    "github.com/go-chi/chi/v5"
    "github.com/evn/eom_backendl/models"
    "github.com/evn/eom_backendl/repositories"
    "github.com/evn/eom_backendl/config"
)

type AppVersionHandler struct {
    repo *repositories.AppVersionRepository
    db   *sql.DB  // Добавляем DB для доступа к пользователям
}

func NewAppVersionHandler(db *sql.DB) *AppVersionHandler {
    return &AppVersionHandler{
        repo: repositories.NewAppVersionRepository(db),
        db:   db,
    }
}

// CheckVersionHandler проверяет наличие обновлений
func (h *AppVersionHandler) CheckVersionHandler(w http.ResponseWriter, r *http.Request) {
    userID, ok := r.Context().Value(config.UserIDKey).(int)
    if !ok {
        RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
        return
    }
    
    var req models.VersionCheckRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    // Определяем платформу по User-Agent или из запроса
    if req.Platform == "" {
        req.Platform = h.detectPlatform(r)
    }
    
    response, err := h.repo.CheckVersion(req.Platform, req.CurrentVersion, req.BuildNumber)
    if err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to check version: "+err.Error())
        return
    }
    
    // Логируем проверку обновлений
    fmt.Printf("User %d checked for updates. Platform: %s, Current: %s, Build: %d, HasUpdate: %t\n", 
        userID, req.Platform, req.CurrentVersion, req.BuildNumber, response.HasUpdate)
    
    RespondWithJSON(w, http.StatusOK, response)
}

// GetLatestVersionHandler возвращает последнюю версию для платформы
func (h *AppVersionHandler) GetLatestVersionHandler(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    if platform == "" {
        platform = h.detectPlatform(r)
    }
    
    version, err := h.repo.GetLatestVersion(platform)
    if err != nil {
        RespondWithError(w, http.StatusNotFound, "No version found: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, version)
}

// ListVersionsHandler возвращает список всех версий (для админов)
func (h *AppVersionHandler) ListVersionsHandler(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    
    var versions []models.AppVersion
    var err error
    
    if platform != "" {
        versions, err = h.repo.GetAllVersions(platform)
    } else {
        // Получаем все версии для всех платформ
        androidVersions, _ := h.repo.GetAllVersions("android")
        iosVersions, _ := h.repo.GetAllVersions("ios")
        versions = append(androidVersions, iosVersions...)
    }
    
    if err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to list versions: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, versions)
}

// CreateVersionHandler создает новую версию (только для superadmin)
func (h *AppVersionHandler) CreateVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    var version models.AppVersion
    if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    if err := h.repo.CreateVersion(&version); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to create version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusCreated, version)
}

// UpdateVersionHandler обновляет существующую версию (только для superadmin)
func (h *AppVersionHandler) UpdateVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    idStr := chi.URLParam(r, "id")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid version ID")
        return
    }
    
    var version models.AppVersion
    if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    version.ID = id
    if err := h.repo.UpdateVersion(&version); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to update version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, version)
}

// DeleteVersionHandler удаляет версию (только для superadmin)
func (h *AppVersionHandler) DeleteVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    idStr := chi.URLParam(r, "id")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid version ID")
        return
    }
    
    if err := h.repo.DeleteVersion(id); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to delete version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Version deleted successfully"})
}

// Вспомогательные методы

func (h *AppVersionHandler) detectPlatform(r *http.Request) string {
    userAgent := r.Header.Get("User-Agent")
    switch {
    case strings.Contains(userAgent, "Android"):
        return "android"
    case strings.Contains(userAgent, "iPhone"), strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "iOS"):
        return "ios"
    default:
        return "unknown"
    }
}

func (h *AppVersionHandler) isSuperAdmin(r *http.Request) bool {
    userID, ok := r.Context().Value(config.UserIDKey).(int)
    if !ok {
        return false
    }
    
    role := h.getUserRole(userID)
    return role == "superadmin"
}

// Вспомогательная функция для получения роли пользователя
func (h *AppVersionHandler) getUserRole(userID int) string {
    var role string
    err := h.db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
    if err != nil {
        return "user"
    }
    return role
}
// handlers/auth.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/services"
)

type AuthHandler struct {
	db                  *sql.DB
	jwtService          *services.JWTService
	telegramAuthService *services.TelegramAuthService
}

func NewAuthHandler(db *sql.DB, jwtService *services.JWTService, tgService *services.TelegramAuthService) *AuthHandler {
	return &AuthHandler{
		db:                  db,
		jwtService:          jwtService,
		telegramAuthService: tgService,
	}
}

func (h *AuthHandler) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	type RequestBody struct {
		RefreshToken string `json:"refresh_token"`
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if body.RefreshToken == "" {
		RespondWithError(w, http.StatusUnauthorized, "Refresh token required")
		return
	}

	userID, err := h.jwtService.ValidateRefreshToken(body.RefreshToken)
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	var username, role string
	err = h.db.QueryRow("SELECT username, role FROM users WHERE id = $1", userID).Scan(&username, &role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "User not found")
		return
	}

	accessToken, refreshToken, err := h.jwtService.GenerateToken(userID, username, role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Could not generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{
		"token":         accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var regData struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		Password  string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&regData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", regData.Username).Scan(&count)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if count > 0 {
		RespondWithError(w, http.StatusBadRequest, "Username already exists")
		return
	}

	passwordHash, err := services.HashPassword(regData.Password)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	_, err = h.db.Exec(`
		INSERT INTO users (username, first_name, password_hash, role, status, is_active)
		VALUES ($1, $2, $3, 'user', 'active', TRUE)`,
		regData.Username,
		regData.FirstName,
		passwordHash,
	)

	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	RespondWithJSON(w, http.StatusCreated, map[string]string{
		"message": "User registered successfully",
	})
}

func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var loginData struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	var user struct {
		ID           int
		Username     string
		PasswordHash string
		Role         string
		Status       string
	}

	row := h.db.QueryRow(`
		SELECT id, username, password_hash, role, status
		FROM users
		WHERE LOWER(username) = LOWER($1)`,
		loginData.Username,
	)

	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		} else {
			RespondWithError(w, http.StatusInternalServerError, "Database error: "+err.Error())
		}
		return
	}

	if !services.CheckPasswordHash(loginData.Password, user.PasswordHash) {
		RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if user.Status == "pending" && user.Role != "superadmin" {
		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"status":   user.Status,
			"message":  "Account awaiting admin approval",
			"user_id":  user.ID,
			"username": user.Username,
			"role":     user.Role,
		})
		return
	}

	token, refreshToken, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{
		"token":         token,
		"refresh_token": refreshToken,
		"role":          user.Role,
	})
}

func (h *AuthHandler) TelegramAuthHandler(w http.ResponseWriter, r *http.Request) {
	var tgData map[string]string
	if err := json.NewDecoder(r.Body).Decode(&tgData); err != nil {
		log.Printf("Failed to decode Telegram auth request body: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	validatedData, err := h.telegramAuthService.ValidateAndExtract(tgData)
	if err != nil {
		log.Printf("Telegram auth validation failed: %v", err)
		RespondWithError(w, http.StatusUnauthorized, "Telegram auth failed: "+err.Error())
		return
	}

	if validatedData == nil {
		log.Println("Telegram auth validation returned nil data")
		RespondWithError(w, http.StatusUnauthorized, "Telegram auth validation returned nil data")
		return
	}

	tgIDStr := validatedData["id"]
	if tgIDStr == "" {
		log.Println("Missing 'id' in validated Telegram data")
		RespondWithError(w, http.StatusBadRequest, "Missing Telegram user ID")
		return
	}

	tgID, err := strconv.Atoi(tgIDStr)
	if err != nil {
		log.Printf("Invalid Telegram ID format: %s", tgIDStr)
		RespondWithError(w, http.StatusInternalServerError, "Invalid Telegram ID format")
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  string
		TelegramID sql.NullInt64
		Role       string
		Status     string
	}

	// Сначала ищем по telegram_id
	err = h.db.QueryRow(`
		SELECT id, username, first_name, telegram_id, role, status
		FROM users
		WHERE telegram_id = $1`,
		tgID,
	).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role, &user.Status)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("Database error finding user by telegram_id %d: %v", tgID, err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Если не найден по telegram_id — пробуем найти или создать
	if errors.Is(err, sql.ErrNoRows) {
		tgUsername := validatedData["username"]
		if tgUsername == "" {
			tgUsername = "tg_user_" + tgIDStr
		}

		// Пытаемся найти по username
		err = h.db.QueryRow(`
			SELECT id, username, first_name, telegram_id, role, status
			FROM users
			WHERE LOWER(username) = LOWER($1)`,
			tgUsername,
		).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role, &user.Status)

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Database error finding user by username %s: %v", tgUsername, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		if errors.Is(err, sql.ErrNoRows) {
			// Создаём нового пользователя
			firstName := validatedData["first_name"]
			if firstName == "" {
				firstName = tgUsername
			}

			err = h.db.QueryRow(`
				INSERT INTO users (telegram_id, username, first_name, role, status, is_active)
				VALUES ($1, $2, $3, 'user', 'pending', TRUE)
				RETURNING id, username, first_name`,
				tgID,
				tgUsername,
				firstName,
			).Scan(&user.ID, &user.Username, &user.FirstName)

			if err != nil {
				log.Printf("Failed to create new user for telegram_id %d: %v", tgID, err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create user")
				return
			}

			user.TelegramID = sql.NullInt64{Int64: int64(tgID), Valid: true}
			user.Role = "user"
			user.Status = "pending"
		} else {
			// Найден по username, но без telegram_id — обновляем
			_, err = h.db.Exec(`UPDATE users SET telegram_id = $1 WHERE id = $2`, tgID, user.ID)
			if err != nil {
				log.Printf("Failed to update user %d with telegram_id %d: %v", user.ID, tgID, err)
				// Не критично — продолжаем
			}
			user.TelegramID = sql.NullInt64{Int64: int64(tgID), Valid: true}
		}
	} else {
		// Найден по telegram_id — обновляем first_name на всякий случай
		_, err = h.db.Exec(`
			UPDATE users SET first_name = $1 WHERE id = $2`,
			validatedData["first_name"],
			user.ID,
		)
		if err != nil {
			log.Printf("Failed to update first_name for user %d: %v", user.ID, err)
			// Не критично
		}
	}

	// Проверка статуса
	if user.Status == "pending" && user.Role != "superadmin" {
		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"status":      user.Status,
			"message":     "Account awaiting admin approval",
			"user_id":     user.ID,
			"username":    user.Username,
			"first_name":  user.FirstName,
			"telegram_id": user.TelegramID.Int64,
			"role":        user.Role,
		})
		return
	}

	// Генерация токенов
	token, refreshToken, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		log.Printf("Failed to generate JWT tokens for user ID %d: %v", user.ID, err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
		"first_name":    user.FirstName,
		"telegram_id":   user.TelegramID.Int64,
		"role":          user.Role,
		"status":        user.Status,
	})
}

func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) TelegramAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			data[key] = values[0]
		}
	}

	if _, ok := data["id"]; !ok {
		RespondWithError(w, http.StatusBadRequest, "Missing 'id'")
		return
	}
	if _, ok := data["hash"]; !ok {
		RespondWithError(w, http.StatusBadRequest, "Missing 'hash'")
		return
	}

	log.Printf("Telegram callback received: %+v", data)

	jsonData, err := json.Marshal(data)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to encode data")
		return
	}

	resp, err := http.Post(
		"https://start.eom.kz/api/auth/telegram ",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Internal service error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Invalid response from auth handler")
		return
	}

	if resp.StatusCode == http.StatusOK {
		token, ok := result["token"].(string)
		if !ok {
			RespondWithError(w, http.StatusInternalServerError, "Token not found in response")
			return
		}

		html := fmt.Sprintf(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>Auth Success</title>
				<script>
					window.location.href = "https://start.eom.kz/api/auth/telegram-success?token=%s";
				</script>
			</head>
			<body>
				<p>Авторизация прошла успешно... Перенаправление...</p>
			</body>
			</html>
			`, token)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	} else {
		errorMsg, _ := result["error"].(string)
		if errorMsg == "" {
			errorMsg = "Authorization failed"
		}
		RespondWithError(w, resp.StatusCode, errorMsg)
	}
}

func (h *AuthHandler) CompleteRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDVal := ctx.Value(config.UserIDKey)
	if userIDVal == nil {
		RespondWithError(w, http.StatusUnauthorized, "User ID not found in context")
		return
	}
	userID, ok := userIDVal.(int)
	if !ok {
		RespondWithError(w, http.StatusInternalServerError, "Invalid User ID type in context")
		return
	}

	var regData struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Phone     string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&regData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data: "+err.Error())
		return
	}
	if regData.FirstName == "" {
		RespondWithError(w, http.StatusBadRequest, "First name is required")
		return
	}

	_, err := h.db.Exec(`
		UPDATE users 
		SET first_name = $1, last_name = $2, phone = $3, status = 'pending', is_active = FALSE 
		WHERE id = $4`,
		regData.FirstName,
		regData.LastName,
		regData.Phone,
		userID,
	)
	if err != nil {
		log.Printf("Database error updating user %d: %v", userID, err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to update user profile")
		return
	}

	log.Printf("User %d completed registration and is now pending approval", userID)

	var updatedUser struct {
		Status   string `json:"status"`
		IsActive bool   `json:"is_active"`
	}
	err = h.db.QueryRow("SELECT status, is_active FROM users WHERE id = $1", userID).Scan(&updatedUser.Status, &updatedUser.IsActive)
	if err != nil {
		log.Printf("Database error fetching updated user %d: %v", userID, err)
		updatedUser.Status = "pending"
		updatedUser.IsActive = false
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Registration completed. Awaiting administrator approval.",
		"status":    updatedUser.Status,
		"is_active": updatedUser.IsActive,
	})
} // handlers/auto_end_shifts_handler.go
package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"time"
)

// AutoEndShifts проверяет активные смены и завершает те, что вышли за пределы временного диапазона
func AutoEndShifts(db *sql.DB) (int, error) {
	query := `
		SELECT s.id, s.user_id, s.slot_time_range, s.start_time 
		FROM slots s
		WHERE s.end_time IS NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("DB query error (active slots): %v", err)
		return 0, err
	}
	defer rows.Close()

	var toEnd []struct{ ID, UserID int }

	for rows.Next() {
		var id, userID int
		var slotTimeRange string
		var startTime time.Time

		if err := rows.Scan(&id, &userID, &slotTimeRange, &startTime); err != nil {
			log.Printf("Error scanning active slot: %v", err)
			continue
		}

		// Нормализуем временной слот (убираем лишние пробелы, приводим к стандарту)
		slotTimeRange = NormalizeSlot(slotTimeRange)

		// Получаем текущее время (уже в локальном поясе сервера, например +05:00)
		now := time.Now()

		// Определяем время окончания смены
		var endTime time.Time
		switch slotTimeRange {
		case "07:00-15:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
		case "15:00-23:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
		case "07:00-23:00":
			endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
		default:
			log.Printf("Invalid slot time range: %s", slotTimeRange)
			continue
		}

		// Если текущее время позже времени окончания — завершаем смену
		if now.After(endTime) {
			toEnd = append(toEnd, struct{ ID, UserID int }{ID: id, UserID: userID})
		}
	}

	if err = rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
		return 0, err
	}

	// Завершаем смены
	endedCount := 0
	for _, slot := range toEnd {
		if err := endSlot(db, slot.ID, slot.UserID); err != nil {
			log.Printf("Failed to auto-end slot ID %d: %v", slot.ID, err)
		} else {
			endedCount++
		}
	}

	return endedCount, nil
}

// AutoEndShiftsHandler — HTTP-эндпоинт для ручного вызова (например, для дебага)
func AutoEndShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		endedCount, err := AutoEndShifts(db)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to process auto-end shifts")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":      "Auto-end shifts completed",
			"slots_ended":  endedCount,
			"processed_at": time.Now().Format(time.RFC3339), // в локальном времени
		})
	}
}

// endSlot — закрывает одну смену
func endSlot(db *sql.DB, slotID, userID int) error {
	var startTime time.Time
	err := db.QueryRow("SELECT start_time FROM slots WHERE id = $1 AND end_time IS NULL", slotID).Scan(&startTime)
	if err == sql.ErrNoRows {
		return nil // смена уже завершена
	} else if err != nil {
		return err
	}

	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Seconds())

	_, err = db.Exec("UPDATE slots SET end_time = $1, worked_duration = $2 WHERE id = $3", endTime, duration, slotID)
	return err
}

// NormalizeSlot приводит временной слот к единому формату
/*func NormalizeSlot(slot string) string {
	switch slot {
	case "07:00 - 15:00", "07:00–15:00", "07:00-15:00", "7:00-15:00":
		return "07:00-15:00"
	case "15:00 - 23:00", "15:00–23:00", "15:00-23:00", "15:00-23:00":
		return "15:00-23:00"
	case "07:00 - 23:00", "07:00–23:00", "07:00-23:00":
		return "07:00-23:00"
	default:
		return slot
	}
}
*/
package handlers

import (
	"database/sql"
	"log"
	"net/http"
)

type EndedShift struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	SlotTimeRange string `json:"slot_time_range"`
	Position      string `json:"position"`
	Zone          string `json:"zone"`
	Selfie        string `json:"selfie"`
}

func GetEndedShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := `
			SELECT s.id, s.user_id, u.username, s.start_time, s.end_time, 
			       s.slot_time_range, s.position, s.zone, s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.end_time IS NOT NULL
			ORDER BY s.end_time DESC
		`

		rows, err := db.Query(query)
		if err != nil {
			log.Printf("DB query error (ended shifts): %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var shifts []EndedShift
		for rows.Next() {
			var shift EndedShift
			var endTime sql.NullString
			err := rows.Scan(
				&shift.ID,
				&shift.UserID,
				&shift.Username,
				&shift.StartTime,
				&endTime,
				&shift.SlotTimeRange,
				&shift.Position,
				&shift.Zone,
				&shift.Selfie,
			)
			if err != nil {
				log.Printf("Error scanning ended shift row: %v", err)
				continue
			}
			shift.EndTime = endTime.String
			shifts = append(shifts, shift)
		}

		if err = rows.Err(); err != nil {
			log.Printf("Row iteration error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}
// handlers/force_end_shift.go
package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func ForceEndShiftHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		var slotID int
		var startTime time.Time
		err = db.QueryRow(`
            SELECT id, start_time 
            FROM slots 
            WHERE user_id = $1 AND end_time IS NULL
        `, userID).Scan(&slotID, &startTime)
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "No active slot found for the user")
			return
		} else if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		endTime := time.Now()
		duration := int(endTime.Sub(startTime).Seconds())

		_, err = db.Exec(`
            UPDATE slots 
            SET end_time = $1, worked_duration = $2 
            WHERE id = $3
        `, endTime, duration, slotID)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":     "Slot ended",
			"worked_time": FormatDuration(duration), // ✅ без handlers.
		})
	}
}
// handlers/geotrack_handler.go

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/models"
	"github.com/evn/eom_backendl/services"
)

type GeoTrackHandler struct {
	service *services.GeoTrackService
}

func NewGeoTrackHandler(service *services.GeoTrackService) *GeoTrackHandler {
	return &GeoTrackHandler{service: service}
}

func (h *GeoTrackHandler) PostGeo(w http.ResponseWriter, r *http.Request) {
	var update models.GeoUpdate

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	userID, ok := r.Context().Value(config.UserIDKey).(int)
	if !ok {
		RespondWithError(w, http.StatusUnauthorized, "User ID not found in context")
		return
	}
	update.UserID = strconv.Itoa(userID)

	if err := h.service.HandleUpdate(r.Context(), &update); err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to save location")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *GeoTrackHandler) GetLast(w http.ResponseWriter, r *http.Request) {
	locations, err := h.service.GetLastLocations(r.Context())
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "DB error")
		return
	}
	RespondWithJSON(w, http.StatusOK, locations)
}
// handlers/history.go
package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func GetHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.Atoi(chi.URLParam(r, "user"))
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		rows, err := db.Query(`
			SELECT lat, lng, timestamp FROM location_history 
			WHERE user_id = $1 
			ORDER BY timestamp DESC LIMIT 100
		`, userID)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "DB error")
			return
		}
		defer rows.Close()

		var history []map[string]interface{}
		for rows.Next() {
			var lat, lng float64
			var timestamp time.Time
			if err := rows.Scan(&lat, &lng, &timestamp); err != nil {
				continue
			}
			history = append(history, map[string]interface{}{
				"lat":       lat,
				"lng":       lng,
				"timestamp": timestamp,
			})
		}
		RespondWithJSON(w, http.StatusOK, history)
	}
}
// handlers/map_handler.go
package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type MapHandler struct {
	db *sql.DB
}

func NewMapHandler(db *sql.DB) *MapHandler {
	return &MapHandler{db: db}
}

type Map struct {
	ID          int    `json:"id"`
	City        string `json:"city"`
	Description string `json:"description"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	UploadDate  string `json:"upload_date"`
}

// UploadMapHandler загружает новую карту
func (h *MapHandler) UploadMapHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Ограничиваем размер загружаемого файла до 40 МБ
	err := r.ParseMultipartForm(40 << 20)
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Request too large or invalid")
		return
	}

	city := r.FormValue("city")
	if city == "" {
		RespondWithError(w, http.StatusBadRequest, "City is required")
		return
	}

	description := r.FormValue("description")

	file, handler, err := r.FormFile("geojson_file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Error retrieving file")
		return
	}
	defer file.Close()

	ext := filepath.Ext(handler.Filename)
	if ext != ".geojson" && ext != ".json" {
		RespondWithError(w, http.StatusBadRequest, "Only .geojson and .json files are allowed")
		return
	}

	mapDir := "./uploads/maps"
	if err := os.MkdirAll(mapDir, 0755); err != nil {
		log.Printf("Error creating map directory: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error creating directory")
		return
	}

	// Сначала создаём запись в БД, чтобы получить уникальный ID
	var mapID int
	err = h.db.QueryRow(`
		INSERT INTO maps (city, description, file_name, file_size)
		VALUES ($1, $2, '', 0)
		RETURNING id
	`, city, description).Scan(&mapID)
	if err != nil {
		log.Printf("Error inserting map into database: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error saving map to database")
		return
	}

	// Генерируем имя файла с использованием реального ID
	filename := fmt.Sprintf("map_%d%s", mapID, ext)
	filePath := filepath.Join(mapDir, filename)

	// Создаём файл на диске
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		// Откат: удаляем запись из БД
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error creating file")
		return
	}
	defer dst.Close()

	// Копируем содержимое
	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Error copying file: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error saving file")
		return
	}

	// Получаем размер файла
	fileInfo, err := dst.Stat()
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error reading file")
		return
	}

	// Обновляем запись в БД с реальными данными файла
	_, err = h.db.Exec(`
		UPDATE maps
		SET file_name = $1, file_size = $2
		WHERE id = $3
	`, filename, fileInfo.Size(), mapID)
	if err != nil {
		log.Printf("Error updating map record: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error finalizing map upload")
		return
	}

	response := map[string]interface{}{
		"id":          mapID,
		"city":        city,
		"description": description,
		"file_name":   filename,
		"file_size":   fileInfo.Size(),
		"message":     "Map uploaded successfully",
	}
	RespondWithJSON(w, http.StatusCreated, response)
}

// GetMapsHandler возвращает список всех загруженных карт
func (h *MapHandler) GetMapsHandler(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, city, description, file_name, file_size, upload_date
		FROM maps
		ORDER BY upload_date DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var maps []Map
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.City, &m.Description, &m.FileName, &m.FileSize, &m.UploadDate); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		maps = append(maps, m)
	}

	RespondWithJSON(w, http.StatusOK, maps)
}

// GetMapByIDHandler возвращает информацию о конкретной карте
func (h *MapHandler) GetMapByIDHandler(w http.ResponseWriter, r *http.Request) {
	mapIDStr := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	var m Map
	query := `
		SELECT id, city, description, file_name, file_size, upload_date
		FROM maps
		WHERE id = $1
	`
	err = h.db.QueryRow(query, id).Scan(&m.ID, &m.City, &m.Description, &m.FileName, &m.FileSize, &m.UploadDate)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Map not found")
			return
		}
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	RespondWithJSON(w, http.StatusOK, m)
}

// DeleteMapHandler удаляет карту
func (h *MapHandler) DeleteMapHandler(w http.ResponseWriter, r *http.Request) {
	mapIDStr := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	var fileName string
	err = h.db.QueryRow("SELECT file_name FROM maps WHERE id = $1", id).Scan(&fileName)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Map not found")
			return
		}
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	_, err = h.db.Exec("DELETE FROM maps WHERE id = $1", id)
	if err != nil {
		log.Printf("Database delete error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	filePath := filepath.Join("./uploads/maps", fileName)
	if err := os.Remove(filePath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Warning: failed to delete map file %s: %v", filePath, err)
		}
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Map deleted successfully"})
}

// ServeMapFileHandler отдает файл карты для скачивания
func (h *MapHandler) ServeMapFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	filePath := filepath.Join("./uploads/maps", filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		RespondWithError(w, http.StatusNotFound, "File not found")
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	http.ServeFile(w, r, filePath)
}

// CreateMapsTable создает таблицу для хранения информации о картах
func CreateMapsTable(db *sql.DB) error {
	query := `
    CREATE TABLE IF NOT EXISTS maps (
        id SERIAL PRIMARY KEY,
        city TEXT NOT NULL,
        description TEXT,
        file_name TEXT NOT NULL,
        file_size BIGINT NOT NULL,
        upload_date TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    );
    `
	_, err := db.Exec(query)
	return err
}
package handlers

import (
	"net/http"

	"github.com/go-chi/jwtauth/v5"
)

func AdminOnlyMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, claims, err := jwtauth.FromContext(r.Context())
			
			// Проверяем наличие и валидность токена
			if err != nil || token == nil {
				RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			// Проверяем роль пользователя
			if role, ok := claims["role"].(string); !ok || role != "admin" {
				RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
// handlers/profile.go
package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/jwtauth/v5"
)

type ProfileHandler struct {
	db *sql.DB
}

func NewProfileHandler(db *sql.DB) *ProfileHandler {
	return &ProfileHandler{db: db}
}

func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	var userID int
	if rawID, ok := claims["user_id"]; ok {
		switch v := rawID.(type) {
		case float64:
			userID = int(v)
		case int:
			userID = v
		case string:
			if id, err := strconv.Atoi(v); err == nil {
				userID = id
			} else {
				RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
				return
			}
		default:
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID type")
			return
		}
	} else {
		RespondWithError(w, http.StatusBadRequest, "User ID not found in token")
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  sql.NullString
		TelegramID sql.NullInt64
		Role       string
		AvatarURL  sql.NullString
		Zone       sql.NullString
		Status     string
		IsActive   bool
	}

	err = h.db.QueryRow(`
	SELECT id, username, first_name, telegram_id, role, avatar_url, zone, status, is_active
	FROM users
	WHERE id = $1`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.FirstName,
		&user.TelegramID,
		&user.Role,
		&user.AvatarURL,
		&user.Zone,
		&user.Status,
		&user.IsActive,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Пользователь не найден — это не ошибка сервера
			log.Printf("User not found (ID: %d)", userID)
			RespondWithError(w, http.StatusNotFound, "User not found")
			return
		}
		// Настоящая ошибка базы данных (например, подключение, синтаксис и т.д.)
		log.Printf("Database error in GetProfile (user %d): %v", userID, err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	firstName := ""
	if user.FirstName.Valid {
		firstName = user.FirstName.String
	}

	var position string
	switch user.Role {
	case "scout":
		position = "Скаут"
	case "supervisor":
		position = "Супервайзер"
	case "coordinator":
		position = "Координатор"
	case "superadmin":
		position = "Суперадмин"
	case "courier":
		position = "Курьер"
	default:
		position = "Стажер"
	}

	var finalAvatarURL interface{}
	if user.AvatarURL.Valid && user.AvatarURL.String != "" {
		finalAvatarURL = user.AvatarURL.String
	} else {
		finalAvatarURL = nil
	}

	var finalZone interface{}
	if user.Zone.Valid && user.Zone.String != "" {
		finalZone = user.Zone.String
	} else {
		finalZone = "Центр"
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"firstName":  firstName,
		"telegramId": nullInt64ToInterface(user.TelegramID),
		"role":       user.Role,
		"avatarUrl":  finalAvatarURL,
		"position":   position,
		"zone":       finalZone,
		"status":     user.Status,
		"is_active":  user.IsActive,
	})
}

func nullInt64ToInterface(n sql.NullInt64) interface{} {
	if n.Valid {
		return n.Int64
	}
	return nil
}
package handlers

import (
	"net/http"
    "encoding/json"

)

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
// handlers/scooter_stats_handler.go
package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	// Замените "github.com/evn/eom_backendl" на имя вашего модуля из go.mod, если оно другое
	"github.com/evn/eom_backendl/models"

	_ "github.com/mattn/go-sqlite3"
)

// ScooterStatsHandler структура для обработчика статистики
type ScooterStatsHandler struct {
	botDBPath string
}

// NewScooterStatsHandler создает новый экземпляр обработчика
func NewScooterStatsHandler(botDBPath string) *ScooterStatsHandler {
	return &ScooterStatsHandler{
		botDBPath: botDBPath,
	}
}

// GetShiftStatsHandler обрабатывает запрос /api/scooter-stats/shift
func (h *ScooterStatsHandler) GetShiftStatsHandler(w http.ResponseWriter, r *http.Request) {
	// Открываем соединение с SQLite базой бота
	botDB, err := sql.Open("sqlite3", h.botDBPath)
	if err != nil {
		log.Printf("Error opening scooter DB: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to connect to scooter database")
		return
	}
	defer func() {
		if closeErr := botDB.Close(); closeErr != nil {
			log.Printf("Error closing scooter DB: %v", closeErr)
		}
	}()

	// Проверка соединения
	if err := botDB.Ping(); err != nil {
		log.Printf("Error pinging scooter DB: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to ping scooter database")
		return
	}

	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		log.Printf("Error loading timezone: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	now := time.Now().In(loc)
	startTime, endTime, shiftName := getShiftTimeRange(now, loc)

	// Запрос к базе данных бота
	query := `
		SELECT 
			service, 
			accepted_by_user_id, 
			accepted_by_username, 
			accepted_by_fullname 
		FROM accepted_scooters 
		WHERE timestamp BETWEEN $1 AND $2
	`
	rows, err := botDB.Query(query, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Printf("Error querying scooter database: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	// Структуры для сбора данных
	userStats := make(map[string]*models.ScooterStat) // map[user_id_string]*ScooterStat
	serviceTotals := make(map[string]int)
	totalAll := 0

	for rows.Next() {
		var service string
		var userID int64
		var username, fullName sql.NullString

		err := rows.Scan(&service, &userID, &username, &fullName)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		userIDStr := fmt.Sprintf("%d", userID)

		// Инициализируем статистику пользователя, если нужно
		if _, exists := userStats[userIDStr]; !exists {
			userStats[userIDStr] = &models.ScooterStat{
				Username: username.String,
				FullName: fullName.String,
				Services: make(map[string]int),
				Total:    0,
			}
		}

		// Обновляем данные пользователя
		userStats[userIDStr].Services[service]++
		userStats[userIDStr].Total++

		// Обновляем общие итоги
		serviceTotals[service]++
		totalAll++
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	response := models.ShiftStats{
		ShiftName: shiftName,
		StartTime: startTime,
		EndTime:   endTime,
		Stats:     userStats,
		Totals:    serviceTotals,
		TotalAll:  totalAll,
	}

	RespondWithJSON(w, http.StatusOK, response)
}

// getShiftTimeRange определяет текущую смену
func getShiftTimeRange(now time.Time, loc *time.Location) (time.Time, time.Time, string) {
	today := now.Truncate(24 * time.Hour)

	morningShiftStart := time.Date(today.Year(), today.Month(), today.Day(), 7, 0, 0, 0, loc)
	morningShiftEnd := time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, loc)
	eveningShiftStart := time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, loc)
	eveningShiftEnd := time.Date(today.Year(), today.Month(), today.Day()+1, 4, 0, 0, 0, loc) // +1 день

	if (now.After(morningShiftStart) || now.Equal(morningShiftStart)) && now.Before(morningShiftEnd) {
		return morningShiftStart, morningShiftEnd, "утреннюю смену"
	} else if (now.After(eveningShiftStart) || now.Equal(eveningShiftStart)) && now.Before(eveningShiftEnd) {
		return eveningShiftStart, eveningShiftEnd, "вечернюю смену"
	} else {
		// Если сейчас ночь (00:00 - 07:00), считаем это концом предыдущей вечерней смены
		prevEveningStart := time.Date(today.Year(), today.Month(), today.Day()-1, 15, 0, 0, 0, loc)
		return prevEveningStart, morningShiftStart, "вечернюю смену (с учетом ночных часов)"
	}
}
// handlers/shift_generator.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type GenerateShiftsRequest struct {
	Date         string `json:"date"`
	MorningCount int    `json:"morning_count"`
	EveningCount int    `json:"evening_count"`
	ScoutIDs     []int  `json:"scout_ids"`
}

func GenerateShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GenerateShiftsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD")
			return
		}

		validScouts, err := filterAvailableScouts(db, req.ScoutIDs, date)
		if err != nil {
			log.Printf("Error checking scout availability: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		if len(validScouts) < req.MorningCount+req.EveningCount {
			RespondWithError(w, http.StatusBadRequest, "Недостаточно доступных скаутов")
			return
		}

		tx, err := db.Begin()
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Transaction error")
			return
		}
		defer tx.Rollback()

		slotTime := "07:00 - 15:00"
		for i := 0; i < req.MorningCount; i++ {
			if i >= len(validScouts) {
				break
			}
			if err := createSlot(tx, validScouts[i], date, slotTime); err != nil {
				log.Printf("Failed to create morning shift: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create morning shift")
				return
			}
		}

		slotTime = "15:00 - 23:00"
		for i := req.MorningCount; i < req.MorningCount+req.EveningCount; i++ {
			if i >= len(validScouts) {
				break
			}
			if err := createSlot(tx, validScouts[i], date, slotTime); err != nil {
				log.Printf("Failed to create evening shift: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create evening shift")
				return
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Shift generation commit error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Commit error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "Смены сгенерированы",
		})
	}
}

func filterAvailableScouts(db *sql.DB, scoutIDs []int, date time.Time) ([]int, error) {
	if len(scoutIDs) == 0 {
		return []int{}, nil
	}

	placeholders := make([]string, len(scoutIDs))
	args := make([]interface{}, len(scoutIDs)+1)
	for i, id := range scoutIDs {
		placeholders[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}
	args[len(scoutIDs)] = date.Format("2006-01-02")

	query := fmt.Sprintf(`
		SELECT user_id FROM slots 
		WHERE user_id IN (%s) 
		AND DATE(start_time) = $%d
		AND end_time IS NULL
	`, strings.Join(placeholders, ","), len(scoutIDs)+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	busy := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		busy[id] = true
	}

	var available []int
	for _, id := range scoutIDs {
		if !busy[id] {
			available = append(available, id)
		}
	}
	return available, nil
}

func createSlot(tx *sql.Tx, userID int, date time.Time, slotTime string) error {
	_, err := tx.Exec(`
		INSERT INTO slots (user_id, start_time, slot_time_range, position, zone, selfie_path) 
		VALUES ($1, $2, $3, 'Скаут', 'Центр', '')
	`, userID, date.Format("2006-01-02 07:00:00"), slotTime)
	return err
}

func GetShiftsByDateHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dateStr := chi.URLParam(r, "date")
		if dateStr == "" {
			RespondWithError(w, http.StatusBadRequest, "Date is required")
			return
		}

		_, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD")
			return
		}

		query := `
			SELECT 
				s.id,
				s.user_id,
				u.username,
				u.first_name,
				s.start_time,
				s.slot_time_range,
				s.position,
				s.zone,
				s.selfie_path,
				s.end_time
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE DATE(s.start_time) = $1
			ORDER BY s.start_time
		`

		rows, err := db.Query(query, dateStr)
		if err != nil {
			log.Printf("Error querying shifts for date %s: %v", dateStr, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var id, userID int
			var username, firstName, startTime, slotTimeRange, position, zone, selfie, endTime sql.NullString
			if err := rows.Scan(&id, &userID, &username, &firstName, &startTime, &slotTimeRange, &position, &zone, &selfie, &endTime); err != nil {
				log.Printf("Error scanning shift row: %v", err)
				continue
			}

			shift := map[string]interface{}{
				"id":              id,
				"user_id":         userID,
				"username":        username.String,
				"first_name":      firstName.String,
				"start_time":      startTime.String,
				"shift_type":      getShiftTypeFromTimeRange(slotTimeRange.String),
				"position":        position.String,
				"zone":            zone.String,
				"selfie":          selfie.String,
				"end_time":        endTime.String,
			}
			shifts = append(shifts, shift)
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}

func getShiftTypeFromTimeRange(timeRange string) string {
	if strings.Contains(timeRange, "07:00") {
		return "morning"
	} else if strings.Contains(timeRange, "15:00") {
		return "evening"
	}
	return "unknown"
}// handlers/slot_handler.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"crypto/rand"
	_ "image/jpeg"
	_ "image/png"
	"github.com/evn/eom_backendl/config"
	"github.com/go-chi/chi/v5"
)

// -------------------------------
// Вспомогательные функции
// -------------------------------

// generateSafeFilename генерирует уникальное имя файла для селфи
func generateSafeFilename(userID int, ext string) string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("selfie_%d_%d%s", userID, time.Now().UnixNano(), ext)
	}
	hash := fmt.Sprintf("%x", randomBytes)
	return fmt.Sprintf("selfie_%d_%s%s", userID, hash, ext)
}

// -------------------------------
// Обработчики
// -------------------------------

func StartSlotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var activeCount int
		err := db.QueryRow("SELECT COUNT(*) FROM slots WHERE user_id = $1 AND end_time IS NULL", userID).Scan(&activeCount)
		if err != nil {
			log.Printf("DB error checking active slots for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if activeCount > 0 {
			RespondWithError(w, http.StatusBadRequest, "Slot already active")
			return
		}

		var position string
		err = db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&position)
		if err != nil {
			log.Printf("DB error fetching role for user %d: %v", userID, err)
			position = "user"
		}

		positionMap := map[string]string{
			"superadmin":   "Суперадмин",
			"admin":        "Администратор",
			"coordinator":  "Координатор",
			"scout":        "Скаут",
			"user":         "Пользователь",
		}

		if readablePosition, exists := positionMap[position]; exists {
			position = readablePosition
		} else {
			position = "Сотрудник"
		}

		if err := r.ParseMultipartForm(5 << 20); err != nil {
			RespondWithError(w, http.StatusBadRequest, "File too large or malformed")
			return
		}

		slotTimeRange := r.FormValue("slot_time_range")
		zone := r.FormValue("zone")

		if slotTimeRange == "" || zone == "" {
			RespondWithError(w, http.StatusBadRequest, "Missing required fields")
			return
		}

		// Нормализуем временной слот
		slotTimeRange = NormalizeSlot(slotTimeRange)

		if !canStartShift(slotTimeRange) {
			RespondWithError(w, http.StatusBadRequest, "Смену можно начать только за 20 минут до её начала или в течение смены")
			return
		}

		// Проверяем, существует ли зона
		var zoneExists int
		err = db.QueryRow("SELECT COUNT(*) FROM zones WHERE name = $1", zone).Scan(&zoneExists)
		if err != nil {
			log.Printf("DB error checking zone %s: %v", zone, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if zoneExists == 0 {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone: "+zone)
			return
		}

		// Проверяем, существует ли временной слот
		var slotExists int
		err = db.QueryRow("SELECT COUNT(*) FROM available_time_slots WHERE slot_time_range = $1", slotTimeRange).Scan(&slotExists)
		if err != nil {
			log.Printf("DB error checking time slot %s: %v", slotTimeRange, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if slotExists == 0 {
			RespondWithError(w, http.StatusBadRequest, "Invalid time slot: "+slotTimeRange)
			return
		}

		// Проверяем селфи
		file, _, err := r.FormFile("selfie")
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Selfie image is required")
			return
		}
		defer file.Close()

		buff := make([]byte, 512)
		_, err = file.Read(buff)
		if err != nil && err != io.EOF {
			RespondWithError(w, http.StatusInternalServerError, "Error reading file")
			return
		}
		contentType := http.DetectContentType(buff)
		if contentType != "image/jpeg" && contentType != "image/png" {
			RespondWithError(w, http.StatusBadRequest, "Only JPEG and PNG images allowed")
			return
		}

		file.Seek(0, 0)
		ext := ".jpg"
		if contentType == "image/png" {
			ext = ".png"
		}

		filename := generateSafeFilename(userID, ext)
		fullPath := filepath.Join("./uploads/selfies", filename)
		if err := os.MkdirAll("./uploads/selfies", 0755); err != nil {
			log.Printf("Failed to create uploads dir: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Server error")
			return
		}

		out, err := os.Create(fullPath)
		if err != nil {
			log.Printf("Failed to create file %s: %v", fullPath, err)
			RespondWithError(w, http.StatusInternalServerError, "Server error")
			return
		}
		defer out.Close()

		_, err = io.Copy(out, file)
		if err != nil {
			os.Remove(fullPath)
			log.Printf("Failed to save selfie for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to save image")
			return
		}

		// Вставляем смену
		result, err := db.Exec(`
			INSERT INTO slots (user_id, start_time, slot_time_range, position, zone, selfie_path)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, time.Now(), slotTimeRange, position, zone, "/uploads/selfies/"+filename)

		if err != nil {
			os.Remove(fullPath)
			log.Printf("DB error inserting slot for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		slotID, err := result.LastInsertId()
		if err != nil {
			log.Printf("Failed to get slot ID: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to get slot ID")
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"message":           "Slot started successfully",
			"selfie":            "/uploads/selfies/" + filename,
			"id":                slotID,
			"user_id":           userID,
			"slot_time_range":   slotTimeRange,
			"position":          position,
			"zone":              zone,
			"start_time":        time.Now().Format(time.RFC3339),
		})
	}
}

// canStartShift проверяет, можно ли начать смену в текущее время
func canStartShift(slotTimeRange string) bool {
	now := time.Now()
	hour, min := now.Hour(), now.Minute()

	switch slotTimeRange {
	case "07:00-15:00":
		return (hour == 6 && min >= 40) || (hour >= 7 && hour < 15) || (hour == 15 && min == 0)
	case "15:00-23:00":
		return (hour == 14 && min >= 40) || (hour >= 15 && hour < 23) || (hour == 23 && min == 0)
	case "07:00-23:00":
		return (hour == 6 && min >= 40) || (hour >= 7 && hour < 23) || (hour == 23 && min == 0)
	default:
		return false
	}
}

func EndSlotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var slotID int
		var startTime time.Time
		err := db.QueryRow(`
			SELECT id, start_time FROM slots WHERE user_id = $1 AND end_time IS NULL
		`, userID).Scan(&slotID, &startTime)
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusBadRequest, "No active slot found")
			return
		} else if err != nil {
			log.Printf("DB error fetching active slot for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		endTime := time.Now()
		duration := int(endTime.Sub(startTime).Seconds())

		_, err = db.Exec(`
			UPDATE slots SET end_time = $1, worked_duration = $2 WHERE id = $3
		`, endTime, duration, slotID)
		if err != nil {
			log.Printf("DB error ending slot %d: %v", slotID, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":     "Slot ended",
			"worked_time": FormatDuration(duration),
		})
	}
}

func GetActiveShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rows, err := db.Query(`
			SELECT 
				s.id,
				s.user_id,
				u.username,
				s.slot_time_range,
				s.position,
				s.zone,
				s.start_time,
				s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.end_time IS NULL
		`)
		if err != nil {
			log.Printf("DB error fetching active shifts: %v", err)
			http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var id, userID int
			var username, slotTimeRange, position, zone, selfiePath string
			var startTime time.Time
			if err := rows.Scan(&id, &userID, &username, &slotTimeRange, &position, &zone, &startTime, &selfiePath); err != nil {
				log.Printf("Error scanning active shift row: %v", err)
				continue
			}
			slotTimeRange = NormalizeSlot(slotTimeRange)
			shifts = append(shifts, map[string]interface{}{
				"id":              id,
				"user_id":         userID,
				"username":        username,
				"slot_time_range": slotTimeRange,
				"position":        position,
				"zone":            zone,
				"start_time":      startTime,
				"is_active":       true,
				"selfie":          selfiePath,
			})
		}
		if shifts == nil {
			shifts = []map[string]interface{}{}
		}
		json.NewEncoder(w).Encode(shifts)
	}
}

func GetUserActiveShiftHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var id int
		var username, slotTimeRange, position, zone, selfiePath string
		var startTime time.Time
		err := db.QueryRow(`
			SELECT 
				s.id,
				u.username,
				s.slot_time_range,
				s.position,
				s.zone,
				s.start_time,
				s.selfie_path
			FROM slots s
			JOIN users u ON s.user_id = u.id
			WHERE s.user_id = $1 AND s.end_time IS NULL
		`, userID).Scan(&id, &username, &slotTimeRange, &position, &zone, &startTime, &selfiePath)

		if err == sql.ErrNoRows {
			w.Write([]byte("null"))
			return
		} else if err != nil {
			log.Printf("DB error fetching user active shift %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		activeShift := map[string]interface{}{
			"id":              id,
			"user_id":         userID,
			"username":        username,
			"slot_time_range": slotTimeRange,
			"position":        position,
			"zone":            zone,
			"start_time":      startTime.Format(time.RFC3339),
			"is_active":       true,
			"selfie":          selfiePath,
		}
		json.NewEncoder(w).Encode(activeShift)
	}
}

func GetShiftsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		rows, err := db.Query(`
			SELECT 
				start_time, end_time, slot_time_range, position, zone, worked_duration
			FROM slots 
			WHERE user_id = $1 AND end_time IS NOT NULL
			ORDER BY start_time DESC
		`, userID)
		if err != nil {
			log.Printf("DB error fetching shifts for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to query shifts")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var startTime, endTime time.Time
			var slotTimeRange, position, zone sql.NullString
			var workedDuration sql.NullInt64
			if err := rows.Scan(&startTime, &endTime, &slotTimeRange, &position, &zone, &workedDuration); err != nil {
				log.Printf("Error scanning shift history row: %v", err)
				continue
			}
			shift := map[string]interface{}{
				"date":             startTime.Format("2006-01-02"),
				"selected_slot":    slotTimeRange.String,
				"worked_time":      FormatDuration(int(workedDuration.Int64)),
				"work_period":      fmt.Sprintf("%s–%s", startTime.Format("15:04"), endTime.Format("15:04")),
				"transport_status": "Транспорт не указан",
				"new_tasks":        0,
			}
			shifts = append(shifts, shift)
		}
		if shifts == nil {
			shifts = []map[string]interface{}{}
		}
		RespondWithJSON(w, http.StatusOK, shifts)
	}
}

func GetUserShiftsByIDHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetUserIDStr := chi.URLParam(r, "userID")
		targetUserID, err := strconv.Atoi(targetUserIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		currentUserID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var currentUserRole string
		err = db.QueryRow("SELECT role FROM users WHERE id = $1", currentUserID).Scan(&currentUserRole)
		if err != nil {
			log.Printf("DB error fetching current user role: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to load user role")
			return
		}

		if currentUserRole != "admin" && currentUserRole != "superadmin" && currentUserID != targetUserID {
			RespondWithError(w, http.StatusForbidden, "Access denied")
			return
		}

		rows, err := db.Query(`
			SELECT 
				start_time, end_time, slot_time_range, position, zone, worked_duration
			FROM slots 
			WHERE user_id = $1 AND end_time IS NOT NULL
			ORDER BY start_time DESC
		`, targetUserID)
		if err != nil {
			log.Printf("DB error fetching shifts for user %d: %v", targetUserID, err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to query shifts")
			return
		}
		defer rows.Close()

		var shifts []map[string]interface{}
		for rows.Next() {
			var startTime, endTime time.Time
			var slotTimeRange, position, zone sql.NullString
			var workedDuration sql.NullInt64
			if err := rows.Scan(&startTime, &endTime, &slotTimeRange, &position, &zone, &workedDuration); err != nil {
				log.Printf("Error scanning target user shift row: %v", err)
				continue
			}
			shift := map[string]interface{}{
				"date":             startTime.Format("2006-01-02"),
				"selected_slot":    slotTimeRange.String,
				"worked_time":      FormatDuration(int(workedDuration.Int64)),
				"work_period":      fmt.Sprintf("%s–%s", startTime.Format("15:04"), endTime.Format("15:04")),
				"transport_status": "Транспорт не указан",
				"new_tasks":        0,
			}
			shifts = append(shifts, shift)
		}

		if shifts == nil {
			shifts = []map[string]interface{}{}
		}

		RespondWithJSON(w, http.StatusOK, shifts)
	}
}

func GetAvailablePositionsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(config.UserIDKey).(int)
		if !ok {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		var role string
		err := db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
		if err != nil {
			log.Printf("DB error fetching role for user %d: %v", userID, err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to load user role")
			return
		}

		positionMap := map[string]string{
			"superadmin":   "Суперадмин",
			"admin":        "Администратор",
			"coordinator":  "Координатор",
			"scout":        "Скаут",
			"user":         "Пользователь",
		}

		position := "Сотрудник"
		if readablePosition, exists := positionMap[role]; exists {
			position = readablePosition
		}
		RespondWithJSON(w, http.StatusOK, []string{position})
	}
}

func GetAvailableTimeSlotsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var timeSlots []string
		rows, err := db.Query("SELECT slot_time_range FROM available_time_slots")
		if err != nil {
			log.Printf("DB error fetching time slots: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to load time slots")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var timeSlot string
			if err := rows.Scan(&timeSlot); err != nil {
				log.Printf("Error scanning time slot: %v", err)
				continue
			}
			timeSlots = append(timeSlots, NormalizeSlot(timeSlot))
		}
		RespondWithJSON(w, http.StatusOK, timeSlots)
	}
}package handlers

import (
	"database/sql"
	"net/http"
	"log"
)

type User struct {
	ID        int            `json:"id"`
	Username  string         `json:"username"`
	FirstName string         `json:"first_name"`
	Role      string         `json:"role"`
}

func ListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, username, first_name, role FROM users")
		if err != nil {
			log.Printf("Error querying users: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to query users")
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var id int
			var username, firstName, role sql.NullString
			if err := rows.Scan(&id, &username, &firstName, &role); err != nil {
				log.Printf("Error scanning user row: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to process user data")
				return
			}
			
			// Создаём структуру User, преобразуя sql.NullString в обычный string
			user := User{
				ID:        id,
				Username:  username.String,
				FirstName: firstName.String,
				Role:      role.String,
			}
			users = append(users, user)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error after iterating rows: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Error processing rows")
			return
		}

		RespondWithJSON(w, http.StatusOK, users)
	}
}
package handlers

import (
	"database/sql"
	"net/http"
	"log"
)

// UserProfileResponse - структура для ответа с профилем пользователя
type UserProfileResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}
// GetProfileHandler - обработчик для получения профиля пользователя
func GetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем userID из контекста (должен быть установлен middleware аутентификации)
		userIDVal := r.Context().Value("userID")
		if userIDVal == nil {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		userID, ok := userIDVal.(int)
		if !ok {
			RespondWithError(w, http.StatusInternalServerError, "Invalid user ID format")
			return
		}

		// Структура для сканирования из БД с поддержкой NULL
		var user struct {
			ID        int            `json:"id"`
			Username  string         `json:"username"`
			FirstName sql.NullString `json:"first_name"`
			Role      string         `json:"role"`
			IsActive  bool           `json:"is_active"`
		}

		// Выполняем запрос к базе данных
		err := db.QueryRowContext(r.Context(), 
			"SELECT id, username, first_name, role, is_active FROM users WHERE id = $1", 
			userID).
			Scan(&user.ID, &user.Username, &user.FirstName, &user.Role, &user.IsActive)

		if err != nil {
			if err == sql.ErrNoRows {
				RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error fetching user %d: %v", userID, err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to fetch user profile")
			}
			return
		}

		// Преобразуем NULL в пустую строку для ответа
		firstName := ""
		if user.FirstName.Valid {
			firstName = user.FirstName.String
		}

		// Формируем ответ
		response := UserProfileResponse{
			ID:        user.ID,
			Username:  user.Username,
			FirstName: firstName,
			Role:      user.Role,
			IsActive:  user.IsActive,
		}

		RespondWithJSON(w, http.StatusOK, response)
	}
}

// AlternativeGetProfileHandler - альтернативная реализация с указателями
func AlternativeGetProfileHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDVal := r.Context().Value("userID")
		if userIDVal == nil {
			RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
			return
		}

		userID := userIDVal.(int)

		var user struct {
			ID        int     `json:"id"`
			Username  string  `json:"username"`
			FirstName *string `json:"first_name"`
			Role      string  `json:"role"`
			IsActive  bool    `json:"is_active"`
		}

		err := db.QueryRowContext(r.Context(), 
			"SELECT id, username, first_name, role, is_active FROM users WHERE id = $1", 
			userID).
			Scan(&user.ID, &user.Username, &user.FirstName, &user.Role, &user.IsActive)

		if err != nil {
			if err == sql.ErrNoRows {
				RespondWithError(w, http.StatusNotFound, "User not found")
			} else {
				log.Printf("Database error: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Database error")
			}
			return
		}

		// Если FirstName is nil, будет отправлен null в JSON
		RespondWithJSON(w, http.StatusOK, user)
	}
}
// handlers/utils.go
package handlers

import (
	"fmt"
	"strings"
)

func NormalizeSlot(slot string) string {
	slot = strings.ReplaceAll(slot, "–", "-")
	slot = strings.ReplaceAll(slot, "—", "-")
	slot = strings.ReplaceAll(slot, " ", "")
	return slot
}

func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "0 мин"
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%d ч %d мин", hours, mins)
	}
	return fmt.Sprintf("%d мин", mins)
}
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetAvailableZonesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, name FROM zones ORDER BY name")
		if err != nil {
			log.Printf("Database error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var zones []Zone
		for rows.Next() {
			var zone Zone
			if err := rows.Scan(&zone.ID, &zone.Name); err != nil {
				continue
			}
			zones = append(zones, zone)
		}

		RespondWithJSON(w, http.StatusOK, zones)
	}
}

func CreateZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var zone Zone
		if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if zone.Name == "" {
			RespondWithError(w, http.StatusBadRequest, "Zone name is required")
			return
		}

		var newID int
		err := db.QueryRow(`
			INSERT INTO zones (name) 
			VALUES ($1) 
			ON CONFLICT (name) DO NOTHING 
			RETURNING id
		`, zone.Name).Scan(&newID)

		if err != nil {
			if err == sql.ErrNoRows {
				// Запись уже существует — получаем её ID
				err = db.QueryRow("SELECT id FROM zones WHERE name = $1", zone.Name).Scan(&newID)
				if err != nil {
					log.Printf("Failed to fetch existing zone ID: %v", err)
					RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve zone")
					return
				}
			} else {
				log.Printf("Database insert error: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create zone")
				return
			}
		}

		zone.ID = newID
		RespondWithJSON(w, http.StatusCreated, zone)
	}
}

func UpdateZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone ID")
			return
		}

		var zone Zone
		if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		result, err := db.Exec("UPDATE zones SET name = $1 WHERE id = $2", zone.Name, id)
		if err != nil {
			log.Printf("Database update error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to update zone")
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "Zone not found")
			return
		}

		zone.ID = id
		RespondWithJSON(w, http.StatusOK, zone)
	}
}

func DeleteZoneHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid zone ID")
			return
		}

		result, err := db.Exec("DELETE FROM zones WHERE id = $1", id)
		if err != nil {
			log.Printf("Database delete error: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete zone")
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "Zone not found")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}
// models/active_shift.go
package models

type ActiveShift struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Username      string `json:"username"`
	StartTime     string `json:"start_time"`
	SlotTimeRange string `json:"slot_time_range"`
	Position      string `json:"position"`
	Zone          string `json:"zone"`
	SelfiePath    string `json:"selfie_path"`
}
package models

import (
    "time"
)

type AppVersion struct {
    ID             int       `json:"id" db:"id"`
    Platform       string    `json:"platform" db:"platform"`
    Version        string    `json:"version" db:"version"`
    BuildNumber    int       `json:"build_number" db:"build_number"`
    ReleaseNotes   string    `json:"release_notes" db:"release_notes"`
    DownloadURL    string    `json:"download_url" db:"download_url"`
    MinSDKVersion  int       `json:"min_sdk_version" db:"min_sdk_version"`
    IsMandatory    bool      `json:"is_mandatory" db:"is_mandatory"`
    IsActive       bool      `json:"is_active" db:"is_active"`
    CreatedAt      time.Time `json:"created_at" db:"created_at"`
    UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type VersionCheckRequest struct {
    Platform    string `json:"platform"`     // 'android' или 'ios'
    CurrentVersion string `json:"current_version"` // '1.0.0'
    BuildNumber int    `json:"build_number"` // 100
    DeviceInfo  string `json:"device_info,omitempty"`
}

type VersionCheckResponse struct {
    HasUpdate     bool       `json:"has_update"`
    LatestVersion *AppVersion `json:"latest_version,omitempty"`
    Message       string     `json:"message,omitempty"`
    IsMandatory   bool       `json:"is_mandatory"`
}
// models/location.go

package models

import "time"

type GeoUpdate struct {
	ID        int64     `json:"id,omitempty"`
	UserID    string    `json:"user_id" binding:"required"`
	Lat       float64   `json:"lat" binding:"required"`
	Lon       float64   `json:"lon" binding:"required"`
	Speed     float64   `json:"speed,omitempty"`
	Accuracy  float64   `json:"accuracy,omitempty"`
	Battery   int       `json:"battery,omitempty" binding:"min=0,max=100"`
	Event     string    `json:"event,omitempty"`
	CreatedAt time.Time `json:"ts,omitempty"`
}

// Для ответа админу
type LastLocation struct {
	UserID  string    `json:"user_id"`
	Lat     float64   `json:"lat"`
	Lon     float64   `json:"lon"`
	Battery int       `json:"battery"`
	Ts      time.Time `json:"ts"`
}
// models/scooter_stats.go
package models

import "time"

// ScooterStat представляет статистику по одному пользователю
type ScooterStat struct {
	Username string            `json:"username"`
	FullName string            `json:"full_name"`
	Services map[string]int    `json:"services"`
	Total    int               `json:"total"`
}

// ShiftStats представляет полную статистику за смену
type ShiftStats struct {
	ShiftName string                 `json:"shift_name"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
	Stats     map[string]*ScooterStat `json:"stats"` // ключ - user_id как строка
	Totals    map[string]int         `json:"totals"` // Итоги по сервисам
	TotalAll  int                    `json:"total_all"` // Общий итог
}
// models/user_shift_location.go
package models

import (
    "time"
)

type UserShiftLocation struct {
    UserID      int       `json:"user_id"`
    Username    string    `json:"username"`
    Position    string    `json:"position"`
    Zone        string    `json:"zone"`
    StartTime   time.Time `json:"start_time"`
    Lat         *float64  `json:"lat,omitempty"` // Используем указатель для различия 0 и отсутствия значения
    Lng         *float64  `json:"lng,omitempty"`
    Timestamp   *time.Time `json:"timestamp,omitempty"`
    HasLocation bool      `json:"has_location"`
}
package models

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	FirstName    string `json:"first_name,omitempty"`
	TelegramID   string `json:"telegram_id,omitempty"`
	Role         string `json:"role"`
	AvatarURL    string `json:"avatarUrl"`
	Email        string `json:"email,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Password  string `json:"password"`
	Email     string `json:"email,omitempty"`
}

type AuthResponse struct {
	Token    string `json:"token"`
	Role     string `json:"role"`
	UserID   int    `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

type TelegramAuthData struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  string `json:"auth_date"`
	Hash      string `json:"hash"`
}
package repositories

import (
    "database/sql"
    "fmt"
    // "time"
    "github.com/evn/eom_backendl/models"
)

type AppVersionRepository struct {
    DB *sql.DB
}

func NewAppVersionRepository(db *sql.DB) *AppVersionRepository {
    return &AppVersionRepository{DB: db}
}

// GetLatestVersion получает последнюю активную версию для платформы
func (r *AppVersionRepository) GetLatestVersion(platform string) (*models.AppVersion, error) {
    query := `
        SELECT id, platform, version, build_number, release_notes, download_url, 
               min_sdk_version, is_mandatory, is_active, created_at, updated_at
        FROM app_versions 
        WHERE platform = $1 AND is_active = TRUE 
        ORDER BY build_number DESC 
        LIMIT 1
    `
    
    var version models.AppVersion
    err := r.DB.QueryRow(query, platform).Scan(
        &version.ID,
        &version.Platform,
        &version.Version,
        &version.BuildNumber,
        &version.ReleaseNotes,
        &version.DownloadURL,
        &version.MinSDKVersion,
        &version.IsMandatory,
        &version.IsActive,
        &version.CreatedAt,
        &version.UpdatedAt,
    )
    
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, fmt.Errorf("no active version found for platform %s", platform)
        }
        return nil, fmt.Errorf("failed to get latest version: %w", err)
    }
    
    return &version, nil
}
// CheckVersion проверяет, доступна ли новая версия
func (r *AppVersionRepository) CheckVersion(platform, currentVersion string, buildNumber int) (*models.VersionCheckResponse, error) {
    latestVersion, err := r.GetLatestVersion(platform)
    if err != nil {
        return &models.VersionCheckResponse{
            HasUpdate:   false,
            Message:     "No active versions available",
            IsMandatory: false,
        }, nil
    }
    
    hasUpdate := buildNumber < latestVersion.BuildNumber
    isMandatory := latestVersion.IsMandatory || (hasUpdate && latestVersion.MinSDKVersion > 0)
    
    response := &models.VersionCheckResponse{
        HasUpdate:     hasUpdate,
        IsMandatory:   isMandatory,
    }
    
    if hasUpdate {
        response.LatestVersion = latestVersion
        if isMandatory {
            response.Message = "Доступно обязательное обновление"
        } else {
            response.Message = "Доступно новое обновление"
        }
    } else {
        response.Message = "У вас установлена последняя версия"
    }
    
    return response, nil
}
// GetAllVersions получает все версии для платформы
func (r *AppVersionRepository) GetAllVersions(platform string) ([]models.AppVersion, error) {
    query := `
        SELECT id, platform, version, build_number, release_notes, download_url, 
               min_sdk_version, is_mandatory, is_active, created_at, updated_at
        FROM app_versions 
        WHERE platform = $1 
        ORDER BY build_number DESC
    `
    
    rows, err := r.DB.Query(query, platform)
    if err != nil {
        return nil, fmt.Errorf("failed to query versions: %w", err)
    }
    defer rows.Close()
    
    var versions []models.AppVersion
    for rows.Next() {
        var version models.AppVersion
        err := rows.Scan(
            &version.ID,
            &version.Platform,
            &version.Version,
            &version.BuildNumber,
            &version.ReleaseNotes,
            &version.DownloadURL,
            &version.MinSDKVersion,
            &version.IsMandatory,
            &version.IsActive,
            &version.CreatedAt,
            &version.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan version: %w", err)
        }
        versions = append(versions, version)
    }
    
    return versions, nil
}

// CreateVersion создает новую версию
func (r *AppVersionRepository) CreateVersion(version *models.AppVersion) error {
    query := `
        INSERT INTO app_versions 
        (platform, version, build_number, release_notes, download_url, min_sdk_version, is_mandatory, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
        RETURNING id
    `
    
    // Используем QueryRow для получения id сразу
    err := r.DB.QueryRow(
        query,
        version.Platform,
        version.Version,
        version.BuildNumber,
        version.ReleaseNotes,
        version.DownloadURL,
        version.MinSDKVersion,
        version.IsMandatory,
        version.IsActive,
    ).Scan(&version.ID)
    
    if err != nil {
        return fmt.Errorf("failed to create version: %w", err)
    }
    
    return nil
}

// UpdateVersion обновляет существующую версию
func (r *AppVersionRepository) UpdateVersion(version *models.AppVersion) error {
    query := `
        UPDATE app_versions 
        SET platform = $1, version = $2, build_number = $3, release_notes = $4, 
            download_url = $5, min_sdk_version = $6, is_mandatory = $7, is_active = $8, updated_at = NOW()
        WHERE id = $9
    `
    
    _, err := r.DB.Exec(
        query,
        version.Platform,
        version.Version,
        version.BuildNumber,
        version.ReleaseNotes,
        version.DownloadURL,
        version.MinSDKVersion,
        version.IsMandatory,
        version.IsActive,
        version.ID,
    )
    if err != nil {
        return fmt.Errorf("failed to update version: %w", err)
    }
    
    return nil
}

// DeleteVersion удаляет версию
func (r *AppVersionRepository) DeleteVersion(id int) error {
    query := `DELETE FROM app_versions WHERE id = $1`
    _, err := r.DB.Exec(query, id)
    if err != nil {
        return fmt.Errorf("failed to delete version: %w", err)
    }
    return nil
}   // repositories/position_repository.go

package repositories

import (
	"context"
	"database/sql"
	"time"

	"github.com/evn/eom_backendl/models"
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
// services/client.go
package services

import "github.com/gorilla/websocket"

type Client struct {
    Conn   *websocket.Conn
    Send   chan []byte
    UserID int
}
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
// services/jwt.go
package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type JWTService struct {
	secretKey   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	redisClient *redis.Client
}

func NewJWTService(secretKey string, redisClient *redis.Client) *JWTService {
	return &JWTService{
		secretKey:   []byte(secretKey),
		accessTTL:   120 * time.Minute,
		refreshTTL:  7 * 24 * time.Hour,
		redisClient: redisClient,
	}
}

func (s *JWTService) GenerateToken(userID int, username, role string) (string, string, error) {
	// Генерируем jti для refresh токена
	refreshJTI, err := s.generateJTI()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate jti: %v", err)
	}

	// Access Token
	accessClaims := jwt.MapClaims{
		"user_id":  strconv.Itoa(userID),
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(s.accessTTL).Unix(),
		"iat":      time.Now().Unix(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %v", err)
	}

	// Refresh Token
	refreshClaims := jwt.MapClaims{
		"user_id": strconv.Itoa(userID),
		"jti":     refreshJTI,
		"exp":     time.Now().Add(s.refreshTTL).Unix(),
		"iat":     time.Now().Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}

	// Сохраняем в Redis: ключ = "refresh:<jti>", значение = user_id
	ctx := context.Background()
	err = s.redisClient.Set(ctx, "refresh:"+refreshJTI, userID, s.refreshTTL).Err()
	if err != nil {
		return "", "", fmt.Errorf("failed to store refresh token in Redis: %v", err)
	}

	return accessTokenString, refreshTokenString, nil
}

func (s *JWTService) ValidateToken(tokenString string) (map[string]interface{}, error) {
	return s.parseToken(tokenString)
}

func (s *JWTService) ValidateRefreshToken(tokenString string) (int, error) {
	// Сначала парсим без проверки подписи, чтобы получить jti
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return 0, fmt.Errorf("invalid token format")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("invalid claims")
	}

	jti, ok := claims["jti"].(string)
	if !ok {
		return 0, fmt.Errorf("missing jti in refresh token")
	}

	// Проверяем, что jti есть в Redis
	val, err := s.redisClient.Get(context.Background(), "refresh:"+jti).Result()
	if err == redis.Nil {
		return 0, fmt.Errorf("refresh token not found or revoked")
	} else if err != nil {
		return 0, fmt.Errorf("redis error: %v", err)
	}

	userID, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id in redis")
	}

	// Проверяем сам токен: подпись и срок действия
	_, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secretKey, nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid or expired refresh token: %v", err)
	}

	return userID, nil
}

func (s *JWTService) GenerateAccessToken(userID int, username, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  strconv.Itoa(userID),
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(s.accessTTL).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

func (s *JWTService) RevokeRefreshToken(tokenString string) error {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}
	jti, ok := claims["jti"].(string)
	if !ok {
		return nil
	}
	return s.redisClient.Del(context.Background(), "refresh:"+jti).Err()
}

func (s *JWTService) generateJTI() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// parseToken — внутренний метод парсинга JWT
func (s *JWTService) parseToken(tokenString string) (map[string]interface{}, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return claims, nil
}
package services

import (
	"crypto/rand"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"log"
)

type TelegramAuthService struct {
	BotToken string
}

func NewTelegramAuthService(botToken string) *TelegramAuthService {
	return &TelegramAuthService{BotToken: botToken}
}

func (s *TelegramAuthService) ValidateAndExtract(data map[string]string) (map[string]string, error) {
	log.Printf("Received Telegram data: %+v", data)
	
	requiredFields := []string{"id", "hash"}
	for _, field := range requiredFields {
		if value, exists := data[field]; !exists || value == "" {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	if authDateStr, exists := data["auth_date"]; exists && authDateStr != "" {
		authDate, err := strconv.ParseInt(authDateStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid auth_date format: %v", err)
		}
		
		if time.Now().Unix()-authDate > 86400 {
			return nil, fmt.Errorf("data expired (older than 24 hours)")
		}
	}

	if !s.validateHash(data) {
		return nil, fmt.Errorf("hash validation failed")
	}

	return data, nil
}

func (s *TelegramAuthService) validateHash(data map[string]string) bool {
	hash, exists := data["hash"]
	if !exists || hash == "" {
		log.Printf("Hash not found in data")
		return false
	}

	dataForCheck := make(map[string]string)
	for k, v := range data {
		if k != "hash" && v != "" {
			dataForCheck[k] = v
		}
	}

	keys := make([]string, 0, len(dataForCheck))
	for k := range dataForCheck {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, dataForCheck[k]))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

	log.Printf("Data check string: %q", dataCheckString)

	secretKey := sha256.Sum256([]byte(s.BotToken))
	h := hmac.New(sha256.New, secretKey[:])
	h.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(h.Sum(nil))

	log.Printf("Calculated hash: %s", calculatedHash)
	log.Printf("Received hash: %s", hash)

	return calculatedHash == hash
}

func (s *TelegramAuthService) ValidateWebAppData(initData string) (map[string]string, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse init data: %v", err)
	}

	data := make(map[string]string)
	for k, v := range values {
		if len(v) > 0 {
			data[k] = v[0]
		}
	}

	return s.ValidateAndExtract(data)
}
