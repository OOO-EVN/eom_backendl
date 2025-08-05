package handlers

import (
    "database/sql"
    "encoding/json"
    "net/http"
    "eom_backend/models"
    "eom_backend/services"
    "eom_backend/config"
    "log"

    "golang.org/x/crypto/bcrypt"
)

// AuthHandler содержит зависимости для обработчиков аутентификации
type AuthHandler struct {
    DB                 *sql.DB
    JwtService         *services.JWTService
    TelegramAuthService *services.TelegramAuthService
}

// NewAuthHandler создает новый экземпляр AuthHandler
func NewAuthHandler(db *sql.DB, jwtService *services.JWTService, tgService *services.TelegramAuthService) *AuthHandler {
    return &AuthHandler{
        DB:                 db,
        JwtService:         jwtService,
        TelegramAuthService: tgService,
    }
}

// RegisterHandler обрабатывает регистрацию нового пользователя
func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
    var user models.User
    if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
        http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
        return
    }

    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
    if err != nil {
        http.Error(w, "Ошибка при хешировании пароля", http.StatusInternalServerError)
        return
    }
    user.PasswordHash = string(hashedPassword)

    // Вставка пользователя в БД
    _, err = h.DB.Exec("INSERT INTO users (username, password_hash, first_name) VALUES (?, ?, ?)",
        user.Username, user.PasswordHash, user.FirstName)
    if err != nil {
        http.Error(w, "Ошибка при создании пользователя", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"message": "Пользователь успешно зарегистрирован"})
}

// LoginHandler обрабатывает стандартный вход по логину/паролю
func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
    var loginData struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
        http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
        return
    }

    var user models.User
    err := h.DB.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", loginData.Username).Scan(&user.ID, &user.PasswordHash)
    if err == sql.ErrNoRows {
        http.Error(w, "Неверное имя пользователя или пароль", http.StatusUnauthorized)
        return
    } else if err != nil {
        http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(loginData.Password)); err != nil {
        http.Error(w, "Неверное имя пользователя или пароль", http.StatusUnauthorized)
        return
    }

    token, err := h.JwtService.GenerateToken(user.ID)
    if err != nil {
        http.Error(w, "Не удалось создать токен", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// TelegramAuthHandler обрабатывает аутентификацию через Telegram
func (h *AuthHandler) TelegramAuthHandler(w http.ResponseWriter, r *http.Request) {
    var tgData map[string]string
    if err := json.NewDecoder(r.Body).Decode(&tgData); err != nil {
        http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
        return
    }

    // Валидация данных от Telegram
    params := url.Values{}
    for k, v := range tgData {
        params.Add(k, v)
    }
    isValid, err := h.TelegramAuthService.ValidateData(params)
    if err != nil || !isValid {
        http.Error(w, "Неверные данные Telegram: " + err.Error(), http.StatusUnauthorized)
        return
    }
    
    // Поиск пользователя по Telegram ID
    var user models.User
    err = h.DB.QueryRow("SELECT id, username FROM users WHERE telegram_id = ?", tgData["id"]).Scan(&user.ID, &user.Username)

    if err == sql.ErrNoRows {
        // Пользователь не найден, регистрируем нового
        _, err := h.DB.Exec("INSERT INTO users (telegram_id, username, first_name, password_hash) VALUES (?, ?, ?, ?)",
            tgData["id"], tgData["username"], tgData["first_name"], "") // Пароль не нужен
        if err != nil {
            http.Error(w, "Ошибка регистрации пользователя через Telegram", http.StatusInternalServerError)
            log.Printf("Telegram registration error: %v", err)
            return
        }
        // Получаем ID только что созданного пользователя
        h.DB.QueryRow("SELECT id FROM users WHERE telegram_id = ?", tgData["id"]).Scan(&user.ID)
    } else if err != nil {
        http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
        return
    }

    // Создаем JWT-токен для пользователя
    token, err := h.JwtService.GenerateToken(user.ID)
    if err != nil {
        http.Error(w, "Не удалось создать токен", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"token": token})
}