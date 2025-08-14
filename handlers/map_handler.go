// handlers/map_handler.go
package handlers

import (
	"database/sql"
//	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type MapHandler struct {
	db *sql.DB
}

func NewMapHandler(db *sql.DB) *MapHandler {
	return &MapHandler{db: db}
}

type Map struct {
	ID          int    `json:"id"`
	City        string `json:"city"`
	Description string `json:"description"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	UploadDate  string `json:"upload_date"`
}

// UploadMapHandler загружает новую карту
func (h *MapHandler) UploadMapHandler(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что это multipart/form-data запрос
	if r.Method != http.MethodPost {
		RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Ограничиваем размер загружаемого файла до 40 МБ
	r.ParseMultipartForm(40 << 20)

	// Получаем город
	city := r.FormValue("city")
	if city == "" {
		RespondWithError(w, http.StatusBadRequest, "City is required")
		return
	}

	// Получаем описание (опционально)
	description := r.FormValue("description")

	// Получаем файл
	file, handler, err := r.FormFile("geojson_file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Error retrieving file")
		return
	}
	defer file.Close()

	// Проверяем расширение файла
	ext := filepath.Ext(handler.Filename)
	if ext != ".geojson" && ext != ".json" {
		RespondWithError(w, http.StatusBadRequest, "Only .geojson and .json files are allowed")
		return
	}

	// Создаем директорию для загрузки карт, если её нет
	mapDir := "./uploads/maps"
	if err := os.MkdirAll(mapDir, 0755); err != nil {
		log.Printf("Error creating map directory: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error creating directory")
		return
	}

	// Генерируем уникальное имя файла
	filename := fmt.Sprintf("%d_%s", getNextID(h.db), handler.Filename)
	filePath := filepath.Join(mapDir, filename)

	// Создаем файл на сервере
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error creating file")
		return
	}
	defer dst.Close()

	// Копируем содержимое загруженного файла в созданный файл
	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Error copying file: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error copying file")
		return
	}

	// Получаем информацию о файле
	fileInfo, err := dst.Stat()
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error getting file info")
		return
	}

	// Сохраняем информацию о карте в базу данных
	query := `
		INSERT INTO maps (city, description, file_name, file_size, upload_date)
		VALUES (?, ?, ?, ?, datetime('now'))
	`
	result, err := h.db.Exec(query, city, description, filename, fileInfo.Size())
	if err != nil {
		log.Printf("Error saving map to database: %v", err)
		// Удаляем файл в случае ошибки
		os.Remove(filePath)
		RespondWithError(w, http.StatusInternalServerError, "Error saving map to database")
		return
	}

	mapID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting last insert ID: %v", err)
		// Удаляем файл в случае ошибки
		os.Remove(filePath)
		RespondWithError(w, http.StatusInternalServerError, "Error getting map ID")
		return
	}

	// Возвращаем успешный ответ
	response := map[string]interface{}{
		"id":          mapID,
		"city":        city,
		"description": description,
		"file_name":   filename,
		"file_size":   fileInfo.Size(),
		"message":     "Map uploaded successfully",
	}
	RespondWithJSON(w, http.StatusCreated, response)
}

// GetMapsHandler возвращает список всех загруженных карт
func (h *MapHandler) GetMapsHandler(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, city, description, file_name, file_size, upload_date
		FROM maps
		ORDER BY upload_date DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var maps []Map
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.City, &m.Description, &m.FileName, &m.FileSize, &m.UploadDate); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		maps = append(maps, m)
	}

	RespondWithJSON(w, http.StatusOK, maps)
}

// GetMapByIDHandler возвращает информацию о конкретной карте
func (h *MapHandler) GetMapByIDHandler(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapID)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	var m Map
	query := `
		SELECT id, city, description, file_name, file_size, upload_date
		FROM maps
		WHERE id = ?
	`
	err = h.db.QueryRow(query, id).Scan(&m.ID, &m.City, &m.Description, &m.FileName, &m.FileSize, &m.UploadDate)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Map not found")
			return
		}
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	RespondWithJSON(w, http.StatusOK, m)
}

// DeleteMapHandler удаляет карту
func (h *MapHandler) DeleteMapHandler(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapID)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	// Получаем информацию о файле перед удалением
	var fileName string
	query := `SELECT file_name FROM maps WHERE id = ?`
	err = h.db.QueryRow(query, id).Scan(&fileName)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Map not found")
			return
		}
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Удаляем запись из базы данных
	query = `DELETE FROM maps WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		log.Printf("Database delete error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Error getting rows affected: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if rowsAffected == 0 {
		RespondWithError(w, http.StatusNotFound, "Map not found")
		return
	}

	// Удаляем файл с сервера
	filePath := filepath.Join("./uploads/maps", fileName)
	if err := os.Remove(filePath); err != nil {
		log.Printf("Error deleting file: %v", err)
		// Не возвращаем ошибку, так как запись в БД уже удалена
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Map deleted successfully"})
}

// ServeMapFileHandler отдает файл карты для скачивания
func (h *MapHandler) ServeMapFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	filePath := filepath.Join("./uploads/maps", filename)

	// Проверяем, существует ли файл
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		RespondWithError(w, http.StatusNotFound, "File not found")
		return
	}

	// Устанавливаем заголовки для GeoJSON
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))

	// Отправляем файл
	http.ServeFile(w, r, filePath)
}

// Вспомогательная функция для генерации уникального ID
func getNextID(db *sql.DB) int {
	var maxID int
	err := db.QueryRow("SELECT COALESCE(MAX(id), 0) + 1 FROM maps").Scan(&maxID)
	if err != nil {
		return 1
	}
	return maxID
}

// CreateMapsTable создает таблицу для хранения информации о картах
func CreateMapsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS maps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		city TEXT NOT NULL,
		description TEXT,
		file_name TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		upload_date DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(query)
	return err
}
