// handlers/auth.go
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

func (h *AuthHandler) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	type RequestBody struct {
		RefreshToken string `json:"refresh_token"`
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if body.RefreshToken == "" {
		RespondWithError(w, http.StatusUnauthorized, "Refresh token required")
		return
	}

	userID, err := h.jwtService.ValidateRefreshToken(body.RefreshToken)
	if err != nil {
		RespondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	var username, role string
	err = h.db.QueryRow("SELECT username, role FROM users WHERE id = $1", userID).Scan(&username, &role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "User not found")
		return
	}

	accessToken, refreshToken, err := h.jwtService.GenerateToken(userID, username, role)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Could not generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{
		"token":         accessToken,
		"refresh_token": refreshToken,
	})
}

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
	err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", regData.Username).Scan(&count)
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
		INSERT INTO users (username, first_name, password_hash, role, status, is_active)
		VALUES ($1, $2, $3, 'user', 'active', TRUE)`,
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
		Status       string
	}

	row := h.db.QueryRow(`
		SELECT id, username, password_hash, role, status
		FROM users
		WHERE LOWER(username) = LOWER($1)`,
		loginData.Username,
	)

	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status)
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

	if user.Status == "pending" && user.Role != "superadmin" {
		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"status":   user.Status,
			"message":  "Account awaiting admin approval",
			"user_id":  user.ID,
			"username": user.Username,
			"role":     user.Role,
		})
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

func (h *AuthHandler) TelegramAuthHandler(w http.ResponseWriter, r *http.Request) {
	var tgData map[string]string
	if err := json.NewDecoder(r.Body).Decode(&tgData); err != nil {
		log.Printf("Failed to decode Telegram auth request body: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Invalid request data")
		return
	}

	validatedData, err := h.telegramAuthService.ValidateAndExtract(tgData)
	if err != nil {
		log.Printf("Telegram auth validation failed: %v", err)
		RespondWithError(w, http.StatusUnauthorized, "Telegram auth failed: "+err.Error())
		return
	}

	if validatedData == nil {
		log.Println("Telegram auth validation returned nil data")
		RespondWithError(w, http.StatusUnauthorized, "Telegram auth validation returned nil data")
		return
	}

	tgIDStr := validatedData["id"]
	if tgIDStr == "" {
		log.Println("Missing 'id' in validated Telegram data")
		RespondWithError(w, http.StatusBadRequest, "Missing Telegram user ID")
		return
	}

	tgID, err := strconv.Atoi(tgIDStr)
	if err != nil {
		log.Printf("Invalid Telegram ID format: %s", tgIDStr)
		RespondWithError(w, http.StatusInternalServerError, "Invalid Telegram ID format")
		return
	}

	var user struct {
		ID         int
		Username   string
		FirstName  string
		TelegramID sql.NullInt64
		Role       string
		Status     string
	}

	// Сначала ищем по telegram_id
	err = h.db.QueryRow(`
		SELECT id, username, first_name, telegram_id, role, status
		FROM users
		WHERE telegram_id = $1`,
		tgID,
	).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role, &user.Status)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("Database error finding user by telegram_id %d: %v", tgID, err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Если не найден по telegram_id — пробуем найти или создать
	if errors.Is(err, sql.ErrNoRows) {
		tgUsername := validatedData["username"]
		if tgUsername == "" {
			tgUsername = "tg_user_" + tgIDStr
		}

		// Пытаемся найти по username
		err = h.db.QueryRow(`
			SELECT id, username, first_name, telegram_id, role, status
			FROM users
			WHERE LOWER(username) = LOWER($1)`,
			tgUsername,
		).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role, &user.Status)

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Database error finding user by username %s: %v", tgUsername, err)
			RespondWithError(w, http.StatusInternalServerError, "Database error")
			return
		}

		if errors.Is(err, sql.ErrNoRows) {
			// Создаём нового пользователя
			firstName := validatedData["first_name"]
			if firstName == "" {
				firstName = tgUsername
			}
			photoURL := validatedData["photo_url"] // ← добавлено

			err = h.db.QueryRow(`
				INSERT INTO users (telegram_id, username, first_name, role, status, is_active)
				VALUES ($1, $2, $3, 'user', 'pending', TRUE)
				RETURNING id, username, first_name`,
				tgID,
				tgUsername,
				firstName,
				photoURL, // ← добавлено

			).Scan(&user.ID, &user.Username, &user.FirstName)

			if err != nil {
				log.Printf("Failed to create new user for telegram_id %d: %v", tgID, err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create user")
				return
			}

			user.TelegramID = sql.NullInt64{Int64: int64(tgID), Valid: true}
			user.Role = "user"
			user.Status = "pending"
		} else {
			// Найден по username, но без telegram_id — обновляем
			photoURL := validatedData["photo_url"]
			_, err = h.db.Exec(`
    UPDATE users 
    SET telegram_id = $1, avatar_url = $2 
    WHERE id = $3`,
				tgID, photoURL, user.ID)
			if err != nil {
				log.Printf("Failed to update user %d with telegram_id %d: %v", user.ID, tgID, err)
				// Не критично — продолжаем
			}
			user.TelegramID = sql.NullInt64{Int64: int64(tgID), Valid: true}
		}
	} else {
		// Найден по telegram_id — обновляем first_name на всякий случай
		_, err = h.db.Exec(`
    UPDATE users 
    SET first_name = $1, avatar_url = $2 
    WHERE id = $3`,
			validatedData["first_name"],
			validatedData["photo_url"], // ← добавлено
			user.ID,
		)
		if err != nil {
			log.Printf("Failed to update first_name for user %d: %v", user.ID, err)
			// Не критично
		}
	}

	// Проверка статуса
	if user.Status == "pending" && user.Role != "superadmin" {
		RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"status":      user.Status,
			"message":     "Account awaiting admin approval",
			"user_id":     user.ID,
			"username":    user.Username,
			"first_name":  user.FirstName,
			"telegram_id": user.TelegramID.Int64,
			"role":        user.Role,
		})
		return
	}

	// Генерация токенов
	token, refreshToken, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		log.Printf("Failed to generate JWT tokens for user ID %d: %v", user.ID, err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"username":      user.Username,
		"first_name":    user.FirstName,
		"telegram_id":   user.TelegramID.Int64,
		"role":          user.Role,
		"status":        user.Status,
	})
}

func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

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
		"https://start.eom.kz/api/auth/telegram",
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
					window.location.href = "https://start.eom.kz/api/auth/telegram-success?token=%s";
				</script>
			</head>
			<body>
				<p>Авторизация прошла успешно... Перенаправление...</p>
			</body>
			</html>
			`, token)

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
		SET first_name = $1, last_name = $2, phone = $3, status = 'pending', is_active = FALSE 
		WHERE id = $4`,
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

	var updatedUser struct {
		Status   string `json:"status"`
		IsActive bool   `json:"is_active"`
	}
	err = h.db.QueryRow("SELECT status, is_active FROM users WHERE id = $1", userID).Scan(&updatedUser.Status, &updatedUser.IsActive)
	if err != nil {
		log.Printf("Database error fetching updated user %d: %v", userID, err)
		updatedUser.Status = "pending"
		updatedUser.IsActive = false
	}

	RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Registration completed. Awaiting administrator approval.",
		"status":    updatedUser.Status,
		"is_active": updatedUser.IsActive,
	})
}
