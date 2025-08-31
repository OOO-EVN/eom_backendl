-- db/migrations/002_zones.sql
CREATE TABLE IF NOT EXISTS zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

-- Начальные зоны — цифры
INSERT OR IGNORE INTO zones (name) VALUES ('1');
INSERT OR IGNORE INTO zones (name) VALUES ('2');
INSERT OR IGNORE INTO zones (name) VALUES ('3');
INSERT OR IGNORE INTO zones (name) VALUES ('4');
INSERT OR IGNORE INTO zones (name) VALUES ('5');
