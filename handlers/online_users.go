package handlers

import (
	"net/http"

	"github.com/evn/eom_backendl/services"
)

// GetOnlineUsersHandler - обработчик для получения списка онлайн пользователей
func GetOnlineUsersHandler(redisStore *services.RedisStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		onlineUsers, err := redisStore.GetOnlineUsers()
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to get online users")
			return
		}

		RespondWithJSON(w, http.StatusOK, onlineUsers)
	}
}
