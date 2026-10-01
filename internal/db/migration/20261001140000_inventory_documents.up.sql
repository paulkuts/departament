-- 20261001140000_inventory_documents.up.sql
-- Инвентаризация: архивные документы материального отдела (таблицы, ведомости, сканы).
--
-- Документы приложением не разбираются: они лежат списком в разделе
-- «Инвентаризация», их можно скачать и удалить. Метаданные — в таблице,
-- сам файл — в каталоге data/documents (имя на диске случайное).
-- Раздел и файлы доступны только администраторам.

CREATE TABLE IF NOT EXISTS inventory_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,          -- исходное имя файла, как его видит человек
    stored_name TEXT NOT NULL,       -- имя файла на диске (uuid + расширение)
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    uploaded_by TEXT,                -- кто загрузил
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (uploaded_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_documents_created ON inventory_documents(created_at);
