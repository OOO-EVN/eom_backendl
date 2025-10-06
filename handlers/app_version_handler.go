package handlers

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    
    "github.com/go-chi/chi/v5"
    "github.com/evn/eom_backendl/models"
    "github.com/evn/eom_backendl/repositories"
    "github.com/evn/eom_backendl/config"
)

type AppVersionHandler struct {
    repo *repositories.AppVersionRepository
    db   *sql.DB  // Добавляем DB для доступа к пользователям
}

func NewAppVersionHandler(db *sql.DB) *AppVersionHandler {
    return &AppVersionHandler{
        repo: repositories.NewAppVersionRepository(db),
        db:   db,
    }
}

// CheckVersionHandler проверяет наличие обновлений
func (h *AppVersionHandler) CheckVersionHandler(w http.ResponseWriter, r *http.Request) {
    userID, ok := r.Context().Value(config.UserIDKey).(int)
    if !ok {
        RespondWithError(w, http.StatusUnauthorized, "User not authenticated")
        return
    }
    
    var req models.VersionCheckRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    // Определяем платформу по User-Agent или из запроса
    if req.Platform == "" {
        req.Platform = h.detectPlatform(r)
    }
    
    response, err := h.repo.CheckVersion(req.Platform, req.CurrentVersion, req.BuildNumber)
    if err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to check version: "+err.Error())
        return
    }
    
    // Логируем проверку обновлений
    fmt.Printf("User %d checked for updates. Platform: %s, Current: %s, Build: %d, HasUpdate: %t\n", 
        userID, req.Platform, req.CurrentVersion, req.BuildNumber, response.HasUpdate)
    
    RespondWithJSON(w, http.StatusOK, response)
}

// GetLatestVersionHandler возвращает последнюю версию для платформы
func (h *AppVersionHandler) GetLatestVersionHandler(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    if platform == "" {
        platform = h.detectPlatform(r)
    }
    
    version, err := h.repo.GetLatestVersion(platform)
    if err != nil {
        RespondWithError(w, http.StatusNotFound, "No version found: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, version)
}

// ListVersionsHandler возвращает список всех версий (для админов)
func (h *AppVersionHandler) ListVersionsHandler(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    
    var versions []models.AppVersion
    var err error
    
    if platform != "" {
        versions, err = h.repo.GetAllVersions(platform)
    } else {
        // Получаем все версии для всех платформ
        androidVersions, _ := h.repo.GetAllVersions("android")
        iosVersions, _ := h.repo.GetAllVersions("ios")
        versions = append(androidVersions, iosVersions...)
    }
    
    if err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to list versions: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, versions)
}

// CreateVersionHandler создает новую версию (только для superadmin)
func (h *AppVersionHandler) CreateVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    var version models.AppVersion
    if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    if err := h.repo.CreateVersion(&version); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to create version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusCreated, version)
}

// UpdateVersionHandler обновляет существующую версию (только для superadmin)
func (h *AppVersionHandler) UpdateVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    idStr := chi.URLParam(r, "id")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid version ID")
        return
    }
    
    var version models.AppVersion
    if err := json.NewDecoder(r.Body).Decode(&version); err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    version.ID = id
    if err := h.repo.UpdateVersion(&version); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to update version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, version)
}

// DeleteVersionHandler удаляет версию (только для superadmin)
func (h *AppVersionHandler) DeleteVersionHandler(w http.ResponseWriter, r *http.Request) {
    // Проверка прав администратора
    if !h.isSuperAdmin(r) {
        RespondWithError(w, http.StatusForbidden, "Access denied")
        return
    }
    
    idStr := chi.URLParam(r, "id")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        RespondWithError(w, http.StatusBadRequest, "Invalid version ID")
        return
    }
    
    if err := h.repo.DeleteVersion(id); err != nil {
        RespondWithError(w, http.StatusInternalServerError, "Failed to delete version: "+err.Error())
        return
    }
    
    RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Version deleted successfully"})
}

// Вспомогательные методы

func (h *AppVersionHandler) detectPlatform(r *http.Request) string {
    userAgent := r.Header.Get("User-Agent")
    switch {
    case strings.Contains(userAgent, "Android"):
        return "android"
    case strings.Contains(userAgent, "iPhone"), strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "iOS"):
        return "ios"
    default:
        return "unknown"
    }
}

func (h *AppVersionHandler) isSuperAdmin(r *http.Request) bool {
    userID, ok := r.Context().Value(config.UserIDKey).(int)
    if !ok {
        return false
    }
    
    role := h.getUserRole(userID)
    return role == "superadmin"
}

// Вспомогательная функция для получения роли пользователя
func (h *AppVersionHandler) getUserRole(userID int) string {
    var role string
    err := h.db.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
    if err != nil {
        return "user"
    }
    return role
}
