// internal/models/inventory_photo.go

package models

import "time"

type InventoryPhoto struct {
	ID          int64     `json:"id" db:"id"`
	InventoryID int64     `json:"inventory_id" db:"inventory_id"`
	Filename    string    `json:"filename" db:"filename"`
	StoredName  string    `json:"-" db:"stored_name"` // не отдаём клиенту
	ContentType string    `json:"content_type" db:"content_type"`
	SizeBytes   int64     `json:"size_bytes" db:"size_bytes"`
	UploadedBy  *string   `json:"uploaded_by,omitempty" db:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}
