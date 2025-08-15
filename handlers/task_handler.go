// handlers/task_handler.go
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evn/eom_backendl/config"
	"github.com/go-chi/chi/v5"
)

// Task представляет структуру задания
type Task struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	AssigneeUsername string `json:"assignee_username"`
	CreatedBy        string `json:"created_by"`
	Priority         string `json:"priority"` // low, medium, high
	Status           string `json:"status"`   // pending, in_progress, completed, cancelled
	Deadline         string `json:"deadline,omitempty"`
	ImageURL         string `json:"image_url,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// TaskHandler — обработчик для операций с заданиями
type TaskHandler struct {
	DB *sql.DB
}

func NewTaskHandler(db *sql.DB) *TaskHandler {
	return &TaskHandler{DB: db}
}

// CreateTasksTable — создаёт таблицу tasks, если её нет
func CreateTasksTable(db *sql.DB) error {
	// Используем TEXT для дат, совместимо с SQLite
	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		assignee_username TEXT NOT NULL REFERENCES users(username),
		created_by_username TEXT NOT NULL REFERENCES users(username),
		priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'cancelled')),
		deadline TEXT, -- ISO 8601 string
		image_path TEXT,
		created_at TEXT DEFAULT (datetime('now')), -- ISO 8601 string
		updated_at TEXT DEFAULT (datetime('now'))  -- ISO 8601 string
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_username);
	CREATE INDEX IF NOT EXISTS idx_tasks_created_by ON tasks(created_by_username);
	`
	_, err := db.Exec(query)
	return err
}

