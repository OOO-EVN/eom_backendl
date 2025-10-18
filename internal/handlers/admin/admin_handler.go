package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/evn/eom_backendl/internal/pkg/response"
	"github.com/go-chi/chi/v5"
)

type CreateUserRequest struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func CreateUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input CreateUserRequest

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if input.Username == "" {
			response.RespondWithError(w, http.StatusBadRequest, "Username is required")
			return
		}

		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", input.Username).Scan(&count)
		if err != nil {
			log.Printf("DB error checking for existing user: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "DB error")
			return
		}
		if count > 0 {
			response.RespondWithError(w, http.StatusConflict, "Username already exists")
			return
		}

		_, err = db.Exec(
			"INSERT INTO users (username, first_name, role) VALUES ($1, $2, $3)",
			input.Username,
			input.FirstName,
			"scout",
		)
		if err != nil {
			log.Printf("DB error creating user: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "DB error creating user")
			return
		}

		response.RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "User created successfully"})
	}
}
func UpdateUserRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		var update struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		var roleExists int
		err = db.QueryRow("SELECT COUNT(*) FROM roles WHERE name = $1", update.Role).Scan(&roleExists)
		if err != nil || roleExists == 0 {
			response.RespondWithError(w, http.StatusBadRequest, "Role does not exist")
			return
		}

		_, err = db.Exec("UPDATE users SET role = $1 WHERE id = $2", update.Role, userID)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to update user role: "+err.Error())
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User role updated successfully"})
	}
}

func UpdateUserStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем userID из URL
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		// Декодируем тело запроса
		var req struct {
			Status string `json:"status"` // Ожидаем "active" или "pending"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Проверяем, что статус допустимый
		if req.Status != "active" && req.Status != "pending" {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid status value. Must be 'active' or 'pending'")
			return
		}

		// Подготавливаем значения для БД
		isActive := 0
		if req.Status == "active" {
			isActive = 1
		}

		// Обновляем запись в БД
		_, err = db.Exec("UPDATE users SET status = $1, is_active = $2 WHERE id = $3", req.Status, isActive, userID)
		if err != nil {
			log.Printf("Failed to update user %d status: %v", userID, err)
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to update user status")
			return
		}

		// Отправляем успешный ответ
		response.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User status updated successfully"})
	}
}
func DeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		result, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
		if err != nil {
			log.Printf("Failed to delete user: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to delete user")
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Failed to get rows affected: %v", err)
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to check deletion status")
			return
		}

		if rowsAffected == 0 {
			response.RespondWithError(w, http.StatusNotFound, "User not found")
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
	}
}

func CreateRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var newRole struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&newRole); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		_, err := db.Exec("INSERT INTO roles (name) VALUES ($1)", newRole.Name)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to create new role: "+err.Error())
			return
		}

		response.RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "Role created successfully"})
	}
}

func DeleteRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var roleToDelete struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&roleToDelete); err != nil {
			response.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if roleToDelete.Name == "user" || roleToDelete.Name == "superadmin" {
			response.RespondWithError(w, http.StatusBadRequest, "Cannot delete this role")
			return
		}

		_, err := db.Exec("DELETE FROM roles WHERE name = $1", roleToDelete.Name)
		if err != nil {
			response.RespondWithError(w, http.StatusInternalServerError, "Failed to delete role: "+err.Error())
			return
		}

		response.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Role deleted successfully"})
	}
}
