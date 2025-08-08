package db

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "log"
    "os" // Добавьте os
)

// InitDB инициализирует соединение с базой данных и создает таблицы
func InitDB(dsn string) *sql.DB {
    log.Println("Попытка открыть базу данных по пути:", dsn)
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        log.Fatalf("Ошибка при открытии базы данных: %v", err)
    }

    if err = db.Ping(); err != nil {
        log.Fatalf("Ошибка при подключении к базе данных: %v", err)
    }
    log.Println("Успешное подключение к базе данных.")

    createTable(db) // Убедимся, что createTable() тоже содержит логирование
    log.Println("База данных успешно инициализирована.")
    return db
}

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
    log.Println("Таблицы успешно созданы (если не существовали).")
}
