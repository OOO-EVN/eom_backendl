 package handlers


import (

    "database/sql"

    "encoding/json"

    "errors"

    "fmt"

    "io"

    "log"

    "net/http"

    "strconv"

    "strings"


    "github.com/evn/eom_backendl/services"

)


type AuthHandler struct {

    db                  *sql.DB

    jwtService          *services.JWTService

    telegramAuthService *services.TelegramAuthService

}


func NewAuthHandler(db *sql.DB, jwtService *services.JWTService, tgService *services.TelegramAuthService) *AuthHandler {

    return &AuthHandler{

        db:                  db,

        jwtService:          jwtService,

        telegramAuthService: tgService,

    }

}


func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {

    var regData struct {

        Username  string `json:"username"`

        FirstName string `json:"first_name"`

        Password  string `json:"password"`

    }

    if err := json.NewDecoder(r.Body).Decode(&regData); err != nil {

        RespondWithError(w, http.StatusBadRequest, "Invalid request data")

        return

    }

    var count int

    err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", regData.Username).Scan(&count)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Database error")

        return

    }

    if count > 0 {

        RespondWithError(w, http.StatusBadRequest, "Username already exists")

        return

    }

    passwordHash, err := services.HashPassword(regData.Password)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to hash password")

        return

    }

    _, err = h.db.Exec(`

        INSERT INTO users (username, first_name, password_hash, role)

        VALUES (?, ?, ?, 'user')`,

        regData.Username,

        regData.FirstName,

        passwordHash,

    )

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to create user")

        return

    }

    RespondWithJSON(w, http.StatusCreated, map[string]string{

        "message": "User registered successfully",

    })

}


func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {

    var loginData struct {

        Username string `json:"username"`

        Password string `json:"password"`

    }

    if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {

        RespondWithError(w, http.StatusBadRequest, "Invalid request data")

        return

    }

    var user struct {

        ID           int

        Username     string

        PasswordHash string

        Role         string

    }

    row := h.db.QueryRow(`

        SELECT id, username, password_hash, role

        FROM users

        WHERE username = ? COLLATE NOCASE`,

        loginData.Username,

    )

    err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role)

    if err != nil {

        if errors.Is(err, sql.ErrNoRows) {

            RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")

        } else {

            RespondWithError(w, http.StatusInternalServerError, "Database error: "+err.Error())

        }

        return

    }

    if !services.CheckPasswordHash(loginData.Password, user.PasswordHash) {

        RespondWithError(w, http.StatusUnauthorized, "Invalid credentials")

        return

    }

    token, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")

        return

    }

    RespondWithJSON(w, http.StatusOK, map[string]string{

        "token": token,

        "role":  user.Role,

    })

}


func (h *AuthHandler) TelegramAuthHandler(w http.ResponseWriter, r *http.Request) {

    var tgData map[string]string

    if err := json.NewDecoder(r.Body).Decode(&tgData); err != nil {

        RespondWithError(w, http.StatusBadRequest, "Invalid request data")

        return

    }

    validatedData, err := h.telegramAuthService.ValidateAndExtract(tgData)

    if err != nil {

        RespondWithError(w, http.StatusUnauthorized, "Telegram auth failed: "+err.Error())

        return

    }

    var user struct {

        ID         int

        Username   string

        FirstName  string

        TelegramID int

        Role       string

    }

    tgID, _ := strconv.Atoi(validatedData["id"])

    err = h.db.QueryRow(`

        SELECT id, username, first_name, telegram_id, role

        FROM users

        WHERE telegram_id = ?`,

        tgID,

    ).Scan(&user.ID, &user.Username, &user.FirstName, &user.TelegramID, &user.Role)

    if err != nil && !errors.Is(err, sql.ErrNoRows) {

        RespondWithError(w, http.StatusInternalServerError, "Database error")

        return

    }

    if errors.Is(err, sql.ErrNoRows) {

        username := validatedData["username"]

        if username == "" {

            username = "tg_user_" + validatedData["id"]

        }

        res, err := h.db.Exec(`

            INSERT INTO users (telegram_id, username, first_name, role)

            VALUES (?, ?, ?, 'user')`,

            tgID,

            username,

            validatedData["first_name"],

        )

        if err != nil {

            RespondWithError(w, http.StatusInternalServerError, "Failed to create user: "+err.Error())

            return

        }

        id, _ := res.LastInsertId()

        user.ID = int(id)

        user.Username = username

        user.FirstName = validatedData["first_name"]

        user.TelegramID = tgID

        user.Role = "user"

    }

    token, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to generate token")

        return

    }

    RespondWithJSON(w, http.StatusOK, map[string]interface{}{

        "token":       token,

        "user_id":     user.ID,

        "username":    user.Username,

        "first_name":  user.FirstName,

        "telegram_id": user.TelegramID,

        "role":        user.Role,

    })

}


func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {

    RespondWithJSON(w, http.StatusOK, map[string]string{

        "message": "Logged out successfully",

    })

}


func (h *AuthHandler) TelegramAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {

    data := make(map[string]string)

    for key, values := range r.URL.Query() {

        if len(values) > 0 {

            data[key] = values[0]

        }

    }


    if _, ok := data["id"]; !ok {

        RespondWithError(w, http.StatusBadRequest, "Missing 'id'")

        return

    }

    if _, ok := data["hash"]; !ok {

        RespondWithError(w, http.StatusBadRequest, "Missing 'hash'")

        return

    }


    log.Printf("Telegram callback received: %+v", data)


    jsonData, err := json.Marshal(data)

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Failed to encode data")

        return

    }


    resp, err := http.Post(

        "http://localhost:6066/api/auth/telegram",

        "application/json",

        strings.NewReader(string(jsonData)),

    )

    if err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Internal service error: "+err.Error())

        return

    }

    defer resp.Body.Close()


    body, _ := io.ReadAll(resp.Body)

    var result map[string]interface{}

    if err := json.Unmarshal(body, &result); err != nil {

        RespondWithError(w, http.StatusInternalServerError, "Invalid response from auth handler")

        return

    }


    if resp.StatusCode == http.StatusOK {

        token, ok := result["token"].(string)

        if !ok {

            RespondWithError(w, http.StatusInternalServerError, "Token not found in response")

            return

        }


        html := fmt.Sprintf(`

        <!DOCTYPE html>

        <html>

        <head>

            <title>Auth Success</title>

            <script>

                window.location.href = "%s/api/auth/telegram-success?token=%s";

            </script>

        </head>

        <body>

            <p>Авторизация прошла успешно... Перенаправление...</p>

        </body>

        </html>

        `, AppConfigBackendURL, token)


        w.Header().Set("Content-Type", "text/html; charset=utf-8")

        w.WriteHeader(http.StatusOK)

        w.Write([]byte(html))

    } else {

        errorMsg, _ := result["error"].(string)

        if errorMsg == "" {

            errorMsg = "Authorization failed"

        }

        RespondWithError(w, resp.StatusCode, errorMsg)

    }

}



const AppConfigBackendURL = "https://eom-sharing.duckdns.org" 
