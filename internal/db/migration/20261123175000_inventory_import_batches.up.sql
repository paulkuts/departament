-- 20261123175000_inventory_import_batches.up.sql
-- Инвентаризация: массовая вставка строк из Excel («Добавить таблицей»).
--
-- Две новые колонки описи — из таблицы материального отдела: дата принятия
-- к учёту и износ в процентах. Цены и суммы не возвращаются.
--
-- Пакет импорта помечается меткой: в inventory_import_batches лежит сводка,
-- в inventory_import_rows — что именно сделал пакет (добавил строку или
-- обновил существующую) и снимок прежних значений для обновлённых. Это даёт
-- откат одной командой: удалить добавленные строки, вернуть прежние значения.
-- Внешнего ключа на inventory_numbers у item_id намеренно нет: история пакета
-- должна переживать удаление строки, откат сам решает, что можно снять.

ALTER TABLE inventory_numbers ADD COLUMN accepted_at TEXT NOT NULL DEFAULT '';
ALTER TABLE inventory_numbers ADD COLUMN wear_percent REAL;

CREATE TABLE IF NOT EXISTS inventory_import_batches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    label TEXT NOT NULL DEFAULT '',
    document_number TEXT NOT NULL DEFAULT '',
    added INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS inventory_import_rows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id INTEGER NOT NULL,
    item_id INTEGER NOT NULL,
    action TEXT NOT NULL DEFAULT 'added',
    before_json TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (batch_id) REFERENCES inventory_import_batches(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_inventory_import_rows_batch ON inventory_import_rows(batch_id);
