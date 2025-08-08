package config

import (
    "os"
)

// Config хранит все конфигурации приложения
type Config struct {
    DatabaseDSN string
    JwtSecret   string
    ServerPort  string
    TelegramBotToken string // Новый параметр
}

// NewConfig создает и возвращает новый экземпляр Config
func NewConfig() *Config {
    // Получение переменных окружения.
    // Рекомендуется использовать переменные окружения для секретных данных.
    dsn := os.Getenv("DATABASE_DSN")
    if dsn == "" {
        dsn = "./data.db"
    }

    jwtSecret := os.Getenv("JWT_SECRET")
    if jwtSecret == "" {
        jwtSecret = "0hn/a5hwoWLn4nrmogQo+zDCM7h9203J4Iwhkp7b2ns=" // Измените в продакшене!
    }

    port := os.Getenv("SERVER_PORT")
    if port == "" {
        port = "6066"
    }
    
    telegramBotToken := os.Getenv("TELEGRAM_BOT_TOKEN")
    if telegramBotToken == "" {
        // Укажите токен вашего бота
        telegramBotToken = "8213575254:AAEhzM_f_LJ-RRdaME2YAiA7tqtzWjaS-Wk" 
    }

    return &Config{
        DatabaseDSN: dsn,
        JwtSecret:   jwtSecret,
        ServerPort:  port,
        TelegramBotToken: telegramBotToken,
    }
}