// CreateTaskHandler — создание нового задания (с фото опционально или без фото)
func (h *TaskHandler) CreateTaskHandler(w http.ResponseWriter, r *http.Request) {
	// Безопасное извлечение userID из контекста
	userIDVal := r.Context().Value(config.UserIDKey)
	if userIDVal == nil {
		log.Println("CreateTaskHandler: userID not found in context")
		RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
		return
	}

	userID, ok := userIDVal.(int)
	if !ok {
		log.Printf("CreateTaskHandler: userID in context is not int: %T", userIDVal)
		RespondWithError(w, http.StatusInternalServerError, "Invalid User ID type in context")
		return
	}

	var creatorUsername string
	err := h.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&creatorUsername)
	if err != nil {
		log.Printf("CreateTaskHandler: DB error getting creator username: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to get creator information")
		return
	}

	// --- НАЧАЛО ИЗМЕНЕНИЙ ---
	var (
		title             string
		description       string
		assigneeUsername  string
		priority          string = "medium" // Значение по умолчанию
		deadlineStr       string // Используем string для дедлайна
		imagePath         string
	)

	contentType := r.Header.Get("Content-Type")

	// Определяем тип запроса
	if strings.Contains(contentType, "multipart/form-data") {
		// Обрабатываем как multipart/form-data
		log.Println("CreateTaskHandler: Processing as multipart/form-data")
		err = r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			log.Printf("CreateTaskHandler: Error parsing multipart form: %v", err)
			RespondWithError(w, http.StatusBadRequest, "Failed to parse form data")
			return
		}

		title = r.FormValue("title")
		description = r.FormValue("description")
		assigneeUsername = r.FormValue("assignee_username")
		if p := r.FormValue("priority"); p != "" {
			priority = p
		}
		deadlineStr = r.FormValue("deadline") // Получаем строку дедлайна

		// Обработка изображения
		file, header, err := r.FormFile("image")
		if err == nil && file != nil {
			defer func() {
				if cerr := file.Close(); cerr != nil {
					log.Printf("CreateTaskHandler: Error closing uploaded file: %v", cerr)
				}
			}()

			ext := filepath.Ext(header.Filename)
			newFilename := fmt.Sprintf("task_%d_%d%s", userID, time.Now().UnixNano(), ext)
			dstPath := filepath.Join("./uploads/tasks", newFilename)

			dst, err := os.Create(dstPath)
			if err != nil {
				log.Printf("CreateTaskHandler: Error creating image file on server: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to create image file on server")
				return
			}
			defer func() {
				if cerr := dst.Close(); cerr != nil {
					log.Printf("CreateTaskHandler: Error closing destination file: %v", cerr)
				}
			}()

			_, err = io.Copy(dst, file)
			if err != nil {
				log.Printf("CreateTaskHandler: Error saving image file: %v", err)
				RespondWithError(w, http.StatusInternalServerError, "Failed to save image file")
				_ = os.Remove(dstPath)
				return
			}
			imagePath = "/api/admin/tasks/files/" + newFilename
		} else if err != nil && err != http.ErrMissingFile {
			log.Printf("CreateTaskHandler: Error retrieving image file from form: %v", err)
		}

	} else if strings.Contains(contentType, "application/json") {
		// Обрабатываем как application/json
		log.Println("CreateTaskHandler: Processing as application/json")
		// --- ИЗМЕНЕНИЕ: Deadline теперь string ---
		var reqBody struct {
			Title            string `json:"title"`
			Description      string `json:"description"`
			AssigneeUsername string `json:"assignee_username"`
			Priority         string `json:"priority"`
			Deadline         string `json:"deadline"` // Принимаем как string
		}
		// --- КОНЕЦ ИЗМЕНЕНИЯ ---

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields() // Опционально, для строгой проверки
		if err := decoder.Decode(&reqBody); err != nil {
			log.Printf("CreateTaskHandler: Error decoding JSON body: %v", err)
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON in request body")
			return
		}

		// Валидация обязательных полей из JSON
		if reqBody.Title == "" || reqBody.AssigneeUsername == "" {
			RespondWithError(w, http.StatusBadRequest, "Title and assignee_username are required in JSON body")
			return
		}

		title = reqBody.Title
		description = reqBody.Description
		assigneeUsername = reqBody.AssigneeUsername
		if reqBody.Priority != "" {
			priority = reqBody.Priority
		}
		// deadlineStr = reqBody.Deadline // Просто копируем строку
		deadlineStr = reqBody.Deadline // Получаем строку дедлайна из JSON
		// imagePath остается пустым, так как JSON не содержит изображений

	} else {
		log.Printf("CreateTaskHandler: Unsupported Content-Type: %s", contentType)
		RespondWithError(w, http.StatusUnsupportedMediaType, "Content-Type must be multipart/form-data or application/json")
		return
	}
	// --- КОНЕЦ ИЗМЕНЕНИЙ ---

	// Валидация полей (общая часть)
	if title == "" || assigneeUsername == "" {
		// Эта проверка также останется, если валидация в JSON части не сработала
		RespondWithError(w, http.StatusBadRequest, "Title and assignee are required")
		return
	}

	var exists bool
	err = h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", assigneeUsername).Scan(&exists)
	if err != nil {
		log.Printf("CreateTaskHandler: DB error checking assignee: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error while checking assignee")
		return
	}
	if !exists {
		RespondWithError(w, http.StatusBadRequest, "Invalid assignee username")
		return
	}

	switch priority {
	case "low", "medium", "high":
	default:
		priority = "medium"
	}

	// --- ИЗМЕНЕНИЕ: Валидация строки дедлайна ---
	var deadline *string
	if deadlineStr != "" {
		_, err := time.Parse(time.RFC3339, deadlineStr) // Проверяем формат RFC3339
		if err == nil {
			deadline = &deadlineStr
		} else {
			log.Printf("CreateTaskHandler: Invalid deadline format '%s', ignoring. Error: %v", deadlineStr, err)
			// Опционально: можно вернуть ошибку 400 вместо игнорирования
			// RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid deadline format: %v", err))
			// return
		}
	}
	// --- КОНЕЦ ИЗМЕНЕНИЯ ---

	// Вставка в БД
	var taskID int64
	query := `
		INSERT INTO tasks (
			title, description, assignee_username, created_by_username,
			priority, status, deadline, image_path
		) VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
	`
	result, err := h.DB.Exec(query, title, description, assigneeUsername, creatorUsername, priority, deadline, imagePath)
	if err != nil {
		log.Printf("CreateTaskHandler: DB insert error: %v", err)
		if imagePath != "" {
			filename := filepath.Base(imagePath)
			filePath := filepath.Join("./uploads/tasks", filename)
			_ = os.Remove(filePath)
		}
		RespondWithError(w, http.StatusInternalServerError, "Failed to create task in database")
		return
	}

	taskID, err = result.LastInsertId()
	if err != nil {
		log.Printf("CreateTaskHandler: Error getting last insert ID: %v", err)
		// Файл уже сохранен, но запись в БД не удалась. Лучше удалить файл.
		if imagePath != "" {
			filename := filepath.Base(imagePath)
			filePath := filepath.Join("./uploads/tasks", filename)
			_ = os.Remove(filePath)
		}
		RespondWithError(w, http.StatusInternalServerError, "Failed to get task ID")
		return
	}

	response := map[string]interface{}{
		"id":                taskID,
		"title":             title,
		"description":       description,
		"assignee_username": assigneeUsername,
		"created_by":        creatorUsername,
		"priority":          priority,
		"status":            "pending",
		"deadline":          deadlineStr, // Возвращаем оригинальную строку
		"image_url":         imagePath,
		"created_at":        time.Now().Format(time.RFC3339),
	}

	RespondWithJSON(w, http.StatusCreated, response)
}

