package services

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
)

// JWTService предоставляет функционал для работы с JWT
type JWTService struct {
    secret string
}

// NewJWTService создает новый экземпляр JWTService
func NewJWTService(secret string) *JWTService {
    return &JWTService{secret: secret}
}

// GenerateToken создает новый JWT-токен для пользователя
func (s *JWTService) GenerateToken(userID int) (string, error) {
    claims := jwt.MapClaims{
        "user_id": userID,
        "exp":     time.Now().Add(time.Hour * 24).Unix(), // Токен истекает через 24 часа
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.secret))
}

// ValidateToken проверяет валидность JWT-токена
func (s *JWTService) ValidateToken(tokenString string) (*jwt.Token, error) {
    return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, jwt.ErrSignatureInvalid
        }
        return []byte(s.secret), nil
    })
}