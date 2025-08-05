package services

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/url"
    "sort"
    "strings"
)

// TelegramAuthService предоставляет функционал для валидации данных от Telegram.
type TelegramAuthService struct {
    BotToken string
}

// NewTelegramAuthService создает новый экземпляр TelegramAuthService.
func NewTelegramAuthService(botToken string) *TelegramAuthService {
    return &TelegramAuthService{BotToken: botToken}
}

// ValidateData проверяет подлинность данных, полученных от виджета Telegram.
// Подробная инструкция по валидации: https://core.telegram.org/widgets/login
func (s *TelegramAuthService) ValidateData(params url.Values) (bool, error) {
    if params.Get("hash") == "" {
        return false, fmt.Errorf("отсутствует хэш для проверки")
    }

    dataCheckString := make([]string, 0)
    for key, value := range params {
        if key != "hash" {
            dataCheckString = append(dataCheckString, fmt.Sprintf("%s=%s", key, value[0]))
        }
    }

    sort.Strings(dataCheckString)
    dataString := strings.Join(dataCheckString, "\n")

    secretKey := sha256.Sum256([]byte(s.BotToken))
    h := hmac.New(sha256.New, secretKey[:])
    h.Write([]byte(dataString))
    
    calculatedHash := hex.EncodeToString(h.Sum(nil))

    return calculatedHash == params.Get("hash"), nil
}