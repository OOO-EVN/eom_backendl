// handlers/map_handler.go
package handlers

import (
	"database/sql"
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
	if r.Method != http.MethodPost {
		RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Ограничиваем размер загружаемого файла до 40 МБ
	err := r.ParseMultipartForm(40 << 20)
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Request too large or invalid")
		return
	}

	city := r.FormValue("city")
	if city == "" {
		RespondWithError(w, http.StatusBadRequest, "City is required")
		return
	}

	description := r.FormValue("description")

	file, handler, err := r.FormFile("geojson_file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		RespondWithError(w, http.StatusBadRequest, "Error retrieving file")
		return
	}
	defer file.Close()

	ext := filepath.Ext(handler.Filename)
	if ext != ".geojson" && ext != ".json" {
		RespondWithError(w, http.StatusBadRequest, "Only .geojson and .json files are allowed")
		return
	}

	mapDir := "./uploads/maps"
	if err := os.MkdirAll(mapDir, 0755); err != nil {
		log.Printf("Error creating map directory: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error creating directory")
		return
	}

	// Сначала создаём запись в БД, чтобы получить уникальный ID
	var mapID int
	err = h.db.QueryRow(`
		INSERT INTO maps (city, description, file_name, file_size)
		VALUES ($1, $2, '', 0)
		RETURNING id
	`, city, description).Scan(&mapID)
	if err != nil {
		log.Printf("Error inserting map into database: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error saving map to database")
		return
	}

	// Генерируем имя файла с использованием реального ID
	filename := fmt.Sprintf("map_%d%s", mapID, ext)
	filePath := filepath.Join(mapDir, filename)

	// Создаём файл на диске
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		// Откат: удаляем запись из БД
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error creating file")
		return
	}
	defer dst.Close()

	// Копируем содержимое
	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Error copying file: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error saving file")
		return
	}

	// Получаем размер файла
	fileInfo, err := dst.Stat()
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error reading file")
		return
	}

	// Обновляем запись в БД с реальными данными файла
	_, err = h.db.Exec(`
		UPDATE maps
		SET file_name = $1, file_size = $2
		WHERE id = $3
	`, filename, fileInfo.Size(), mapID)
	if err != nil {
		log.Printf("Error updating map record: %v", err)
		dst.Close()
		os.Remove(filePath)
		h.db.Exec("DELETE FROM maps WHERE id = $1", mapID)
		RespondWithError(w, http.StatusInternalServerError, "Error finalizing map upload")
		return
	}

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
	mapIDStr := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	var m Map
	query := `
		SELECT id, city, description, file_name, file_size, upload_date
		FROM maps
		WHERE id = $1
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
	mapIDStr := chi.URLParam(r, "mapID")
	id, err := strconv.Atoi(mapIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid map ID")
		return
	}

	var fileName string
	err = h.db.QueryRow("SELECT file_name FROM maps WHERE id = $1", id).Scan(&fileName)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Map not found")
			return
		}
		log.Printf("Database query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	_, err = h.db.Exec("DELETE FROM maps WHERE id = $1", id)
	if err != nil {
		log.Printf("Database delete error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}

	filePath := filepath.Join("./uploads/maps", fileName)
	if err := os.Remove(filePath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Warning: failed to delete map file %s: %v", filePath, err)
		}
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Map deleted successfully"})
}

// ServeMapFileHandler отдает файл карты для скачивания
func (h *MapHandler) ServeMapFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	filePath := filepath.Join("./uploads/maps", filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		RespondWithError(w, http.StatusNotFound, "File not found")
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	http.ServeFile(w, r, filePath)
}

// CreateMapsTable создает таблицу для хранения информации о картах
func CreateMapsTable(db *sql.DB) error {
	query := `
    CREATE TABLE IF NOT EXISTS maps (
        id SERIAL PRIMARY KEY,
        city TEXT NOT NULL,
        description TEXT,
        file_name TEXT NOT NULL,
        file_size BIGINT NOT NULL,
        upload_date TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    );
    `
	_, err := db.Exec(query)
	return err
}
