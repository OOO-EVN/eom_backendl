// models/location.go

package models

import "time"

type GeoUpdate struct {
	ID        int64     `json:"id,omitempty"`
	UserID    string    `json:"user_id" binding:"required"`
	Lat       float64   `json:"lat" binding:"required"`
	Lon       float64   `json:"lon" binding:"required"`
	Speed     float64   `json:"speed,omitempty"`
	Accuracy  float64   `json:"accuracy,omitempty"`
	Battery   int       `json:"battery,omitempty" binding:"min=0,max=100"`
	Event     string    `json:"event,omitempty"`
	CreatedAt time.Time `json:"ts,omitempty"`
}

// Для ответа админу
type LastLocation struct {
	UserID  string    `json:"user_id"`
	Lat     float64   `json:"lat"`
	Lon     float64   `json:"lon"`
	Battery int       `json:"battery"`
	Ts      time.Time `json:"ts"`
}
