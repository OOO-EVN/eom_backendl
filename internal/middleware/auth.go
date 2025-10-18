package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evn/eom_backendl/config"
	"github.com/evn/eom_backendl/internal/pkg/response"
	authService "github.com/evn/eom_backendl/internal/services/auth"
	"github.com/go-chi/jwtauth/v5"
)

// AddUserIDToContext добавляет user_id из JWT в контекст запроса.
func AddUserIDToContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil || token == nil {
				next.ServeHTTP(w, r)
				return
			}
			claims := token.PrivateClaims()
			var userID int
			if rawID, ok := claims["user_id"]; ok {
				switch v := rawID.(type) {
				case float64:
					userID = int(v)
				case int:
					userID = v
				case string:
					if id, err := strconv.Atoi(v); err == nil {
						userID = id
					}
				}
			}
			if userID != 0 {
				ctx := context.WithValue(r.Context(), config.UserIDKey, userID)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SuperadminOnly проверяет, что роль пользователя — "superadmin".
func SuperadminOnly(jwtService *authService.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil {
				response.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}
			claims, err := token.AsMap(r.Context())
			if err != nil {
				response.RespondWithError(w, http.StatusUnauthorized, "Invalid claims")
				return
			}
			if claims["role"] != "superadmin" {
				response.RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}