// GetTasksHandler — получение всех заданий
func (h *TaskHandler) GetTasksHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Добавить фильтрацию по admin_username, если нужно
	query := `
		SELECT id, title, description, assignee_username, created_by_username,
		       priority, status, deadline, image_path, created_at, updated_at
		FROM tasks
		ORDER BY created_at DESC
	`

	rows, err := h.DB.Query(query)
	if err != nil {
		log.Printf("GetTasksHandler: DB query error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error while fetching tasks")
		return
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("GetTasksHandler: Error closing rows: %v", cerr)
		}
	}()

	var tasks []Task
	for rows.Next() {
		var t Task
		// Используем sql.NullString/NullTime для полей, которые могут быть NULL
		var deadline, createdAt, updatedAt sql.NullString
		var imagePath sql.NullString

		err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &t.AssigneeUsername, &t.CreatedBy,
			&t.Priority, &t.Status, &deadline, &imagePath, &createdAt, &updatedAt,
		)
		if err != nil {
			log.Printf("GetTasksHandler: Error scanning row: %v", err)
			continue // Пропускаем эту строку и продолжаем
		}

		// Обрабатываем NULL значения
		if deadline.Valid {
			t.Deadline = deadline.String
		}
		if imagePath.Valid {
			t.ImageURL = imagePath.String
		}
		if createdAt.Valid {
			t.CreatedAt = createdAt.String
		} else {
			// На всякий случай, если DEFAULT не сработал
			t.CreatedAt = time.Now().Format(time.RFC3339)
		}
		if updatedAt.Valid {
			t.UpdatedAt = updatedAt.String
		} else {
			t.UpdatedAt = t.CreatedAt // Если updated_at NULL, используем created_at
		}

		tasks = append(tasks, t)
	}

	// Проверяем ошибки после итерации
	if err = rows.Err(); err != nil {
		log.Printf("GetTasksHandler: Rows iteration error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Error iterating through tasks")
		return
	}

	RespondWithJSON(w, http.StatusOK, tasks)
}
// handlers/task_handler.go

