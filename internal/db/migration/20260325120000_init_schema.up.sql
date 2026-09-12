-- Включаем поддержку внешних ключей (обязательно при каждом подключении!)
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    avatar TEXT,
    full_name TEXT NOT NULL,       -- ФИО
    password TEXT NOT NULL,
    role TEXT NOT NULL,            -- Роль: студент, преподаватель, сотрудник, админ
    position TEXT,
    phone TEXT,                    -- Контактный телефон
    email TEXT NOT NULL UNIQUE,    -- Email
    date_of_birth TEXT,
    office TEXT,
    is_active BOOLEAN DEFAULT 1,   -- Активен ли пользователь (уволился/выпустился)
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tokens (
    user_id TEXT,
    role TEXT,
    token_id TEXT,
    expired_at TIMESTAMP
);

-- Таблица ключей
CREATE TABLE IF NOT EXISTS keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_number TEXT UNIQUE NOT NULL, -- Уникальный номер ключа (бирка)
    room_description TEXT NOT NULL,  -- Какое помещение открывает (напр. "Лаборатория 305")
    status TEXT DEFAULT 'available', -- Статус: 'available' (свободен), 'issued' (выдан), 'lost' (утерян)
    notes TEXT                       -- Примечания (напр. "дубликат", "сломан замок")
);

-- Таблица журнала выдачи/возврата
CREATE TABLE IF NOT EXISTS key_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id INTEGER NOT NULL,         -- Ссылка на ключ
    user_id TEXT NOT NULL,       -- Ссылка на пользователя
    action_type TEXT NOT NULL,       -- Тип действия: 'issue' (выдача), 'return' (возврат)
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, -- Время события
    comment TEXT,                    -- Комментарий оператора (кто выдал, состояние ключа)
    FOREIGN KEY (key_id) REFERENCES keys(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Индексы для ускорения поиска по истории
CREATE INDEX IF NOT EXISTS idx_key_logs_key_id ON key_logs(key_id);
CREATE INDEX IF NOT EXISTS idx_key_logs_user_id ON key_logs(user_id);


CREATE TABLE IF NOT EXISTS inventory(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('equipment', 'inventory', 'raw_material', 'other')),
    name TEXT NOT NULL NOT NULL, -- НАИМЕНОВАНИЕ
    description TEXT, -- КАК РАБОТАТЬ
    location TEXT NOT NULL, -- РАСПОЛОЖЕНИЕ
    documentation TEXT, -- ДОКУМЕНТАЦИЯ
    inventory_number TEXT UNIQUE, -- ИНВЕНТАРНЫЙ НОМЕР
    responsible_id TEXT NOT NULL, -- ОТВЕТСТВЕННЫЙ
    status BOOLEAN DEFAULT 1, -- Статус: 1 - ДОСТУПЕН, 0 - НЕ ДОСТУПЕН
    unavailable_reason TEXT,
    last_verification_date DATE, -- ДАТА ПОВЕРКИ
    next_verification_date DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (responsible_id) REFERENCES users(id)
);

-- internal/db/migration/20260729100000_add_inventory_photos.up.sql

CREATE TABLE IF NOT EXISTS inventory_photos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    inventory_id INTEGER NOT NULL,
    filename TEXT NOT NULL,          -- оригинальное имя файла
    stored_name TEXT NOT NULL,       -- имя на диске (UUID)
    content_type TEXT NOT NULL,      -- image/jpeg, image/png
    size_bytes INTEGER NOT NULL,
    uploaded_by TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inventory_id) REFERENCES inventory(id) ON DELETE CASCADE,
    FOREIGN KEY (uploaded_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_photos_inventory ON inventory_photos(inventory_id);

CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,                 -- Название публикации
    details TEXT,                        -- Выходные данные
    indexing TEXT,                       -- Индексирование (квартиль)
    white_list_level TEXT,               -- Уровень белого списка
    funding TEXT,                        -- Финансирование (ГЗ, РНФ...)
    link TEXT,                           -- Ссылка
    status TEXT NOT NULL DEFAULT 'planned', -- planned / submitted / published
    created_by TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id)
);

-- Статьи ↔ Авторы (автор может быть сотрудником ИЛИ внешним)
CREATE TABLE IF NOT EXISTS article_authors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id INTEGER NOT NULL,
    user_id TEXT,                        -- nullable: ссылка на сотрудника
    name TEXT NOT NULL,                  -- отображаемое имя (всегда)
    sort_order INTEGER DEFAULT 0,        -- порядок авторов
    FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_article_authors_article ON article_authors(article_id);
CREATE INDEX IF NOT EXISTS idx_articles_status ON articles(status);

CREATE TABLE IF NOT EXISTS events(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    creator_id TEXT NOT NULL,
    title TEXT NOT NULL,
    location TEXT NOT NULL,
    description TEXT,
    start_time TIMESTAMP NOT NULL,
    is_public BOOLEAN DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_events_creator ON events(creator_id);
CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
CREATE INDEX IF NOT EXISTS idx_events_is_public ON events(is_public);