// services/client.go
package services

import "github.com/gorilla/websocket"

type Client struct {
    Conn   *websocket.Conn
    Send   chan []byte
    UserID int
}
