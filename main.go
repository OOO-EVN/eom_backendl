package main

import (
    "fmt"
    "log"
    "net/http"

    "eom_backend/config"
    "eom_backend/db"
    "eom_backend/handlers"
    "eom_backend/services"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func main() {
    // 1. Загрузка конфигурации
    cfg := config.NewConfig()

    // 2. Инициализация базы данных
    database := db.InitDB(cfg.DatabaseDSN)
    defer database.Close()

    // 3. Инициализация сервисов
    jwtService := services.NewJWTService(cfg.JwtSecret)
    telegramAuthService := services.NewTelegramAuthService(cfg.TelegramBotToken)

    // 4. Инициализация обработчиков
    authHandler := handlers.NewAuthHandler(database, jwtService, telegramAuthService)

    // 5. Настройка маршрутизатора
    router := chi.NewRouter()
    router.Use(middleware.Logger)
    router.Use(middleware.Recoverer)

    // Public маршруты
    router.Post("/api/auth/register", authHandler.RegisterHandler)
    router.Post("/api/auth/login", authHandler.LoginHandler)
    router.Post("/api/auth/telegram", authHandler.TelegramAuthHandler)

    // 6. Запуск сервера
    log.Printf("Сервер запущен на порту :%s", cfg.ServerPort)
    log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", cfg.ServerPort), router))
}