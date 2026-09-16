-- 20260916160000_inventory_registry.up.sql
-- Справочник инвентарных номеров кафедры (таблица учёта) и две пометки у объектов.
--
--   1) inventory_numbers — номера из таблицы: оригинал, нормализованный вид для
--      сверки (регистр, пробелы, дефисы и знак № не важны) и наименование.
--   2) inventory.no_number_on_item — на самом приборе номера нет.
--   3) inventory.not_in_registry  — введённого номера нет в таблице; ставится
--      автоматически при сохранении, вручную галочку можно снять.
--
-- Обе колонки добавляются через ALTER TABLE ADD COLUMN — пересборка таблицы не нужна.

CREATE TABLE IF NOT EXISTS inventory_numbers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    number TEXT NOT NULL,
    normalized TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'таблица кафедры',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE inventory ADD COLUMN no_number_on_item BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE inventory ADD COLUMN not_in_registry BOOLEAN NOT NULL DEFAULT 0;
