package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"github.com/go-chi/chi/v5"
)

// Структура для обновления роли пользователя
type UserRoleUpdate struct {
	Role string `json:"role"`
}

// Хэндлер для обновления роли пользователя
func UpdateUserRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "userID")
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid User ID")
			return
		}

		var update UserRoleUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Проверяем, существует ли такая роль
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

// Структура для создания/удаления роли
type Role struct {
	Name string `json:"name"`
}

// Хэндлер для добавления новой роли
func CreateRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var newRole Role
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

// Хэндлер для удаления роли
func DeleteRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var roleToDelete Role
		if err := json.NewDecoder(r.Body).Decode(&roleToDelete); err != nil {
			RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Защита от удаления обязательных ролей
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
