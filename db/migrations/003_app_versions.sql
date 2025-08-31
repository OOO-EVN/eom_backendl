-- Создание таблицы версий приложения
CREATE TABLE IF NOT EXISTS app_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    platform TEXT NOT NULL, -- 'android' или 'ios'
    version TEXT NOT NULL,  -- '1.0.0'
    build_number INTEGER NOT NULL,
    release_notes TEXT,
    download_url TEXT NOT NULL,
    min_sdk_version INTEGER DEFAULT 0,
    is_mandatory BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для оптимизации
CREATE INDEX IF NOT EXISTS idx_app_versions_platform_version ON app_versions(platform, version);
CREATE INDEX IF NOT EXISTS idx_app_versions_active ON app_versions(is_active);
CREATE INDEX IF NOT EXISTS idx_app_versions_build ON app_versions(build_number);

-- Пример начальных данных (адаптируйте под свои нужды)
INSERT OR IGNORE INTO app_versions (platform, version, build_number, release_notes, download_url, is_mandatory, is_active) VALUES
('android', '1.0.0', 100, 'Первый релиз приложения', 'https://example.com/app-release.apk', FALSE, TRUE),
('ios', '1.0.0', 100, 'Первый релиз приложения', 'https://apps.apple.com/app/id123456789', FALSE, TRUE);
