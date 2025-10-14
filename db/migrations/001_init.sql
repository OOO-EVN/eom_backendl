-- lib/db/migrations/001_init.sql
-- Создание всех необходимых таблиц

-- Создание таблицы пользователей
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    first_name TEXT,
    last_name TEXT,
    role TEXT DEFAULT 'user',
    is_active BOOLEAN DEFAULT FALSE,
    telegram_id TEXT UNIQUE,
    email TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Создание таблицы смен
CREATE TABLE IF NOT EXISTS slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    slot_time_range TEXT NOT NULL,
    position TEXT NOT NULL,
    zone TEXT NOT NULL,
    selfie_path TEXT,
    start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
    end_time DATETIME,
    worked_duration INTEGER DEFAULT 0,  -- Добавляем недостающую колонку
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Создание таблицы заданий
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    assignee_username TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    priority TEXT DEFAULT 'medium',
    status TEXT DEFAULT 'pending',
    deadline DATETIME,
    image_url TEXT,
    created_by INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (assignee_username) REFERENCES users(username) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Создание таблицы карт
CREATE TABLE IF NOT EXISTS maps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    city TEXT NOT NULL,
    description TEXT,
    file_name TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    upload_date DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Создание таблицы ролей
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Создание индексов для ускорения запросов
CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_slots_user_id ON slots(user_id);
CREATE INDEX IF NOT EXISTS idx_slots_start_time ON slots(start_time);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_username);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
CREATE INDEX IF NOT EXISTS idx_maps_city ON maps(city);
CREATE INDEX IF NOT EXISTS idx_maps_upload_date ON maps(upload_date);

-- Вставка стандартных ролей
INSERT OR IGNORE INTO roles (name, description) VALUES 
    ('superadmin', 'Полный доступ ко всем функциям'),
    ('admin', 'Административные функции'),
    ('coordinator', 'Координатор зон'),
    ('scout', 'Скаут (пользователь)'),
    ('user', 'Обычный пользователь');

-- Вставка суперадмина (логин: evn, пароль: root-evn)
INSERT OR IGNORE INTO users 
(username, password_hash, first_name, last_name, role, is_active) 
VALUES 
('evn', '$2a$10$8K1p/a0dURXAm7QiTRqNa.E3YPWsQjW/GowrgA094kL4FxH.lBs8O', 'Evn', 'Root', 'superadmin', TRUE);
CREATE TABLE IF NOT EXISTS positions (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL,
  lat DOUBLE PRECISION NOT NULL,
  lon DOUBLE PRECISION NOT NULL,
  speed DOUBLE PRECISION,
  accuracy DOUBLE PRECISION,
  battery INT CHECK (battery BETWEEN 0 AND 100),
  event TEXT DEFAULT 'heartbeat',
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_positions_user_id ON positions(user_id);
CREATE INDEX IF NOT EXISTS idx_positions_created_at ON positions(created_at);