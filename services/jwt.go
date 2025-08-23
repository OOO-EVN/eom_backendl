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
    refreshStorage map[string]time.Time 
}

func NewJWTService(secretKey string) *JWTService {
    return &JWTService{
        secretKey:      []byte(secretKey),
        accessTTL:      120 * time.Minute,
        refreshTTL:     7 * 24 * time.Hour,
        refreshStorage: make(map[string]time.Time),
    }
}

// GenerateToken генерирует пару access и refresh токенов
func (s *JWTService) GenerateToken(userID int, username, role string) (string, string, error) {
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

    s.refreshStorage[refreshTokenString] = time.Now().Add(s.refreshTTL)
    return accessTokenString, refreshTokenString, nil
}

// ValidateToken проверяет access-токен и возвращает claims
func (s *JWTService) ValidateToken(tokenString string) (map[string]interface{}, error) {
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

// ValidateRefreshToken проверяет refresh-токен и возвращает userID
func (s *JWTService) ValidateRefreshToken(tokenString string) (int, error) {
    expirationTime, exists := s.refreshStorage[tokenString]
    if !exists {
        return 0, fmt.Errorf("invalid refresh token")
    }

    if time.Now().After(expirationTime) {
        delete(s.refreshStorage, tokenString)
        return 0, fmt.Errorf("refresh token expired")
    }

    claims := jwt.MapClaims{}
    token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return s.secretKey, nil
    })

    if err != nil || !token.Valid {
        delete(s.refreshStorage, tokenString)
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

// GenerateAccessToken генерирует новый access_token по данным пользователя
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

func (s *JWTService) RevokeRefreshToken(tokenString string) {
    delete(s.refreshStorage, tokenString)
}
