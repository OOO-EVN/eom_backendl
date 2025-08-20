package models

import "time"

type Location struct {
    UserID    int       `json:"user_id"`
    Username  string    `json:"username"`
    Lat       float64   `json:"lat"`
    Lng       float64   `json:"lng"`
    Timestamp time.Time `json:"timestamp"`
}
