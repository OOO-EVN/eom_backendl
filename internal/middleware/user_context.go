// internal/middleware/user_context.go
package middleware

import (
	"context"
	// "net/http"
	"strconv"

	"github.com/go-chi/jwtauth/v5"
	"net/http"

	"github.com/evn/eom_backendl/internal/pkg/response"
	authService "github.com/evn/eom_backendl/internal/services/auth"
	// "github.com/go-chi/jwtauth/v5"
)

// UserIDContextKey — ключ для хранения user ID в контексте.
// Экспортируемый (заглавная буква), чтобы был доступен в других пакетах.
type contextKey string

const UserIDContextKey contextKey = "user_id"

// GetUserIDFromContext возвращает user_id из контекста.
func GetUserIDFromContext(ctx context.Context) (int, bool) {
	if val := ctx.Value(UserIDContextKey); val != nil {
		if id, ok := val.(int); ok {
			return id, true
		}
	}
	return 0, false
}

// AddUserIDToContext извлекает user_id из JWT и кладёт в контекст.
func AddUserIDToContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, claims, _ := jwtauth.FromContext(r.Context())
			if claims == nil {
				next.ServeHTTP(w, r)
				return
			}

			var userID int
			if id, ok := claims["user_id"].(float64); ok {
				userID = int(id)
			} else if idStr, ok := claims["user_id"].(string); ok {
				if id, err := strconv.Atoi(idStr); err == nil {
					userID = id
				}
			}

			if userID != 0 {
				ctx := context.WithValue(r.Context(), UserIDContextKey, userID)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}
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

			role, ok := claims["role"].(string)
			if !ok {
				response.RespondWithError(w, http.StatusForbidden, "Role not found")
				return
			}

			switch role {
			case "supervisor", "coordinator", "superadmin":
				// Всё ок, разрешено
			default:
				response.RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
