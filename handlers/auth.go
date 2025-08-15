package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/services"
)

type AuthHandler struct {
	db                  *sql.DB
	jwtService          *services.JWTService
	telegramAuthService *services.TelegramAuthService
}

func NewAuthHandler(db *sql.DB, jwtService *services.JWTService, tgService *services.TelegramAuthService) *AuthHandler {
	return &AuthHandler{
		db:                  db,
		jwtService:          jwtService,
		telegramAuthService: tgService,
	}
}

// RefreshTokenHandler — обновление access_token с помощью refresh_token
// handlers/auth.go
func (h *AuthHandler) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	type RequestBody struct {
		RefreshToken string `json:"refresh_token"`
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Проверяем refresh-токен
	userID, err := h.jwtService.ValidateRefreshToken(body.RefreshToken)
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Получаем username и role
	var username, role string
	err = h.db.QueryRow("SELECT username, role FROM users WHERE id = ?", userID).Scan(&username, &role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "User not found")
		return
	}

	// Генерируем новый access_token
	accessToken, err := h.jwtService.GenerateAccessToken(userID, username, role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Could not generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{
		"access_token": accessToken,
	})
}
// RegisterHandler — регистрация нового пользователя
func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var regData struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		Password  string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&regData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", regData.Username).Scan(&count)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if count > 0 {
		RespondWithError(w, http.StatusBadRequest, "Username already exists")
		return
	}

	passwordHash, err := services.HashPassword(regData.Password)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	_, err = h.db.Exec(`
        INSERT INTO users (username, first_name, password_hash, role)
        VALUES (?, ?, ?, 'user')`,
		regData.Username,
		regData.FirstName,
		passwordHash,
	)

	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	RespondWithJSON(w, http.StatusCreated, map[string]string{
		"message": "User registered successfully",
	})
}

// LoginHandler — вход пользователя
func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var loginData struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	var user struct {
		ID           int
		Username     string
		PasswordHash string
		Role         string
	}

	row := h.db.QueryRow(`
        SELECT id, username, password_hash, role
        FROM users
        WHERE username = ? COLLATE NOCASE`,
		loginData.Username,
	)

	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		} else {
			RespondWithError(w, http.StatusInternalServerError, "Database error: "+err.Error())
		}
		return
	}

	if !services.CheckPasswordHash(loginData.Password, user.PasswordHash) {
		RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	token, refreshToken, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{
		"token":         token,
		"refresh_token": refreshToken,
		"role":          user.Role,
	})
}

// TelegramAuthHandler — аутентификация через Telegram
func (h *AuthHandler) TelegramAuthHandler(w http.ResponseWriter, r *http.Request) {
	var tgData map[string]string
	if err := json.NewDecoder(r.Body).Decode(&tgData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	validatedData, err := h.telegramAuthService.ValidateAndExtract(tgData)
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Telegram auth failed: "+err.Error())
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  string
		TelegramID int
		Role       string
	}

	tgID, _ := strconv.Atoi(validatedData["id"])

	err = h.db.QueryRow(`
        SELECT id, username, first_name, telegram_id, role
        FROM users
        WHERE telegram_id = ?`,
		tgID,
	).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
		username := validatedData["username"]
		if username == "" {
			username = "tg_user_" + validatedData["id"]
		}

		res, err := h.db.Exec(`
            INSERT INTO users (telegram_id, username, first_name, role)
            VALUES (?, ?, ?, 'user')`,
			tgID,
			username,
			validatedData["first_name"],
		)

		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())
			return
		}

		id, _ := res.LastInsertId()
		user.ID = int(id)
		user.Username = username
		user.FirstName = validatedData["first_name"]
		user.TelegramID = tgID
		user.Role = "user"
	}

	token, refreshToken, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
		"first_name":    user.FirstName,
		"telegram_id":   user.TelegramID,
		"role":          user.Role,
	})
}

// LogoutHandler — выход (в будущем можно добавить отмену refresh-токена)
func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// TelegramAuthCallbackHandler — обработка callback от Telegram
func (h *AuthHandler) TelegramAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			data[key] = values[0]
		}
	}

	if _, ok := data["id"]; !ok {
		RespondWithError(w, http.StatusBadRequest, "Missing 'id'")
		return
	}
	if _, ok := data["hash"]; !ok {
		RespondWithError(w, http.StatusBadRequest, "Missing 'hash'")
		return
	}

	log.Printf("Telegram callback received: %+v", data)

	jsonData, err := json.Marshal(data)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to encode data")
		return
	}

	resp, err := http.Post(
		"http://localhost:6066/api/auth/telegram",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Internal service error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Invalid response from auth handler")
		return
	}

	if resp.StatusCode == http.StatusOK {
		token, ok := result["token"].(string)
		if !ok {
			RespondWithError(w, http.StatusInternalServerError, "Token not found in response")
			return
		}

		html := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Auth Success</title>
            <script>
                window.location.href = "%s/api/auth/telegram-success?token=%s";
            </script>
        </head>
        <body>
            <p>Авторизация прошла успешно... Перенаправление...</p>
        </body>
        </html>
        `, AppConfigBackendURL, token)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	} else {
		errorMsg, _ := result["error"].(string)
		if errorMsg == "" {
			errorMsg = "Authorization failed"
		}
		RespondWithError(w, resp.StatusCode, errorMsg)
	}
}

// CompleteRegistrationHandler — завершение регистрации
func (h *AuthHandler) CompleteRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDVal := ctx.Value(config.UserIDKey)
	if userIDVal == nil {
		RespondWithError(w, http.StatusUnauthorized, "User ID not found in context")
		return
	}

	userID, ok := userIDVal.(int)
	if !ok {
		RespondWithError(w, http.StatusInternalServerError, "Invalid User ID type in context")
		return
	}

	var regData struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Phone     string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&regData); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request data: "+err.Error())
		return
	}

	if regData.FirstName == "" {
		RespondWithError(w, http.StatusBadRequest, "First name is required")
		return
	}

	_, err := h.db.Exec(`
		UPDATE users 
		SET first_name = ?, last_name = ?, phone = ?, status = 'pending', is_active = 0
		WHERE id = ?`,
		regData.FirstName,
		regData.LastName,
		regData.Phone,
		userID,
	)

	if err != nil {
		log.Printf("Database error updating user %d: %v", userID, err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to update user profile")
		return
	}

	log.Printf("User %d completed registration and is now pending approval", userID)
	RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Registration completed. Awaiting administrator approval.",
	})
}

const AppConfigBackendURL = "https://eom-sharing.duckdns.org"
