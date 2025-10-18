// services/jwt.go
package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type JWTService struct {
	secretKey   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	redisClient *redis.Client
}

func NewJWTService(secretKey string, redisClient *redis.Client) *JWTService {
	return &JWTService{
		secretKey:   []byte(secretKey),
		accessTTL:   120 * time.Minute,
		refreshTTL:  7 * 24 * time.Hour,
		redisClient: redisClient,
	}
}

func (s *JWTService) GenerateToken(userID int, username, role string) (string, string, error) {
	// Генерируем jti для refresh токена
	refreshJTI, err := s.generateJTI()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate jti: %v", err)
	}

	// Access Token
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

	// Refresh Token
	refreshClaims := jwt.MapClaims{
		"user_id": strconv.Itoa(userID),
		"jti":     refreshJTI,
		"exp":     time.Now().Add(s.refreshTTL).Unix(),
		"iat":     time.Now().Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}

	// Сохраняем в Redis: ключ = "refresh:<jti>", значение = user_id
	ctx := context.Background()
	err = s.redisClient.Set(ctx, "refresh:"+refreshJTI, userID, s.refreshTTL).Err()
	if err != nil {
		return "", "", fmt.Errorf("failed to store refresh token in Redis: %v", err)
	}

	return accessTokenString, refreshTokenString, nil
}

func (s *JWTService) ValidateToken(tokenString string) (map[string]interface{}, error) {
	return s.parseToken(tokenString)
}

func (s *JWTService) ValidateRefreshToken(tokenString string) (int, error) {
	// Сначала парсим без проверки подписи, чтобы получить jti
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return 0, fmt.Errorf("invalid token format")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, fmt.Errorf("invalid claims")
	}

	jti, ok := claims["jti"].(string)
	if !ok {
		return 0, fmt.Errorf("missing jti in refresh token")
	}

	// Проверяем, что jti есть в Redis
	val, err := s.redisClient.Get(context.Background(), "refresh:"+jti).Result()
	if err == redis.Nil {
		return 0, fmt.Errorf("refresh token not found or revoked")
	} else if err != nil {
		return 0, fmt.Errorf("redis error: %v", err)
	}

	userID, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id in redis")
	}

	// Проверяем сам токен: подпись и срок действия
	_, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secretKey, nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid or expired refresh token: %v", err)
	}

	return userID, nil
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

func (s *JWTService) RevokeRefreshToken(tokenString string) error {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}
	jti, ok := claims["jti"].(string)
	if !ok {
		return nil
	}
	return s.redisClient.Del(context.Background(), "refresh:"+jti).Err()
}

func (s *JWTService) generateJTI() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

// parseToken — внутренний метод парсинга JWT
func (s *JWTService) parseToken(tokenString string) (map[string]interface{}, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return claims, nil
}
