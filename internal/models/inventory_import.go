package models

// InventoryImportBatch — пакет массовой вставки строк описи из таблицы
// материального отдела. Метка нужна, чтобы одной командой откатить именно
// этот пакет: см. inventory_import_rows.
type InventoryImportBatch struct {
	ID             int64  `json:"id" db:"id"`
	Label          string `json:"label" db:"label"`
	DocumentNumber string `json:"document_number" db:"document_number"`
	Added          int    `json:"added" db:"added"`
	Updated        int    `json:"updated" db:"updated"`
	CreatedBy      string `json:"created_by" db:"created_by"`
	CreatedAt      string `json:"created_at" db:"created_at"`
}

// InventoryImportProblem — строка, которую пакет не принял, с причиной.
// Row — номер строки во вставке (заголовок не считается).
type InventoryImportProblem struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
	Number string `json:"number,omitempty"`
	Name   string `json:"name,omitempty"`
}

// InventoryImportReport — отчёт пакета: что принято, что пропущено и почему.
// BatchID = 0, если ни одна строка не прошла проверку (пакет не создан).
type InventoryImportReport struct {
	BatchID    int64                    `json:"batch_id"`
	Added      int                      `json:"added"`
	Updated    int                      `json:"updated"`
	Skipped    int                      `json:"skipped"`
	Duplicates int                      `json:"duplicates"`
	Problems   []InventoryImportProblem `json:"problems"`
}

// InventoryImportRollback — итог отката пакета: удалено добавленных,
// восстановлено обновлённых, оставлено (строку уже списали — историю не рвём).
type InventoryImportRollback struct {
	Removed  int `json:"removed"`
	Restored int `json:"restored"`
	Kept     int `json:"kept"`
}
