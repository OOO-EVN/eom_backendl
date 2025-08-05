package db

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "log"
)

// InitDB инициализирует соединение с базой данных и создает таблицы
func InitDB(dsn string) *sql.DB {
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        log.Fatalf("Ошибка при открытии базы данных: %v", err)
    }

    if err = db.Ping(); err != nil {
        log.Fatalf("Ошибка при подключении к базе данных: %v", err)
    }

    createTable(db)
    log.Println("База данных успешно инициализирована")
    return db
}

func createTable(db *sql.DB) {
    // Используем schema.sql для создания таблицы
    schema, err := os.ReadFile("db/schema.sql")
    if err != nil {
        log.Fatalf("Не удалось прочитать файл схемы БД: %v", err)
    }

    _, err = db.Exec(string(schema))
    if err != nil {
        log.Fatalf("Не удалось создать таблицы: %v", err)
    }
}