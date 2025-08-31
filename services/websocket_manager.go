package services

import (
    "database/sql"
    "encoding/json"
    "log"
    "sync"
    "time"

    "github.com/evn/eom_backendl/models"
    "github.com/gorilla/websocket"
)

type WebSocketManager struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    Store      *RedisStore
    db         *sql.DB
    mu         sync.RWMutex
}

func NewWebSocketManager(store *RedisStore, db *sql.DB) *WebSocketManager {
    manager := &WebSocketManager{
        clients:    make(map[*Client]bool),
        broadcast:  make(chan []byte),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        Store:      store,
        db:         db,
    }
    go manager.Run()
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

func (m *WebSocketManager) BroadcastActiveShifts() {
    activeShifts, err := m.Store.GetAllActiveShifts(m.db)
    if err != nil {
        log.Printf("Failed to get active shifts: %v", err)
        return
    }
    data, _ := json.Marshal(map[string]interface{}{
        "type":      "active_shifts",
        "shifts":    activeShifts,
        "timestamp": time.Now().UTC(),
    })
    m.Broadcast(data)
}

func (m *WebSocketManager) Run() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case client := <-m.register:
            m.mu.Lock()
            m.clients[client] = true
            m.mu.Unlock()
            m.BroadcastActiveShifts()
        case client := <-m.unregister:
            m.mu.Lock()
            if _, ok := m.clients[client]; ok {
                delete(m.clients, client)
                close(client.Send)
                m.Store.DeleteLocation(client.UserID)
            }
            m.mu.Unlock()
            m.BroadcastActiveShifts()
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
        case <-ticker.C:
            m.BroadcastActiveShifts()
        }
    }
}

func (m *WebSocketManager) ReadPump(client *Client) {
    defer func() {
        m.Unregister(client)
        client.Conn.Close()
    }()
    for {
        _, message, err := client.Conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("error: %v", err)
            }
            break
        }
        var loc models.Location
        if err := json.Unmarshal(message, &loc); err != nil {
            log.Printf("Error unmarshaling location: %v", err)
            continue
        }
        loc.UserID = client.UserID
        loc.Timestamp = time.Now().UTC()
        if err := m.Store.SaveLocation(&loc); err != nil {
            log.Printf("Error saving location: %v", err)
            continue
        }
        m.BroadcastActiveShifts()
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
            if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
                log.Printf("Error writing message: %v", err)
                return
            }
        case <-ticker.C:
            if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
