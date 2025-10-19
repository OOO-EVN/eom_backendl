package middleware

import "context"

type contextKey string

const UserIDContextKey contextKey = "user_id"

// GetUserIDFromContext возвращает user_id из контекста, если он установлен.
func GetUserIDFromContext(ctx context.Context) (int, bool) {
	if val := ctx.Value(UserIDContextKey); val != nil {
		if id, ok := val.(int); ok {
			return id, true
		}
	}
	return 0, false
}