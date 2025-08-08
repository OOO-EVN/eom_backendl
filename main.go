
package main


import (

    "fmt"

    "log"

    "net/http"


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


    jwtAuth := jwtauth.New("HS256", []byte(cfg.JwtSecret), nil)

    jwtService := services.NewJWTService(cfg.JwtSecret)

    telegramAuthService := services.NewTelegramAuthService(cfg.TelegramBotToken)


    authHandler := handlers.NewAuthHandler(database, jwtService, telegramAuthService)

    profileHandler := handlers.NewProfileHandler(database)

    adminHandler := handlers.NewAdminHandler(database)


    router := chi.NewRouter()

    router.Use(middleware.Logger)

    router.Use(middleware.Recoverer)

    router.Use(jwtauth.Verifier(jwtAuth))


    router.Post("/api/auth/register", authHandler.RegisterHandler)

    router.Post("/api/auth/login", authHandler.LoginHandler)

    router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)

    router.Get("/auth_callback", authHandler.TelegramAuthHandler)

    router.Group(func(r chi.Router) {

        r.Use(jwtauth.Authenticator(jwtAuth))

        r.Get("/api/profile", profileHandler.GetProfile)

        r.Post("/api/logout", authHandler.LogoutHandler)


        r.Group(func(r chi.Router) {

            r.Use(adminOnlyMiddleware(jwtService))

            r.Get("/api/admin/users", adminHandler.ListUsers)

            r.Put("/api/admin/users/role", adminHandler.UpdateUserRole)

        })

    })


    serverAddress := fmt.Sprintf(":%s", cfg.ServerPort)

    log.Printf("Server starting on %s", serverAddress)

    log.Fatal(http.ListenAndServe(serverAddress, router))

}


func adminOnlyMiddleware(jwtService *services.JWTService) func(http.Handler) http.Handler {

    return func(next http.Handler) http.Handler {

        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

            token, _, err := jwtauth.FromContext(r.Context())

            if err != nil {

                http.Error(w, "Invalid token", http.StatusUnauthorized)

                return

            }


            claims, err := token.AsMap(r.Context())

            if err != nil {

                http.Error(w, "Invalid claims", http.StatusUnauthorized)

                return

            }


            if claims["role"] != "admin" {

                http.Error(w, "Access denied", http.StatusForbidden)

                return

            }

            next.ServeHTTP(w, r)

        })

    }

}
