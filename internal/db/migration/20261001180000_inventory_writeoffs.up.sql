-- 20261001180000_inventory_writeoffs.up.sql
-- Инвентаризация: списание позиций описи (расход материальных запасов).
--
-- Порядок работы: администратор отмечает позицию галочкой «Расх.» в описи,
-- заполняет количество и причину списания, скачивает отчёт по форме
-- материального отдела, подписывает его и жмёт «Применить списание» — только
-- после этого опись корректируется: количество уменьшается, а полностью
-- израсходованная строка уходит из списка.
--
-- Отметка живёт в таблице (а не в браузере): открыв раздел с другого
-- устройства, администратор видит те же отмеченные позиции.
-- status: draft — отмечено, applied — списание применено (история).

ALTER TABLE inventory_numbers ADD COLUMN written_off REAL NOT NULL DEFAULT 0;
ALTER TABLE inventory_numbers ADD COLUMN is_written_off INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS inventory_writeoffs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER NOT NULL,                     -- строка описи (inventory_numbers.id)
    quantity REAL NOT NULL DEFAULT 0,             -- сколько списать
    reason TEXT NOT NULL DEFAULT '',              -- причина списания
    status TEXT NOT NULL DEFAULT 'draft',         -- draft | applied
    created_by TEXT,                              -- кто отметил
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    applied_by TEXT,                              -- кто применил списание
    applied_at TIMESTAMP,
    FOREIGN KEY (item_id) REFERENCES inventory_numbers(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id),
    FOREIGN KEY (applied_by) REFERENCES users(id)
);

-- Отметка у позиции одна: повторная галочка обновляет количество и причину.
CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_writeoffs_draft
    ON inventory_writeoffs(item_id) WHERE status = 'draft';
CREATE INDEX IF NOT EXISTS idx_inventory_writeoffs_item ON inventory_writeoffs(item_id);
CREATE INDEX IF NOT EXISTS idx_inventory_numbers_written_off ON inventory_numbers(is_written_off);
