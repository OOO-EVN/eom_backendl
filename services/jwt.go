// services/jwt.go
package services

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	secretKey      []byte
	accessTTL      time.Duration
	refreshTTL     time.Duration
	// Используем time.Time для отслеживания истечения срока действия токена
	refreshStorage map[string]time.Time 
}

func NewJWTService(secretKey string) *JWTService {
	return &JWTService{
		secretKey:      []byte(secretKey),
		// Установите желаемое время жизни
		accessTTL:      120 * time.Minute, // Для тестирования сделаем очень коротким
		refreshTTL:     7 * 24 * time.Hour,
		refreshStorage: make(map[string]time.Time),
	}
}

// GenerateToken генерирует пару access и refresh токенов
func (s *JWTService) GenerateToken(userID int, username, role string) (string, string, error) {
	// --- Access Token ---
	accessClaims := jwt.MapClaims{
		"user_id":  strconv.Itoa(userID),
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(s.accessTTL).Unix(),
		"iat":      time.Now().Unix(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %v", err)
	}

	// --- Refresh Token ---
	refreshClaims := jwt.MapClaims{
		"user_id": strconv.Itoa(userID),
		"exp":     time.Now().Add(s.refreshTTL).Unix(),
		"iat":     time.Now().Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}

	// Сохраняем refresh-токен с временем истечения
	// Это позволяет нам в будущем очищать истёкшие токены или проверять их срок
	s.refreshStorage[refreshTokenString] = time.Now().Add(s.refreshTTL)

	return accessTokenString, refreshTokenString, nil
}

// ValidateRefreshToken проверяет refresh-токен и возвращает userID
func (s *JWTService) ValidateRefreshToken(tokenString string) (int, error) {
	// 1. Проверяем, существует ли токен в нашем "хранилище"
	expirationTime, exists := s.refreshStorage[tokenString]
	if !exists {
		return 0, fmt.Errorf("invalid refresh token")
	}

	// 2. Проверяем, не истёк ли он по времени (на стороне сервера)
	if time.Now().After(expirationTime) {
		// Удаляем истёкший токен
		delete(s.refreshStorage, tokenString)
		return 0, fmt.Errorf("refresh token expired")
	}

	// 3. Парсим токен JWT для проверки его валидности и извлечения данных
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Проверяем метод подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	// 4. Проверяем результат парсинга
	if err != nil || !token.Valid {
		// Если токен невалиден, удаляем его
		delete(s.refreshStorage, tokenString)
		return 0, fmt.Errorf("invalid or expired refresh token")
	}

	// 5. Извлекаем userID из claims
	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		return 0, fmt.Errorf("user_id not found in token")
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id in token")
	}

	// Токен валиден и не истёк
	return userID, nil
}

// GenerateAccessToken генерирует новый access_token по данным пользователя
// Используется после успешной проверки refresh_token
func (s *JWTService) GenerateAccessToken(userID int, username, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  strconv.Itoa(userID),
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(s.accessTTL).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

// Optional: Метод для "logout" - отзыв refresh токена
func (s *JWTService) RevokeRefreshToken(tokenString string) {
	delete(s.refreshStorage, tokenString)
}
