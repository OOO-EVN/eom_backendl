// handlers/scooter_stats_handler.go
package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	// Замените "github.com/evn/eom_backendl" на имя вашего модуля из go.mod, если оно другое
	"github.com/evn/eom_backendl/models"

	_ "github.com/mattn/go-sqlite3"
)

// ScooterStatsHandler структура для обработчика статистики
type ScooterStatsHandler struct {
	botDBPath string
}

// NewScooterStatsHandler создает новый экземпляр обработчика
func NewScooterStatsHandler(botDBPath string) *ScooterStatsHandler {
	return &ScooterStatsHandler{
		botDBPath: botDBPath,
	}
}

// GetShiftStatsHandler обрабатывает запрос /api/scooter-stats/shift
func (h *ScooterStatsHandler) GetShiftStatsHandler(w http.ResponseWriter, r *http.Request) {
	// Открываем соединение с SQLite базой бота
	botDB, err := sql.Open("sqlite3", h.botDBPath)
	if err != nil {
		log.Printf("Error opening scooter DB: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to connect to scooter database")
		return
	}
	defer func() {
		if closeErr := botDB.Close(); closeErr != nil {
			log.Printf("Error closing scooter DB: %v", closeErr)
		}
	}()

	// Проверка соединения
	if err := botDB.Ping(); err != nil {
		log.Printf("Error pinging scooter DB: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to ping scooter database")
		return
	}

	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		log.Printf("Error loading timezone: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	now := time.Now().In(loc)
	startTime, endTime, shiftName := getShiftTimeRange(now, loc)

	// Запрос к базе данных бота
	query := `
		SELECT 
			service, 
			accepted_by_user_id, 
			accepted_by_username, 
			accepted_by_fullname 
		FROM accepted_scooters 
		WHERE timestamp BETWEEN $1 AND $2
	`
	rows, err := botDB.Query(query, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Printf("Error querying scooter database: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	// Структуры для сбора данных
	userStats := make(map[string]*models.ScooterStat) // map[user_id_string]*ScooterStat
	serviceTotals := make(map[string]int)
	totalAll := 0

	for rows.Next() {
		var service string
		var userID int64
		var username, fullName sql.NullString

		err := rows.Scan(&service, &userID, &username, &fullName)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		userIDStr := fmt.Sprintf("%d", userID)

		// Инициализируем статистику пользователя, если нужно
		if _, exists := userStats[userIDStr]; !exists {
			userStats[userIDStr] = &models.ScooterStat{
				Username: username.String,
				FullName: fullName.String,
				Services: make(map[string]int),
				Total:    0,
			}
		}

		// Обновляем данные пользователя
		userStats[userIDStr].Services[service]++
		userStats[userIDStr].Total++

		// Обновляем общие итоги
		serviceTotals[service]++
		totalAll++
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	response := models.ShiftStats{
		ShiftName: shiftName,
		StartTime: startTime,
		EndTime:   endTime,
		Stats:     userStats,
		Totals:    serviceTotals,
		TotalAll:  totalAll,
	}

	RespondWithJSON(w, http.StatusOK, response)
}

// getShiftTimeRange определяет текущую смену
func getShiftTimeRange(now time.Time, loc *time.Location) (time.Time, time.Time, string) {
	today := now.Truncate(24 * time.Hour)

	morningShiftStart := time.Date(today.Year(), today.Month(), today.Day(), 7, 0, 0, 0, loc)
	morningShiftEnd := time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, loc)
	eveningShiftStart := time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, loc)
	eveningShiftEnd := time.Date(today.Year(), today.Month(), today.Day()+1, 4, 0, 0, 0, loc) // +1 день

	if (now.After(morningShiftStart) || now.Equal(morningShiftStart)) && now.Before(morningShiftEnd) {
		return morningShiftStart, morningShiftEnd, "утреннюю смену"
	} else if (now.After(eveningShiftStart) || now.Equal(eveningShiftStart)) && now.Before(eveningShiftEnd) {
		return eveningShiftStart, eveningShiftEnd, "вечернюю смену"
	} else {
		// Если сейчас ночь (00:00 - 07:00), считаем это концом предыдущей вечерней смены
		prevEveningStart := time.Date(today.Year(), today.Month(), today.Day()-1, 15, 0, 0, 0, loc)
		return prevEveningStart, morningShiftStart, "вечернюю смену (с учетом ночных часов)"
	}
}
