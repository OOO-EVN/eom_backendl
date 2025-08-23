package main

import (
    "context"
    "database/sql"
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
    
    // Создаем необходимые таблицы
    if err := handlers.CreateMapsTable(database); err != nil {
        log.Fatalf("Failed to create maps table: %v", err)
    }
    if err := handlers.CreateTasksTable(database); err != nil {
        log.Fatalf("Failed to create tasks table: %v", err)
    }
    
    // Создаем таблицу для версий приложения
    if err := createAppVersionsTable(database); err != nil {
        log.Fatalf("Failed to create app versions table: %v", err)
    }
    
    jwtAuth := jwtauth.New("HS256", []byte(cfg.JwtSecret), nil)
    jwtService := services.NewJWTService(cfg.JwtSecret)
    telegramAuthService := services.NewTelegramAuthService(cfg.TelegramBotToken)
    redisClient := config.NewRedisClient()
    defer redisClient.Close()
    redisStore := services.NewRedisStore(redisClient)
    wsManager := services.NewWebSocketManager(redisStore, database)
    go wsManager.Run()
    
    authHandler := handlers.NewAuthHandler(database, jwtService, telegramAuthService)
    profileHandler := handlers.NewProfileHandler(database)
    mapHandler := handlers.NewMapHandler(database)
    taskHandler := handlers.NewTaskHandler(database)
    scooterStatsHandler := handlers.NewScooterStatsHandler("/root/tg_bot/Sharing/scooters.db")
    
    // Инициализируем обработчик версий приложения
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
    
    // Публичные маршруты
    router.Post("/api/auth/register", authHandler.RegisterHandler)
    router.Post("/api/auth/login", authHandler.LoginHandler)
    router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)
    router.Get("/auth_callback", authHandler.TelegramAuthCallbackHandler)
    router.Get("/api/users", handlers.ListUsersHandler(database))
    router.Handle("/uploads/*", http.StripPrefix("/uploads", http.FileServer(http.Dir("./uploads"))))
    router.Get("/api/active-slots", handlers.GetActiveShiftsHandler(database))
    router.Post("/api/auth/refresh", authHandler.RefreshTokenHandler)
    router.Get("/ws", handlers.WebSocketHandler(wsManager, database, jwtService))
    
    // Защищённые маршруты
    router.Group(func(r chi.Router) {
        r.Use(jwtauth.Authenticator(jwtAuth))
        
        // Профиль и базовые функции
        r.Get("/api/profile", profileHandler.GetProfile)
        r.Post("/api/logout", authHandler.LogoutHandler)
        r.Post("/api/auth/complete-registration", authHandler.CompleteRegistrationHandler)
        
        // Смены
        r.Get("/api/admin/active-shifts", GetActiveShiftsForAll(database))
        r.Get("/api/admin/ended-shifts", handlers.GetEndedShiftsHandler(database))
        r.Post("/api/slot/start", handlers.StartSlotHandler(database))
        r.Post("/api/slot/end", handlers.EndSlotHandler(database))
        r.Get("/api/shifts/active", handlers.GetUserActiveShiftHandler(database))
        r.Get("/api/shifts", handlers.GetShiftsHandler(database))
        r.Get("/api/users/{userID}/shifts", handlers.GetUserShiftsByIDHandler(database))
        
        // Слоты и справочники
        r.Get("/api/slots/positions", handlers.GetAvailablePositionsHandler(database))
        r.Get("/api/slots/times", handlers.GetAvailableTimeSlotsHandler(database))
        r.Get("/api/slots/zones", handlers.GetAvailableZonesHandler(database))
        r.Post("/api/admin/generate-shifts", handlers.GenerateShiftsHandler(database))
        
        // Статистика
        r.Get("/api/scooter-stats/shift", scooterStatsHandler.GetShiftStatsHandler)
        
        // Карты
        r.Get("/api/admin/maps", mapHandler.GetMapsHandler)
        r.Get("/api/admin/maps/{mapID}", mapHandler.GetMapByIDHandler)
        r.Get("/api/admin/maps/files/{filename}", mapHandler.ServeMapFileHandler)
        
        // Задания
        r.Get("/api/admin/tasks", taskHandler.GetTasksHandler)
        r.Get("/api/admin/tasks/files/{filename}", taskHandler.ServeTaskFileHandler)
        r.Get("/api/my/tasks", taskHandler.GetMyTasksHandler)
        
        // OTA Обновления - проверка для всех пользователей
        r.Post("/api/app/version/check", appVersionHandler.CheckVersionHandler)
        r.Get("/api/app/version/latest", appVersionHandler.GetLatestVersionHandler)
        
        // Админские маршруты
        r.Group(func(r chi.Router) {
            r.Use(superadminOnlyMiddleware(jwtService))
            
            // Пользователи
            r.Get("/api/admin/users", handlers.ListAdminUsersHandler(database))
            r.Patch("/api/admin/users/{userID}/role", handlers.UpdateUserRoleHandler(database))
            r.Post("/api/admin/roles", handlers.CreateRoleHandler(database))
            r.Delete("/api/admin/roles", handlers.DeleteRoleHandler(database))
            r.Post("/api/admin/users", handlers.CreateUserHandler(database))
            r.Patch("/api/admin/users/{userID}/status", handlers.UpdateUserStatusHandler(database))
            r.Delete("/api/admin/users/{userID}", handlers.DeleteUserHandler(database))
            r.Post("/api/admin/users/{userID}/end-shift", handlers.ForceEndShiftHandler(database))
            
            // Карты
            r.Post("/api/admin/maps/upload", mapHandler.UploadMapHandler)
            r.Delete("/api/admin/maps/{mapID}", mapHandler.DeleteMapHandler)
            
            // Зоны
            r.Get("/api/admin/zones", handlers.GetAvailableZonesHandler(database))
            r.Post("/api/admin/zones", handlers.CreateZoneHandler(database))
            r.Put("/api/admin/zones/{id}", handlers.UpdateZoneHandler(database))
            r.Delete("/api/admin/zones/{id}", handlers.DeleteZoneHandler(database))
            
            // Задания
            r.Post("/api/admin/tasks", taskHandler.CreateTaskHandler)
            r.Patch("/api/admin/tasks/{taskID}/status", taskHandler.UpdateTaskStatusHandler)
            r.Delete("/api/admin/tasks/{taskID}", taskHandler.DeleteTaskHandler)
            
            // OTA Обновления - управление для админов
            r.Get("/api/admin/app/versions", appVersionHandler.ListVersionsHandler)
            r.Post("/api/admin/app/versions", appVersionHandler.CreateVersionHandler)
            r.Put("/api/admin/app/versions/{id}", appVersionHandler.UpdateVersionHandler)
            r.Delete("/api/admin/app/versions/{id}", appVersionHandler.DeleteVersionHandler)
        })
    })
    
    if err := ensureUploadDirs(); err != nil {
        log.Fatalf("Failed to create upload directories: %v", err)
    }
    
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
        "./uploads/tasks",
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

