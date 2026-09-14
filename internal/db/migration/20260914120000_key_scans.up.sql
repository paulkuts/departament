-- Ключ может быть выдан гостю без аккаунта: user_id становится необязательным,
-- появляются поля гостя и токен его браузера (для повторного скана = сдача).
-- SQLite не умеет ALTER COLUMN, поэтому таблица пересоздаётся целиком;
-- defer_foreign_keys откладывает проверку внешних ключей до конца транзакции.
PRAGMA defer_foreign_keys=ON;

CREATE TABLE key_logs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id INTEGER NOT NULL,
    user_id TEXT,
    action_type TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    comment TEXT,
    guest_name TEXT,
    guest_phone TEXT,
    guest_token TEXT,
    FOREIGN KEY (key_id) REFERENCES keys(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

INSERT INTO key_logs_new (id, key_id, user_id, action_type, timestamp, comment)
    SELECT id, key_id, user_id, action_type, timestamp, comment FROM key_logs;

DROP TABLE key_logs;

ALTER TABLE key_logs_new RENAME TO key_logs;

CREATE INDEX IF NOT EXISTS idx_key_logs_key_id ON key_logs(key_id);
CREATE INDEX IF NOT EXISTS idx_key_logs_user_id ON key_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_key_logs_guest_token ON key_logs(guest_token);
