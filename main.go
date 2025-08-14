// main.go (обновленная часть)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/db"
	"github.com/evn/eom_backendl/handlers"
	"github.com/evn/eom_backendl/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
)

func main() {
	cfg := config.NewConfig()
	database := db.InitDB(cfg.DatabaseDSN)
	defer database.Close()

	// Создаем таблицу для карт, если её нет
	if err := handlers.CreateMapsTable(database); err != nil {
		log.Fatalf("Failed to create maps table: %v", err)
	}

	jwtAuth := jwtauth.New("HS256", []byte(cfg.JwtSecret), nil)
	jwtService := services.NewJWTService(cfg.JwtSecret)
	telegramAuthService := services.NewTelegramAuthService(cfg.TelegramBotToken)

	authHandler := handlers.NewAuthHandler(database, jwtService, telegramAuthService)
	profileHandler := handlers.NewProfileHandler(database)
	mapHandler := handlers.NewMapHandler(database) // Добавляем обработчик карт

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

	router.Post("/api/auth/register", authHandler.RegisterHandler)
	router.Post("/api/auth/login", authHandler.LoginHandler)
	router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)
	router.Get("/auth_callback", authHandler.TelegramAuthCallbackHandler)
	router.Get("/api/users", handlers.ListUsersHandler(database))
	router.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir("./uploads"))))
	router.Get("/api/active-slots", handlers.GetActiveShiftsHandler(database)) // Список всех активных смен

	router.Group(func(r chi.Router) {
		r.Use(jwtauth.Authenticator(jwtAuth))
		r.Get("/api/profile", profileHandler.GetProfile)
		r.Post("/api/logout", authHandler.LogoutHandler)
		r.Get("/api/admin/active-shifts", GetActiveShiftsForAll(database))
		r.Post("/api/slot/start", handlers.StartSlotHandler(database))
		r.Post("/api/slot/end", handlers.EndSlotHandler(database))
		r.Get("/api/shifts/active", handlers.GetUserActiveShiftHandler(database)) // Исправлено на GetUserActiveShiftHandler
		r.Get("/api/shifts", handlers.GetShiftsHandler(database))
		r.Get("/api/slots/positions", handlers.GetAvailablePositionsHandler(database))
		r.Get("/api/slots/times", handlers.GetAvailableTimeSlotsHandler(database))
		r.Get("/api/slots/zones", handlers.GetAvailableZonesHandler(database))
		
		// Добавляем маршруты для работы с картами
		r.Get("/api/admin/maps", mapHandler.GetMapsHandler)
		r.Get("/api/admin/maps/{mapID}", mapHandler.GetMapByIDHandler)
		r.Get("/api/admin/maps/files/{filename}", mapHandler.ServeMapFileHandler)
		
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
			
			// Добавляем маршруты для загрузки и удаления карт (только для superadmin)
			r.Post("/api/admin/maps/upload", mapHandler.UploadMapHandler)
			r.Delete("/api/admin/maps/{mapID}", mapHandler.DeleteMapHandler)
		})
	})

	if err := ensureUploadDirs(); err != nil {
		log.Fatalf("Failed to create upload directories: %v", err)
	}

	serverAddress := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("Server starting on %s", serverAddress)
	log.Fatal(http.ListenAndServe(serverAddress, router))
}

func superadminOnlyMiddleware(jwtService *services.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil {
				RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}
			claims, err := token.AsMap(r.Context())
			if err != nil {
				RespondWithError(w, http.StatusUnauthorized, "Invalid claims")
				return
			}
			if claims["role"] != "superadmin" {
				RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ensureUploadDirs() error {
	dirs := []string{
		"./uploads/selfies",
		"./uploads/maps", // Добавляем директорию для карт
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
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
			RespondWithError(w, http.StatusInternalServerError, "Database error")
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
		RespondWithJSON(w, http.StatusOK, shifts)
	}
}
