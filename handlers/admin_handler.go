package handlers
import (

    "database/sql"

    "encoding/json"

    "net/http"

    "strconv"

)


type AdminHandler struct {

    DB *sql.DB

}


func NewAdminHandler(db *sql.DB) *AdminHandler {

    return &AdminHandler{DB: db}

}


func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {

    rows, err := h.DB.Query("SELECT id, username, first_name, role FROM users ORDER BY id")

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Database error")

        return

    }

    defer rows.Close()


    var users []map[string]interface{}

    for rows.Next() {

        var user struct {

            ID        int

            Username  string

            FirstName string

            Role      string

        }

        if err := rows.Scan(&user.ID, &user.Username, &user.FirstName, &user.Role); err != nil {

            RespondWithError(w, http.StatusInternalServerError, "Database scan error")

            return

        }

        users = append(users, map[string]interface{}{

            "id":        user.ID,

            "username":  user.Username,

            "firstName": user.FirstName,

            "role":      user.Role,

        })

    }


    RespondWithJSON(w, http.StatusOK, users)

}


func (h *AdminHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {

    userID, err := strconv.Atoi(r.URL.Query().Get("id"))

    if err != nil {

        RespondWithError(w, http.StatusBadRequest, "Invalid user ID")

        return

    }


    var input struct {

        Role string `json:"role"`

    }

    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {

        RespondWithError(w, http.StatusBadRequest, "Invalid request data")

        return

    }


    _, err = h.DB.Exec("UPDATE users SET role = ? WHERE id = ?", input.Role, userID)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to update user role")

        return

    }


    RespondWithJSON(w, http.StatusOK, map[string]string{

        "message": "User role updated successfully",

    })

} 
