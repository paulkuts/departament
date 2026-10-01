-- 20261001120000_inventory_registry_details.up.sql
-- Инвентаризация: справочник инвентарных номеров становится описью материального отдела.
--
-- К номеру и наименованию добавляются поля официального учёта:
--   unit     — единица измерения («шт», «кг», «м») — как в таблице;
--   quantity — количество (может быть дробным: 0,5 кг);
--   price    — цена за единицу;
--   amount   — сумма по строке.
--
-- Цена и сумма пока не заполняются: поля заведены заранее, чтобы не менять схему
-- при загрузке таблицы материального отдела. Все четыре колонки добавляются через
-- ALTER TABLE ADD COLUMN — пересборка таблицы не нужна.
--
-- Сами данные раздела конфиденциальны: доступ к API инвентаризации — только администраторам.

-- name_normalized — наименование в верхнем регистре для поиска: SQLite UPPER() и LIKE
-- не поднимают регистр кириллицы, поэтому регистр приводит приложение (Go/Python).
ALTER TABLE inventory_numbers ADD COLUMN name_normalized TEXT NOT NULL DEFAULT '';
UPDATE inventory_numbers SET name_normalized = UPPER(name);
ALTER TABLE inventory_numbers ADD COLUMN unit TEXT NOT NULL DEFAULT '';
ALTER TABLE inventory_numbers ADD COLUMN quantity REAL;
ALTER TABLE inventory_numbers ADD COLUMN price REAL;
ALTER TABLE inventory_numbers ADD COLUMN amount REAL;
