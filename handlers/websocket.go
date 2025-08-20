// handlers/websocket.go
package handlers

import (
    "log"
    "net/http"

    "github.com/evn/eom_backendl/services"
    "github.com/gorilla/websocket"
    "github.com/evn/eom_backendl/config"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

func WebSocketHandler(manager *services.WebSocketManager) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.URL.Query().Get("token")
        if token == "" {
            http.Error(w, "token required", http.StatusUnauthorized)
            return
        }

        userID := 0
        if ctxUserID, ok := r.Context().Value(config.UserIDKey).(int); ok {
            userID = ctxUserID
        }
        if userID == 0 {
            http.Error(w, "invalid user", http.StatusUnauthorized)
            return
        }

        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            log.Print("Upgrade error:", err)
            return
        }

        client := &services.Client{
            Conn:   conn,
            Send:   make(chan []byte, 256),
            UserID: userID,
        }

        manager.Register(client)

        // ✅ Теперь можно вызывать
        go manager.ReadPump(client)
        go manager.WritePump(client)
    }
}
