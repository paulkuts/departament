-- 20261002120000_inventory_name_normalized.up.sql
-- Реестр имущества: поиск одним полем — по наименованию и по инвентарному номеру
-- (как в описи материального отдела). SQLite не поднимает регистр кириллицы
-- (LIKE и UPPER работают только с латиницей), поэтому нормализованное
-- наименование храним отдельной колонкой — как name_normalized у описи.
-- Значение пишет приложение: при создании и правке объекта, а для строк,
-- заведённых раньше, — при первом запуске (repository.InventoryRepo.BackfillNames).

ALTER TABLE inventory ADD COLUMN name_normalized TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_inventory_name_normalized ON inventory(name_normalized);
