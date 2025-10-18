package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"log"
)

type TelegramAuthService struct {
	BotToken string
}

func NewTelegramAuthService(botToken string) *TelegramAuthService {
	return &TelegramAuthService{BotToken: botToken}
}

func (s *TelegramAuthService) ValidateAndExtract(data map[string]string) (map[string]string, error) {
	log.Printf("Received Telegram data: %+v", data)
	
	requiredFields := []string{"id", "hash"}
	for _, field := range requiredFields {
		if value, exists := data[field]; !exists || value == "" {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	if authDateStr, exists := data["auth_date"]; exists && authDateStr != "" {
		authDate, err := strconv.ParseInt(authDateStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid auth_date format: %v", err)
		}
		
		if time.Now().Unix()-authDate > 86400 {
			return nil, fmt.Errorf("data expired (older than 24 hours)")
		}
	}

	if !s.validateHash(data) {
		return nil, fmt.Errorf("hash validation failed")
	}

	return data, nil
}

func (s *TelegramAuthService) validateHash(data map[string]string) bool {
	hash, exists := data["hash"]
	if !exists || hash == "" {
		log.Printf("Hash not found in data")
		return false
	}

	dataForCheck := make(map[string]string)
	for k, v := range data {
		if k != "hash" && v != "" {
			dataForCheck[k] = v
		}
	}

	keys := make([]string, 0, len(dataForCheck))
	for k := range dataForCheck {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, dataForCheck[k]))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

	log.Printf("Data check string: %q", dataCheckString)

	secretKey := sha256.Sum256([]byte(s.BotToken))
	h := hmac.New(sha256.New, secretKey[:])
	h.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(h.Sum(nil))

	log.Printf("Calculated hash: %s", calculatedHash)
	log.Printf("Received hash: %s", hash)

	return calculatedHash == hash
}

func (s *TelegramAuthService) ValidateWebAppData(initData string) (map[string]string, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse init data: %v", err)
	}

	data := make(map[string]string)
	for k, v := range values {
		if len(v) > 0 {
			data[k] = v[0]
		}
	}

	return s.ValidateAndExtract(data)
}