// Создание таблицы для версий приложения
func createAppVersionsTable(db *sql.DB) error {
    query := `
    CREATE TABLE IF NOT EXISTS app_versions (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        platform TEXT NOT NULL,
        version TEXT NOT NULL,
        build_number INTEGER NOT NULL,
        release_notes TEXT,
        download_url TEXT NOT NULL,
        min_sdk_version INTEGER DEFAULT 0,
        is_mandatory BOOLEAN DEFAULT FALSE,
        is_active BOOLEAN DEFAULT TRUE,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    
    CREATE INDEX IF NOT EXISTS idx_app_versions_platform_version ON app_versions(platform, version);
    CREATE INDEX IF NOT EXISTS idx_app_versions_active ON app_versions(is_active);
    CREATE INDEX IF NOT EXISTS idx_app_versions_build ON app_versions(build_number);
    
    INSERT OR IGNORE INTO app_versions (platform, version, build_number, release_notes, download_url, is_mandatory, is_active) VALUES
    ('android', '1.0.0', 100, 'Первый релиз приложения', 'https://example.com/app-release.apk', FALSE, TRUE),
    ('ios', '1.0.0', 100, 'Первый релиз приложения', 'https://apps.apple.com/app/id123456789', FALSE, TRUE);
    `
    
    _, err := db.Exec(query)
    return err
}
