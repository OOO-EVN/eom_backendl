// models/user_shift_location.go
package models

import (
    "time"
)

type UserShiftLocation struct {
    UserID      int       `json:"user_id"`
    Username    string    `json:"username"`
    Position    string    `json:"position"`
    Zone        string    `json:"zone"`
    StartTime   time.Time `json:"start_time"`
    Lat         *float64  `json:"lat,omitempty"` // Используем указатель для различия 0 и отсутствия значения
    Lng         *float64  `json:"lng,omitempty"`
    Timestamp   *time.Time `json:"timestamp,omitempty"`
    HasLocation bool      `json:"has_location"`
}
