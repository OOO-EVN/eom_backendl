// services/websocket_manager.go
package services

import (
    "encoding/json"
    "log"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/evn/eom_backendl/models"
)

type WebSocketManager struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    Store      *RedisStore
    mu         sync.RWMutex
}

func NewWebSocketManager(store *RedisStore) *WebSocketManager {
    manager := &WebSocketManager{
        clients:    make(map[*Client]bool),
        broadcast:  make(chan []byte),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        Store:      store,
    }
    go manager.Run() // ✅ Запускаем экспортируемый метод
    return manager
}

func (m *WebSocketManager) Register(client *Client) {
    m.register <- client
}

func (m *WebSocketManager) Unregister(client *Client) {
    m.unregister <- client
}

func (m *WebSocketManager) Broadcast(message []byte) {
    m.broadcast <- message
}

// ✅ Один единственный Run() — экспортируемый и запускаемый
func (m *WebSocketManager) Run() {
    for {
        select {
        case client := <-m.register:
            m.mu.Lock()
            m.clients[client] = true
            m.mu.Unlock()
            m.BroadcastOnlineUsers()
        case client := <-m.unregister:
            m.mu.Lock()
            if _, ok := m.clients[client]; ok {
                delete(m.clients, client)
                close(client.Send)
            }
            m.mu.Unlock()
            m.BroadcastOnlineUsers()
        case message := <-m.broadcast:
            m.mu.RLock()
            for client := range m.clients {
                select {
                case client.Send <- message:
                default:
                    close(client.Send)
                    delete(m.clients, client)
                }
            }
            m.mu.RUnlock()
        }
    }
}

func (m *WebSocketManager) BroadcastOnlineUsers() {
    locations, err := m.Store.GetAllLocations()
    if err != nil {
        log.Printf("Failed to get locations: %v", err)
        return
    }

    data, _ := json.Marshal(map[string]interface{}{
        "type":      "online_users",
        "users":     locations,
        "timestamp": time.Now().UTC(),
    })
    m.Broadcast(data)
}

// ✅ Методы для работы с клиентом
func (m *WebSocketManager) ReadPump(client *Client) {
    defer func() {
        m.Unregister(client)
        client.Conn.Close()
    }()

    for {
        _, message, err := client.Conn.ReadMessage()
        if err != nil {
            break
        }

        var loc models.Location
        if err := json.Unmarshal(message, &loc); err != nil {
            continue
        }

        loc.UserID = client.UserID
        loc.Timestamp = time.Now().UTC()

        m.Store.SaveLocation(&loc)
        m.BroadcastOnlineUsers()
    }
}

func (m *WebSocketManager) WritePump(client *Client) {
    ticker := time.NewTicker(30 * time.Second)
    defer func() {
        ticker.Stop()
        client.Conn.Close()
    }()

    for {
        select {
        case message, ok := <-client.Send:
            if !ok {
                client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            client.Conn.WriteMessage(websocket.TextMessage, message)
        case <-ticker.C:
            client.Conn.WriteMessage(websocket.PingMessage, nil)
        }
    }
}
