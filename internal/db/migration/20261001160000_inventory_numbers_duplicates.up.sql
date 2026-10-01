-- 20261001160000_inventory_numbers_duplicates.up.sql
-- Опись материального отдела — это список из документов заявок: одна и та же
-- позиция приходит в разных заявках, и её строки должны остаться отдельными
-- (решение Павла: «пусть дублируется, это же список из документов»).
-- Поэтому уникальность инвентарного номера снимается.
--
-- По номеру по-прежнему идёт сверка с объектами реестра и подсказки в форме,
-- но строк с одним номером может быть несколько: Exists считает количество,
-- GetByNumber берёт первую.
--
-- SQLite не позволяет удалить автоиндекс UNIQUE-колонки (DROP INDEX для
-- sqlite_autoindex_* не работает), поэтому таблица пересобирается.

CREATE TABLE inventory_numbers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    number TEXT NOT NULL,
    normalized TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'таблица кафедры',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    name_normalized TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    quantity REAL,
    price REAL,
    amount REAL
);

INSERT INTO inventory_numbers_new (id, number, normalized, name, source, created_at, name_normalized, unit, quantity, price, amount)
    SELECT id, number, normalized, name, source, created_at, name_normalized, unit, quantity, price, amount
    FROM inventory_numbers;

DROP TABLE inventory_numbers;

ALTER TABLE inventory_numbers_new RENAME TO inventory_numbers;

CREATE INDEX IF NOT EXISTS idx_inventory_numbers_normalized ON inventory_numbers (normalized);
CREATE INDEX IF NOT EXISTS idx_inventory_numbers_name ON inventory_numbers (name_normalized);
