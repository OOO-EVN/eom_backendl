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
}