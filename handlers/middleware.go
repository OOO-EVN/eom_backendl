package handlers

import (
	"net/http"

	"github.com/go-chi/jwtauth/v5"
)

func AdminOnlyMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, claims, err := jwtauth.FromContext(r.Context())
			
			// Проверяем наличие и валидность токена
			if err != nil || token == nil {
				RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			// Проверяем роль пользователя
			if role, ok := claims["role"].(string); !ok || role != "admin" {
				RespondWithError(w, http.StatusForbidden, "Access denied")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
