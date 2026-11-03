-- Инвентарный номер может повторяться: одна учётная единица на несколько
-- приборов (комплект: системный блок и монитор под одним номером).
--
-- В исходной таблице inventory.inventory_number объявлен UNIQUE, а SQLite не
-- позволяет снять ограничение у существующей колонки — нужна пересборка таблицы.
-- Порядок из граблей проекта: копии дочерних таблиц -> удалить дочерние ->
-- новая таблица без UNIQUE -> перенос строк -> удалить старую -> переименовать ->
-- воссоздать дочерние точным DDL с боевой базы -> вернуть строки.

-- 1. Копии дочерних таблиц (удаление inventory иначе каскадом унесло бы фото)
CREATE TABLE inventory_photos_copy AS SELECT * FROM inventory_photos;
CREATE TABLE inventory_comments_copy AS SELECT * FROM inventory_comments;
CREATE TABLE inventory_loans_copy AS SELECT * FROM inventory_loans;

DROP TABLE inventory_photos;
DROP TABLE inventory_comments;
DROP TABLE inventory_loans;

-- 2. Новая таблица: схема та же, UNIQUE у inventory_number снят
CREATE TABLE inventory_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL CHECK (type IN ('equipment', 'inventory', 'raw_material', 'other')),
    name TEXT NOT NULL,
    description TEXT,
    location TEXT NOT NULL,
    documentation TEXT,
    inventory_number TEXT,
    responsible_id TEXT,
    status BOOLEAN DEFAULT 1,
    unavailable_reason TEXT,
    last_verification_date DATE,
    next_verification_date DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    no_number_on_item BOOLEAN NOT NULL DEFAULT 0,
    not_in_registry BOOLEAN NOT NULL DEFAULT 0,
    name_normalized TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (responsible_id) REFERENCES users(id)
);

-- 3. Перенос строк (явный список колонок: порядок в старой таблице другой)
INSERT INTO inventory_new (
    id, type, name, description, location, documentation, inventory_number,
    responsible_id, status, unavailable_reason, last_verification_date,
    next_verification_date, created_at, updated_at, no_number_on_item,
    not_in_registry, name_normalized
)
SELECT
    id, type, name, description, location, documentation, inventory_number,
    responsible_id, status, unavailable_reason, last_verification_date,
    next_verification_date, created_at, updated_at, no_number_on_item,
    not_in_registry, name_normalized
FROM inventory;

DROP TABLE inventory;
ALTER TABLE inventory_new RENAME TO inventory;

-- 4. Дочерние таблицы — точный DDL с боевой базы
CREATE TABLE inventory_photos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    inventory_id INTEGER NOT NULL,
    filename TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    uploaded_by TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, thumb_name TEXT NOT NULL DEFAULT '', width INTEGER NOT NULL DEFAULT 0, height INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (inventory_id) REFERENCES inventory(id) ON DELETE CASCADE,
    FOREIGN KEY (uploaded_by) REFERENCES users(id)
);
CREATE INDEX idx_photos_inventory ON inventory_photos(inventory_id);

CREATE TABLE inventory_comments (id INTEGER PRIMARY KEY AUTOINCREMENT, inventory_id INTEGER NOT NULL REFERENCES inventory(id) ON DELETE CASCADE, author_id TEXT NOT NULL REFERENCES users(id), body TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);

CREATE TABLE inventory_loans (id INTEGER PRIMARY KEY AUTOINCREMENT, inventory_id INTEGER NOT NULL REFERENCES inventory(id), borrower_id TEXT NOT NULL REFERENCES users(id), issued_by TEXT NOT NULL REFERENCES users(id), comment TEXT NOT NULL DEFAULT '', issued_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, returned_at TEXT);
CREATE UNIQUE INDEX inventory_active_loan ON inventory_loans(inventory_id) WHERE returned_at IS NULL;

-- 5. Возврат строк дочерних таблиц
INSERT INTO inventory_photos SELECT * FROM inventory_photos_copy;
INSERT INTO inventory_comments SELECT * FROM inventory_comments_copy;
INSERT INTO inventory_loans SELECT * FROM inventory_loans_copy;

DROP TABLE inventory_photos_copy;
DROP TABLE inventory_comments_copy;
DROP TABLE inventory_loans_copy;

-- 6. Индекс поиска по наименованию (в старой таблице он был)
CREATE INDEX idx_inventory_name_normalized ON inventory(name_normalized);
