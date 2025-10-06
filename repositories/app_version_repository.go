package repositories

import (
    "database/sql"
    "fmt"
    // "time"
    "github.com/evn/eom_backendl/models"
)

type AppVersionRepository struct {
    DB *sql.DB
}

func NewAppVersionRepository(db *sql.DB) *AppVersionRepository {
    return &AppVersionRepository{DB: db}
}

// GetLatestVersion получает последнюю активную версию для платформы
func (r *AppVersionRepository) GetLatestVersion(platform string) (*models.AppVersion, error) {
    query := `
        SELECT id, platform, version, build_number, release_notes, download_url, 
               min_sdk_version, is_mandatory, is_active, created_at, updated_at
        FROM app_versions 
        WHERE platform = $1 AND is_active = TRUE 
        ORDER BY build_number DESC 
        LIMIT 1
    `
    
    var version models.AppVersion
    err := r.DB.QueryRow(query, platform).Scan(
        &version.ID,
        &version.Platform,
        &version.Version,
        &version.BuildNumber,
        &version.ReleaseNotes,
        &version.DownloadURL,
        &version.MinSDKVersion,
        &version.IsMandatory,
        &version.IsActive,
        &version.CreatedAt,
        &version.UpdatedAt,
    )
    
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, fmt.Errorf("no active version found for platform %s", platform)
        }
        return nil, fmt.Errorf("failed to get latest version: %w", err)
    }
    
    return &version, nil
}
// CheckVersion проверяет, доступна ли новая версия
func (r *AppVersionRepository) CheckVersion(platform, currentVersion string, buildNumber int) (*models.VersionCheckResponse, error) {
    latestVersion, err := r.GetLatestVersion(platform)
    if err != nil {
        return &models.VersionCheckResponse{
            HasUpdate:   false,
            Message:     "No active versions available",
            IsMandatory: false,
        }, nil
    }
    
    hasUpdate := buildNumber < latestVersion.BuildNumber
    isMandatory := latestVersion.IsMandatory || (hasUpdate && latestVersion.MinSDKVersion > 0)
    
    response := &models.VersionCheckResponse{
        HasUpdate:     hasUpdate,
        IsMandatory:   isMandatory,
    }
    
    if hasUpdate {
        response.LatestVersion = latestVersion
        if isMandatory {
            response.Message = "Доступно обязательное обновление"
        } else {
            response.Message = "Доступно новое обновление"
        }
    } else {
        response.Message = "У вас установлена последняя версия"
    }
    
    return response, nil
}
// GetAllVersions получает все версии для платформы
func (r *AppVersionRepository) GetAllVersions(platform string) ([]models.AppVersion, error) {
    query := `
        SELECT id, platform, version, build_number, release_notes, download_url, 
               min_sdk_version, is_mandatory, is_active, created_at, updated_at
        FROM app_versions 
        WHERE platform = $1 
        ORDER BY build_number DESC
    `
    
    rows, err := r.DB.Query(query, platform)
    if err != nil {
        return nil, fmt.Errorf("failed to query versions: %w", err)
    }
    defer rows.Close()
    
    var versions []models.AppVersion
    for rows.Next() {
        var version models.AppVersion
        err := rows.Scan(
            &version.ID,
            &version.Platform,
            &version.Version,
            &version.BuildNumber,
            &version.ReleaseNotes,
            &version.DownloadURL,
            &version.MinSDKVersion,
            &version.IsMandatory,
            &version.IsActive,
            &version.CreatedAt,
            &version.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan version: %w", err)
        }
        versions = append(versions, version)
    }
    
    return versions, nil
}

// CreateVersion создает новую версию
func (r *AppVersionRepository) CreateVersion(version *models.AppVersion) error {
    query := `
        INSERT INTO app_versions 
        (platform, version, build_number, release_notes, download_url, min_sdk_version, is_mandatory, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
        RETURNING id
    `
    
    // Используем QueryRow для получения id сразу
    err := r.DB.QueryRow(
        query,
        version.Platform,
        version.Version,
        version.BuildNumber,
        version.ReleaseNotes,
        version.DownloadURL,
        version.MinSDKVersion,
        version.IsMandatory,
        version.IsActive,
    ).Scan(&version.ID)
    
    if err != nil {
        return fmt.Errorf("failed to create version: %w", err)
    }
    
    return nil
}

// UpdateVersion обновляет существующую версию
func (r *AppVersionRepository) UpdateVersion(version *models.AppVersion) error {
    query := `
        UPDATE app_versions 
        SET platform = $1, version = $2, build_number = $3, release_notes = $4, 
            download_url = $5, min_sdk_version = $6, is_mandatory = $7, is_active = $8, updated_at = NOW()
        WHERE id = $9
    `
    
    _, err := r.DB.Exec(
        query,
        version.Platform,
        version.Version,
        version.BuildNumber,
        version.ReleaseNotes,
        version.DownloadURL,
        version.MinSDKVersion,
        version.IsMandatory,
        version.IsActive,
        version.ID,
    )
    if err != nil {
        return fmt.Errorf("failed to update version: %w", err)
    }
    
    return nil
}

// DeleteVersion удаляет версию
func (r *AppVersionRepository) DeleteVersion(id int) error {
    query := `DELETE FROM app_versions WHERE id = $1`
    _, err := r.DB.Exec(query, id)
    if err != nil {
        return fmt.Errorf("failed to delete version: %w", err)
    }
    return nil
}   