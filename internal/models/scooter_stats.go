// models/scooter_stats.go
package models

import (
	"net/http"
	"time"
)

// ScooterStat представляет статистику по одному пользователю
type ScooterStat struct {
	Username string         `json:"username"`
	FullName string         `json:"full_name"`
	Services map[string]int `json:"services"`
	Total    int            `json:"total"`
}

// ShiftStats представляет полную статистику за смену
type ShiftStats struct {
	ShiftName string                  `json:"shift_name"`
	StartTime time.Time               `json:"start_time"`
	EndTime   time.Time               `json:"end_time"`
	Stats     map[string]*ScooterStat `json:"stats"`     // ключ - user_id как строка
	Totals    map[string]int          `json:"totals"`    // Итоги по сервисам
	TotalAll  int                     `json:"total_all"` // Общий итог
}

func (s ShiftStats) RespondWithJSON(w http.ResponseWriter, k int, response ShiftStats) {
	panic("unimplemented")
}