// GetMyTasksHandler — получение заданий, назначенных текущему пользователю
func (h *TaskHandler) GetMyTasksHandler(w http.ResponseWriter, r *http.Request) {
	userIDVal := r.Context().Value(config.UserIDKey)
	if userIDVal == nil {
		RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
		return
	}

	userID, ok := userIDVal.(int)
	if !ok {
		RespondWithError(w, http.StatusInternalServerError, "Invalid user ID")
		return
	}

	var username string
	err := h.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	rows, err := h.DB.Query(`
		SELECT id, title, description, assignee_username, created_by_username,
		       priority, status, deadline, image_path, created_at, updated_at
		FROM tasks
		WHERE assignee_username = ?
		ORDER BY created_at DESC
	`, username)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var deadline, createdAt, updatedAt sql.NullString
		var imagePath sql.NullString

		err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &t.AssigneeUsername, &t.CreatedBy,
			&t.Priority, &t.Status, &deadline, &imagePath, &createdAt, &updatedAt,
		)
		if err != nil {
			continue
		}

		if deadline.Valid {
			t.Deadline = deadline.String
		}
		if imagePath.Valid {
			t.ImageURL = imagePath.String
		}
		if createdAt.Valid {
			t.CreatedAt = createdAt.String
		} else {
			t.CreatedAt = time.Now().Format(time.RFC3339)
		}
		if updatedAt.Valid {
			t.UpdatedAt = updatedAt.String
		} else {
			t.UpdatedAt = t.CreatedAt
		}

		tasks = append(tasks, t)
	}

	RespondWithJSON(w, http.StatusOK, tasks)
}
// UpdateTaskStatusHandler — обновление статуса задания
func (h *TaskHandler) UpdateTaskStatusHandler(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "taskID")
	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid task ID format")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid request body JSON")
		return
	}

	// Валидация статуса
	switch req.Status {
	case "pending", "in_progress", "completed", "cancelled":
	default:
		RespondWithError(w, http.StatusBadRequest, "Invalid status value")
		return
	}

	result, err := h.DB.Exec("UPDATE tasks SET status = ?, updated_at = datetime('now') WHERE id = ?", req.Status, taskID)
	if err != nil {
		log.Printf("UpdateTaskStatusHandler: DB update error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error while updating task")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		RespondWithError(w, http.StatusNotFound, "Task not found")
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// DeleteTaskHandler — удаление задания
func (h *TaskHandler) DeleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "taskID")
	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		RespondWithError(w, http.StatusBadRequest, "Invalid task ID format")
		return
	}

	// Получаем путь к изображению перед удалением записи, чтобы удалить файл
	var imagePath sql.NullString
	err = h.DB.QueryRow("SELECT image_path FROM tasks WHERE id = ?", taskID).Scan(&imagePath)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondWithError(w, http.StatusNotFound, "Task not found")
		} else {
			log.Printf("DeleteTaskHandler: DB error fetching image path: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Database error while fetching task")
		}
		return
	}

	// Удаляем запись из БД
	result, err := h.DB.Exec("DELETE FROM tasks WHERE id = ?", taskID)
	if err != nil {
		log.Printf("DeleteTaskHandler: DB delete error: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Database error while deleting task")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Это маловероятно, так как мы уже проверили существование выше, но на всякий случай
		RespondWithError(w, http.StatusNotFound, "Task not found")
		return
	}

	// Удаляем файл изображения, если он существует
	if imagePath.Valid && imagePath.String != "" {
		filename := filepath.Base(imagePath.String)
		filePath := filepath.Join("./uploads/tasks", filename)
		// Игнорируем ошибку удаления файла, так как основная цель - удалить запись из БД
		err = os.Remove(filePath)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("DeleteTaskHandler: Warning - could not delete image file %s: %v", filePath, err)
		}
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ServeTaskFileHandler — раздача файлов изображений заданий
func (h *TaskHandler) ServeTaskFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	// Очищаем имя файла от потенциально опасных путей
	filename = filepath.Clean(filename)
	// Убеждаемся, что файл находится в нужной директории
	if filepath.IsAbs(filename) || filepath.Dir(filename) != "." {
		RespondWithError(w, http.StatusBadRequest, "Invalid filename")
		return
	}
	filePath := filepath.Join("./uploads/tasks", filename)

	// Проверяем, существует ли файл
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		RespondWithError(w, http.StatusNotFound, "File not found")
		return
	}

	// Отправляем файл
	http.ServeFile(w, r, filePath)
}
