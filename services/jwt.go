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
	refreshStorage map[string]bool
}

func NewJWTService(secretKey string) *JWTService {
	return &JWTService{
		secretKey:      []byte(secretKey),
		accessTTL:      15 * time.Minute,
		refreshTTL:     7 * 24 * time.Hour,
		refreshStorage: make(map[string]bool),
	}
}

func (s *JWTService) GenerateToken(userID int, username, role string) (string, string, error) {
	// Создаём access token
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

	// Создаём refresh token
	refreshClaims := jwt.MapClaims{
		"user_id": strconv.Itoa(userID),
		"exp":     time.Now().Add(s.refreshTTL).Unix(),
		"iat":     time.Now().Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey) // Используем тот же secret
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}

	// Сохраняем refresh-токен в памяти
	s.refreshStorage[refreshTokenString] = true

	return accessTokenString, refreshTokenString, nil
}

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

func (s *JWTService) ValidateRefreshToken(tokenString string) (int, error) {
	// Проверяем, что токен активен
	if !s.refreshStorage[tokenString] {
		return 0, fmt.Errorf("invalid or revoked refresh token")
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return s.secretKey, nil
	})

	if err != nil || !token.Valid {
		return 0, fmt.Errorf("invalid or expired refresh token")
	}

	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		return 0, fmt.Errorf("user_id not found in token")
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id in token")
	}

	return userID, nil
}
