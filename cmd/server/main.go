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
	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/evn/eom_backendl/internal/repositories"

	authService "github.com/evn/eom_backendl/internal/services/auth"
	geoService "github.com/evn/eom_backendl/internal/services/geo"

	// Подпакеты handlers с алиасами
	adminHandlers "github.com/evn/eom_backendl/internal/handlers/admin"
	authHandlers "github.com/evn/eom_backendl/internal/handlers/auth"
	geoHandlers "github.com/evn/eom_backendl/internal/handlers/geo"
	mapHandlers "github.com/evn/eom_backendl/internal/handlers/map"
	scooterHandlers "github.com/evn/eom_backendl/internal/handlers/scooter"
	shiftHandlers "github.com/evn/eom_backendl/internal/handlers/shift"

	// Общие хендлеры из корня internal/handlers
	"github.com/evn/eom_backendl/internal/handlers"

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
	jwtService := authService.NewJWTService(cfg.JwtSecret, redisClient)
	telegramAuthService := authService.NewTelegramAuthService(cfg.TelegramBotToken)

	posRepo := repositories.NewPositionRepository(database)
	geoSvc := geoService.NewGeoTrackService(posRepo, redisClient)
	geoHandler := geoHandlers.NewGeoTrackHandler(geoSvc)

	authHandler := authHandlers.NewAuthHandler(database, jwtService, telegramAuthService)
	profileHandler := authHandlers.NewProfileHandler(database)
	mapHandler := mapHandlers.NewMapHandler(database)
	scooterStatsHandler := scooterHandlers.NewScooterStatsHandler("/root/tg_bot/Sharing/scooters.db")
	appVersionHandler := handlers.NewAppVersionHandler(database)

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(jwtauth.Verifier(jwtAuth))

	// Middleware для добавления userID в контекст
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

	// Public routes
	router.Post("/api/geo", geoHandler.PostGeo)
	router.Post("/api/auth/register", authHandler.RegisterHandler)
	router.Post("/api/auth/login", authHandler.LoginHandler)
	router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)
	router.Get("/auth_callback", authHandler.TelegramAuthCallbackHandler)
	router.Get("/api/time-slots/available-for-start", shiftHandlers.GetAvailableTimeSlotsForStartHandler(database))
	router.Get("/api/users", handlers.ListUsersHandler(database))
	router.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir("./uploads"))))
	router.Get("/api/active-slots", shiftHandlers.GetActiveShiftsHandler(database))
	router.Post("/api/auth/refresh", authHandler.RefreshTokenHandler)
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Authenticated routes
	router.Group(func(r chi.Router) {
		r.Use(jwtauth.Authenticator(jwtAuth))

		r.Get("/api/profile", profileHandler.GetProfile)
		r.Post("/api/logout", authHandler.LogoutHandler)
		r.Post("/api/auth/complete-registration", authHandler.CompleteRegistrationHandler)
		r.Get("/api/admin/active-shifts", GetActiveShiftsForAll(database))
		r.Get("/api/admin/ended-shifts", shiftHandlers.GetEndedShiftsHandler(database))
		r.Post("/api/slot/start", shiftHandlers.StartSlotHandler(database))
		r.Post("/api/slot/end", shiftHandlers.EndSlotHandler(database))
		r.Get("/api/shifts/active", shiftHandlers.GetUserActiveShiftHandler(database))
		r.Get("/api/shifts", shiftHandlers.GetShiftsHandler(database))
		r.Get("/api/shifts/date/{date}", shiftHandlers.GetShiftsByDateHandler(database))
		r.Get("/api/users/{userID}/shifts", shiftHandlers.GetUserShiftsByIDHandler(database))
		r.Get("/api/last", geoHandler.GetLast)

		r.Get("/api/slots/positions", shiftHandlers.GetAvailablePositionsHandler(database))
		r.Get("/api/slots/times", shiftHandlers.GetAvailableTimeSlotsHandler(database))
		r.Get("/api/slots/zones", handlers.GetAvailableZonesHandler(database)) // zone_handler.go в корне
		r.Post("/api/admin/generate-shifts", shiftHandlers.GenerateShiftsHandler(database))

		r.Get("/api/scooter-stats/shift", scooterStatsHandler.GetShiftStatsHandler)
		r.Get("/api/admin/maps", mapHandler.GetMapsHandler)
		r.Get("/api/admin/maps/{mapID}", mapHandler.GetMapByIDHandler)
		r.Get("/api/admin/maps/files/{filename}", mapHandler.ServeMapFileHandler)
		r.Post("/api/app/version/check", appVersionHandler.CheckVersionHandler)
		r.Get("/api/app/version/latest", appVersionHandler.GetLatestVersionHandler)

		// Superadmin routes
		r.Group(func(r chi.Router) {
			r.Use(superadminOnlyMiddleware(jwtService))

			r.Get("/api/admin/users", adminHandlers.ListAdminUsersHandler(database))
			r.Patch("/api/admin/users/{userID}/role", adminHandlers.UpdateUserRoleHandler(database))
			r.Post("/api/admin/roles", adminHandlers.CreateRoleHandler(database))
			r.Delete("/api/admin/roles", adminHandlers.DeleteRoleHandler(database))
			r.Post("/api/admin/users", adminHandlers.CreateUserHandler(database))
			r.Patch("/api/admin/users/{userID}/status", adminHandlers.UpdateUserStatusHandler(database))
			r.Delete("/api/admin/users/{userID}", adminHandlers.DeleteUserHandler(database))
			r.Post("/api/admin/users/{userID}/end-shift", adminHandlers.ForceEndShiftHandler(database))
			r.Post("/api/admin/maps/upload", mapHandler.UploadMapHandler)
			r.Delete("/api/admin/maps/{mapID}", mapHandler.DeleteMapHandler)
			r.Get("/api/admin/zones", handlers.GetAvailableZonesHandler(database)) // zone_handler.go в корне
			r.Post("/api/admin/zones", handlers.CreateZoneHandler(database))
			r.Put("/api/admin/zones/{id}", handlers.UpdateZoneHandler(database))
			r.Delete("/api/admin/zones/{id}", handlers.DeleteZoneHandler(database))
			r.Get("/api/admin/app/versions", appVersionHandler.ListVersionsHandler)
			r.Post("/api/admin/app/versions", appVersionHandler.CreateVersionHandler)
			r.Put("/api/admin/app/versions/{id}", appVersionHandler.UpdateVersionHandler)
			r.Delete("/api/admin/app/versions/{id}", appVersionHandler.DeleteVersionHandler)
			r.Get("/api/admin/auto-end-shifts", handlers.AutoEndShiftsHandler(database))
		})
	})

	if err := ensureUploadDirs(); err != nil {
		log.Fatalf("Failed to create upload directories: %v", err)
	}

	go autoEndShiftsLoop(database)

	serverAddress := fmt.Sprintf(":%s", cfg.ServerPort)
	log.Printf("🚀 Server starting on %s", serverAddress)
	log.Fatal(http.ListenAndServe(serverAddress, router))
}

// superadminOnlyMiddleware проверка роли супер-админа
func superadminOnlyMiddleware( *authService.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil {
				response.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}
			claims, err := token.AsMap(r.Context())
			if err != nil {
				response.RespondWithError(w, http.StatusUnauthorized, "Invalid claims")
				return
			}
			if claims["role"] != "superadmin" {
				response.RespondWithError(w, http.StatusForbidden, "Access denied")
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

func autoEndShiftsLoop(db *sql.DB) {
	log.Println("✅ Auto-end shifts job started")
	if count, err := handlers.AutoEndShifts(db); err != nil {
		log.Printf("❌ Startup failed: %v", err)
	} else {
		log.Printf("✅ Startup: ended %d slots", count)
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if count, err := handlers.AutoEndShifts(db); err != nil {
			log.Printf("❌ AutoEndShifts failed: %v", err)
		} else if count > 0 {
			log.Printf("✅ AutoEndShifts: ended %d expired slots", count)
		}
	}
}

// GetActiveShiftsForAll возвращает активные смены всех пользователей
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
			response.RespondWithError(w, http.StatusInternalServerError, "Database error")
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
		response.RespondWithJSON(w, http.StatusOK, shifts)
	}
}
