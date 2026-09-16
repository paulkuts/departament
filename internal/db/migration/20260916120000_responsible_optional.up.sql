-- 20260916120000_responsible_optional.up.sql
-- «Ответственный» у объекта реестра становится необязательным: снимаем NOT NULL
-- с inventory.responsible_id (внешний ключ на users(id) сохраняется).
--
-- SQLite не умеет убирать NOT NULL на месте, поэтому таблица пересобирается целиком.
-- Дочерние таблицы (фотографии, комментарии, выдачи) на время пересборки выносятся
-- в копии: DROP TABLE inventory при включённом foreign_keys каскадом удалил бы
-- фотографии (ON DELETE CASCADE), а выдачи заблокировали бы удаление.
-- Проверено на копии боевой базы: данные, внешние ключи и счётчики AUTOINCREMENT
-- сохраняются, NULL принимается, несуществующий id сотрудника по-прежнему отбивается.

CREATE TABLE inventory_photos_copy AS SELECT * FROM inventory_photos;
CREATE TABLE inventory_comments_copy AS SELECT * FROM inventory_comments;
CREATE TABLE inventory_loans_copy AS SELECT * FROM inventory_loans;

DROP TABLE inventory_photos;
DROP TABLE inventory_comments;
DROP TABLE inventory_loans;

CREATE TABLE inventory_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('equipment', 'inventory', 'raw_material', 'other')),
    name TEXT NOT NULL,
    description TEXT,
    location TEXT NOT NULL,
    documentation TEXT,
    inventory_number TEXT UNIQUE,
    responsible_id TEXT,
    status BOOLEAN DEFAULT 1,
    unavailable_reason TEXT,
    last_verification_date DATE,
    next_verification_date DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (responsible_id) REFERENCES users(id)
);

INSERT INTO inventory_new (id, type, name, description, location, documentation, inventory_number, responsible_id, status, unavailable_reason, last_verification_date, next_verification_date, created_at, updated_at)
SELECT id, type, name, description, location, documentation, inventory_number, responsible_id, status, unavailable_reason, last_verification_date, next_verification_date, created_at, updated_at
FROM inventory;

DROP TABLE inventory;
ALTER TABLE inventory_new RENAME TO inventory;

CREATE TABLE inventory_photos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    inventory_id INTEGER NOT NULL,
    filename TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    uploaded_by TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inventory_id) REFERENCES inventory(id) ON DELETE CASCADE,
    FOREIGN KEY (uploaded_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_photos_inventory ON inventory_photos(inventory_id);

INSERT INTO inventory_photos (id, inventory_id, filename, stored_name, content_type, size_bytes, uploaded_by, created_at)
SELECT id, inventory_id, filename, stored_name, content_type, size_bytes, uploaded_by, created_at
FROM inventory_photos_copy;

CREATE TABLE inventory_comments (id INTEGER PRIMARY KEY AUTOINCREMENT, inventory_id INTEGER NOT NULL REFERENCES inventory(id) ON DELETE CASCADE, author_id TEXT NOT NULL REFERENCES users(id), body TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);

INSERT INTO inventory_comments (id, inventory_id, author_id, body, created_at)
SELECT id, inventory_id, author_id, body, created_at
FROM inventory_comments_copy;

CREATE TABLE inventory_loans (id INTEGER PRIMARY KEY AUTOINCREMENT, inventory_id INTEGER NOT NULL REFERENCES inventory(id), borrower_id TEXT NOT NULL REFERENCES users(id), issued_by TEXT NOT NULL REFERENCES users(id), comment TEXT NOT NULL DEFAULT '', issued_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, returned_at TEXT);

CREATE UNIQUE INDEX IF NOT EXISTS inventory_active_loan ON inventory_loans(inventory_id) WHERE returned_at IS NULL;

INSERT INTO inventory_loans (id, inventory_id, borrower_id, issued_by, comment, issued_at, returned_at)
SELECT id, inventory_id, borrower_id, issued_by, comment, issued_at, returned_at
FROM inventory_loans_copy;

DROP TABLE inventory_photos_copy;
DROP TABLE inventory_comments_copy;
DROP TABLE inventory_loans_copy;
