package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ✅ Исправлено: ожидаем "first_name", а не "firstName"
type CreateUserRequest struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func CreateUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input CreateUserRequest

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if input.Username == "" {
			RespondWithError(w, http.StatusBadRequest, "Username is required")
			return
		}

		_, err := db.Exec(
			"INSERT INTO users (username, first_name, role) VALUES (?, ?, ?)",
			input.Username,
			input.FirstName,
			"scout",
		)
		if err != nil {
			log.Printf("DB error creating user: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "DB error creating user")
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "User created successfully"})
	}
}

// UpdateUserRoleHandler — без изменений
func UpdateUserRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		var update struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		var roleExists int
		err = db.QueryRow("SELECT COUNT(*) FROM roles WHERE name = ?", update.Role).Scan(&roleExists)
		if err != nil || roleExists == 0 {
			RespondWithError(w, http.StatusBadRequest, "Role does not exist")
			return
		}

		_, err = db.Exec("UPDATE users SET role = ? WHERE id = ?", update.Role, userID)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to update user role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User role updated successfully"})
	}
}

// UpdateUserStatusHandler — без изменений
func UpdateUserStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		var update struct {
			IsActive bool `json:"is_active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		_, err = db.Exec("UPDATE users SET is_active = ? WHERE id = ?", update.IsActive, userID)
		if err != nil {
			log.Printf("Failed to update user status: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to update user status")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User status updated successfully"})
	}
}

// DeleteUserHandler — без изменений
func DeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		result, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
		if err != nil {
			log.Printf("Failed to delete user: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete user")
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Failed to get rows affected: %v", err)
			RespondWithError(w, http.StatusInternalServerError, "Failed to check deletion status")
			return
		}

		if rowsAffected == 0 {
			RespondWithError(w, http.StatusNotFound, "User not found")
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
	}
}

// CreateRoleHandler — без изменений
func CreateRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var newRole struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&newRole); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		_, err := db.Exec("INSERT INTO roles (name) VALUES (?)", newRole.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to create new role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "Role created successfully"})
	}
}

// DeleteRoleHandler — без изменений
func DeleteRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var roleToDelete struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&roleToDelete); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if roleToDelete.Name == "user" || roleToDelete.Name == "superadmin" {
			RespondWithError(w, http.StatusBadRequest, "Cannot delete this role")
			return
		}

		_, err := db.Exec("DELETE FROM roles WHERE name = ?", roleToDelete.Name)
		if err != nil {
			RespondWithError(w, http.StatusInternalServerError, "Failed to delete role: "+err.Error())
			return
		}

		RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Role deleted successfully"})
	}
}
