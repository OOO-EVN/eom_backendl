package routes

import (
	"database/sql"
	"net/http"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/internal/handlers"
	adminHandlers "github.com/evn/eom_backendl/internal/handlers/admin"
	authHandlers "github.com/evn/eom_backendl/internal/handlers/auth"
	geoHandlers "github.com/evn/eom_backendl/internal/handlers/geo"
	mapHandlers "github.com/evn/eom_backendl/internal/handlers/map"

	// "github.com/evn/eom_backendl/internal/handlers/promo"
	scooterHandlers "github.com/evn/eom_backendl/internal/handlers/scooter"
	shiftHandlers "github.com/evn/eom_backendl/internal/handlers/shift"

	promoHandlers "github.com/evn/eom_backendl/internal/handlers/promo"
	"github.com/evn/eom_backendl/internal/middleware" // ваш middleware
	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/evn/eom_backendl/internal/repositories"
	authService "github.com/evn/eom_backendl/internal/services/auth"
	geoService "github.com/evn/eom_backendl/internal/services/geo"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware" // ← алиас!
	"github.com/go-chi/jwtauth/v5"
	"github.com/redis/go-redis/v9"
)

// Setup инициализирует и возвращает настроенный маршрутизатор.
func Setup(cfg *config.Config, database *sql.DB, redisClient *redis.Client) *chi.Mux {
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

	// Используем chiMiddleware для Logger и Recoverer
	router.Use(chiMiddleware.Logger)
	router.Use(chiMiddleware.Recoverer)
	router.Use(jwtauth.Verifier(jwtAuth))
	router.Use(middleware.AddUserIDToContext()) // ваш middleware

	// Публичные маршруты
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

	router.Group(func(r chi.Router) {
		r.Use(jwtauth.Authenticator(jwtAuth))

		r.Post("/api/promo/upload", promoHandlers.UploadPromoCodesHandler(database))
		r.Get("/api/promo/stats", promoHandlers.GetPromoStatsHandler(database))
		r.Post("/api/promo/claim/{brand}", promoHandlers.ClaimPromoByBrandHandler(database))

		// Остальные маршруты
		r.Get("/api/profile", profileHandler.GetProfile)
		r.Post("/api/logout", authHandler.LogoutHandler)
		r.Post("/api/auth/complete-registration", authHandler.CompleteRegistrationHandler)
		r.Get("/api/admin/active-shifts", adminHandlers.GetActiveShiftsForAllHandler(database))
		r.Get("/api/admin/ended-shifts", shiftHandlers.GetEndedShiftsHandler(database))
		r.Post("/api/slot/start", shiftHandlers.StartSlotHandler(database))
		r.Post("/api/slot/end", shiftHandlers.EndSlotHandler(database))
		r.Get("/api/shifts/active", shiftHandlers.GetUserActiveShiftHandler(database))
		r.Get("/api/shifts", shiftHandlers.GetShiftsHandler(database))
		r.Get("/api/shifts/date/{date}", shiftHandlers.GetShiftsByDateHandler(database))
		r.Get("/api/users/{userID}/shifts", shiftHandlers.GetUserShiftsByIDHandler(database))
		r.Post("/api/geo", geoHandler.PostGeo)

		r.Get("/api/last", geoHandler.GetLast)
		r.Get("/api/history", geoHandler.GetHistory)
		r.Get("/api/slots/positions", shiftHandlers.GetAvailablePositionsHandler(database))
		r.Get("/api/slots/times", shiftHandlers.GetAvailableTimeSlotsHandler(database))
		r.Get("/api/slots/zones", handlers.GetAvailableZonesHandler(database))
		r.Post("/api/admin/generate-shifts", shiftHandlers.GenerateShiftsHandler(database))
		r.Get("/api/scooter-stats/shift", scooterStatsHandler.GetShiftStatsHandler)
		r.Get("/api/admin/maps", mapHandler.GetMapsHandler)
		r.Get("/api/admin/maps/{mapID}", mapHandler.GetMapByIDHandler)
		r.Get("/api/admin/maps/files/{filename}", mapHandler.ServeMapFileHandler)
		r.Post("/api/app/version/check", appVersionHandler.CheckVersionHandler)
		r.Get("/api/app/version/latest", appVersionHandler.GetLatestVersionHandler)

		// Superadmin-only
		r.Group(func(sr chi.Router) {
			sr.Use(middleware.SuperadminOnly(jwtService))
			sr.Get("/api/admin/users", adminHandlers.ListAdminUsersHandler(database))
			sr.Patch("/api/admin/users/{userID}/role", adminHandlers.UpdateUserRoleHandler(database))
			sr.Post("/api/admin/roles", adminHandlers.CreateRoleHandler(database))
			sr.Delete("/api/admin/roles", adminHandlers.DeleteRoleHandler(database))
			sr.Post("/api/admin/users", adminHandlers.CreateUserHandler(database))
			sr.Patch("/api/admin/users/{userID}/status", adminHandlers.UpdateUserStatusHandler(database))
			sr.Delete("/api/admin/users/{userID}", adminHandlers.DeleteUserHandler(database))
			sr.Post("/api/admin/users/{userID}/end-shift", adminHandlers.ForceEndShiftHandler(database))
			sr.Post("/api/admin/maps/upload", mapHandler.UploadMapHandler)
			sr.Delete("/api/admin/maps/{mapID}", mapHandler.DeleteMapHandler)
			sr.Get("/api/admin/zones", handlers.GetAvailableZonesHandler(database))
			sr.Post("/api/admin/zones", handlers.CreateZoneHandler(database))
			sr.Put("/api/admin/zones/{id}", handlers.UpdateZoneHandler(database))
			sr.Delete("/api/admin/zones/{id}", handlers.DeleteZoneHandler(database))

			r.Post("/api/admin/promo/activate-brand", promoHandlers.SetActivePromoBrandHandler(database))
			r.Delete("/api/admin/promo/activate-brand", promoHandlers.ClearActivePromoBrandHandler(database))
			r.Get("/api/admin/promo/active-brand", promoHandlers.GetActivePromoBrandHandler(database))

			sr.Get("/api/admin/app/versions", appVersionHandler.ListVersionsHandler)
			sr.Post("/api/admin/app/versions", appVersionHandler.CreateVersionHandler)
			sr.Put("/api/admin/app/versions/{id}", appVersionHandler.UpdateVersionHandler)
			sr.Delete("/api/admin/app/versions/{id}", appVersionHandler.DeleteVersionHandler)
			sr.Get("/api/admin/auto-end-shifts", handlers.AutoEndShiftsHandler(database))
		})
	})

	return router
}
