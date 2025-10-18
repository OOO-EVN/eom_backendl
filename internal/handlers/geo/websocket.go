// // handlers/websocket.go
package handlers

// import (
// 	"database/sql"
// 	"log"
// 	"net/http"
// 	"strconv"

// 	"github.com/evn/eom_backendl/services"
// 	"github.com/gorilla/websocket"
// )

// var upgrader = websocket.Upgrader{
// 	CheckOrigin: func(r *http.Request) bool {
// 		return true
// 	},
// }

// func WebSocketHandler(manager *services.WebSocketManager, db *sql.DB, jwtService *services.JWTService) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		log.Printf("WebSocket connection attempt from %s", r.RemoteAddr)

// 		token := r.URL.Query().Get("token")
// 		if token == "" {
// 			log.Printf("WebSocket connection rejected: no token provided")
// 			http.Error(w, "token required", http.StatusUnauthorized)
// 			return
// 		}

// 		claims, err := jwtService.ValidateToken(token)
// 		if err != nil {
// 			log.Printf("WebSocket connection rejected: invalid token - %v", err)
// 			http.Error(w, "invalid token", http.StatusUnauthorized)
// 			return
// 		}

// 		userIDStr, ok := claims["user_id"].(string)
// 		if !ok {
// 			log.Printf("WebSocket connection rejected: invalid user_id in token")
// 			http.Error(w, "invalid user_id in token", http.StatusUnauthorized)
// 			return
// 		}

// 		userID, err := strconv.Atoi(userIDStr)
// 		if err != nil {
// 			log.Printf("WebSocket connection rejected: invalid user_id format - %v", err)
// 			http.Error(w, "invalid user_id format", http.StatusUnauthorized)
// 			return
// 		}

// 		username, ok := claims["username"].(string)
// 		if !ok {
// 			err := db.QueryRow("SELECT username FROM users WHERE id = $1", userID).Scan(&username)
// 			if err != nil {
// 				log.Printf("WebSocket connection rejected: failed to get username - %v", err)
// 				http.Error(w, "failed to get user info", http.StatusInternalServerError)
// 				return
// 			}
// 		}

// 		conn, err := upgrader.Upgrade(w, r, nil)
// 		if err != nil {
// 			log.Printf("WebSocket upgrade error: %v", err)
// 			return
// 		}

// 		log.Printf("WebSocket connection established for user %s (ID: %d)", username, userID)

// 		client := &services.Client{
// 			Conn:   conn,
// 			Send:   make(chan []byte, 256),
// 			UserID: userID,
// 		}

// 		manager.Register(client)
// 		manager.BroadcastActiveShifts()
// 		go manager.ReadPump(client)
// 		go manager.WritePump(client)
// 	}
// }
