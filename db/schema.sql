-- Таблица пользователей
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    first_name TEXT,
    telegram_id BIGINT UNIQUE,
    role TEXT NOT NULL DEFAULT 'user',
    status TEXT DEFAULT 'active',       
    avatar_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица карт
CREATE TABLE IF NOT EXISTS maps (
    id SERIAL PRIMARY KEY,
    city TEXT NOT NULL,
    description TEXT,
    file_name TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    upload_date TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица версий приложения
CREATE TABLE IF NOT EXISTS app_versions (
    id SERIAL PRIMARY KEY,
    platform TEXT NOT NULL,
    version TEXT NOT NULL,
    build_number INTEGER NOT NULL,
    release_notes TEXT,
    download_url TEXT NOT NULL,
    min_sdk_version INTEGER DEFAULT 0,
    is_mandatory BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Индексы
CREATE INDEX IF NOT EXISTS idx_app_versions_platform_version ON app_versions(platform, version);
CREATE INDEX IF NOT EXISTS idx_app_versions_active ON app_versions(is_active);
CREATE INDEX IF NOT EXISTS idx_app_versions_build ON app_versions(build_number);

-- Очистка и инициализация данных
DELETE FROM app_versions WHERE platform = 'android' OR platform = 'ios';

INSERT INTO app_versions (platform, version, build_number, release_notes, download_url, is_mandatory, is_active, created_at, updated_at) VALUES
('android', '1.0.0', 100, 'Первый релиз приложения', 'https://eom-sharing.duckdns.org/uploads/app/app-release.apk  ', FALSE, TRUE, NOW(), NOW()),
('ios', '1.0.0', 100, 'Первый релиз приложения', 'https://eom-sharing.duckdns.org/uploads/app/app-release.ipa  ', FALSE, TRUE, NOW(), NOW());


CREATE TABLE IF NOT EXISTS zones (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица доступных временных слотов
CREATE TABLE IF NOT EXISTS available_time_slots (
    id SERIAL PRIMARY KEY,
    slot_time_range TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица смен (слотов)
CREATE TABLE IF NOT EXISTS slots (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    slot_time_range TEXT NOT NULL,
    position TEXT NOT NULL,
    zone TEXT NOT NULL,
    selfie_path TEXT,
    worked_duration INTEGER, -- в секундах
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Индексы для производительности
CREATE INDEX IF NOT EXISTS idx_slots_user_id ON slots(user_id);
CREATE INDEX IF NOT EXISTS idx_slots_active ON slots(user_id, end_time) WHERE end_time IS NULL;
CREATE INDEX IF NOT EXISTS idx_slots_zone ON slots(zone);
CREATE INDEX IF NOT EXISTS idx_slots_time_range ON slots(slot_time_range);


INSERT INTO available_time_slots (slot_time_range, description) VALUES 
    ('07:00-15:00', 'Утренняя смена'),
    ('15:00-23:00', 'Вечерняя смена'),
    ('07:00-23:00', 'Полная смена')
ON CONFLICT (slot_time_range) DO NOTHING;