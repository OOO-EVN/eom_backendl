package models

import (
    "time"
)

type AppVersion struct {
    ID             int       `json:"id" db:"id"`
    Platform       string    `json:"platform" db:"platform"`
    Version        string    `json:"version" db:"version"`
    BuildNumber    int       `json:"build_number" db:"build_number"`
    ReleaseNotes   string    `json:"release_notes" db:"release_notes"`
    DownloadURL    string    `json:"download_url" db:"download_url"`
    MinSDKVersion  int       `json:"min_sdk_version" db:"min_sdk_version"`
    IsMandatory    bool      `json:"is_mandatory" db:"is_mandatory"`
    IsActive       bool      `json:"is_active" db:"is_active"`
    CreatedAt      time.Time `json:"created_at" db:"created_at"`
    UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type VersionCheckRequest struct {
    Platform    string `json:"platform"`     // 'android' или 'ios'
    CurrentVersion string `json:"current_version"` // '1.0.0'
    BuildNumber int    `json:"build_number"` // 100
    DeviceInfo  string `json:"device_info,omitempty"`
}

type VersionCheckResponse struct {
    HasUpdate     bool       `json:"has_update"`
    LatestVersion *AppVersion `json:"latest_version,omitempty"`
    Message       string     `json:"message,omitempty"`
    IsMandatory   bool       `json:"is_mandatory"`
}
