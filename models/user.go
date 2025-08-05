package models

// User представляет модель пользователя
type User struct {
    ID           int    `json:"id"`
    Username     string `json:"username"`
    PasswordHash string `json:"-"` // Скрываем хэш пароля при сериализации
    FirstName    string `json:"first_name,omitempty"`
    TelegramID   string `json:"telegram_id,omitempty"`
}