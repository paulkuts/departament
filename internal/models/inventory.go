package models

import "time"

type Inventory struct {
	ID                   int64      `json:"id" db:"id"`
	Type                 string     `json:"type" db:"type"`
	Name                 string     `json:"name" db:"name"`
	Description          *string    `json:"description,omitempty" db:"description"`
	Location             *string    `json:"location,omitempty" db:"location"`
	Documentation        *string    `json:"documentation,omitempty" db:"documentation"`
	InventoryNumber      *string    `json:"inventory_number,omitempty" db:"inventory_number"`
	ResponsibleID        *string    `json:"responsible_id,omitempty" db:"responsible_id"`
	Status               bool       `json:"status" db:"status"`
	UnavailableReason    *string    `json:"unavailable_reason,omitempty" db:"unavailable_reason"` // ← новое
	LastVerificationDate *time.Time `json:"last_verification_date,omitempty" db:"last_verification_date"`
	NextVerificationDate *time.Time `json:"next_verification_date,omitempty" db:"next_verification_date"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// InventoryFilter — параметры фильтрации и пагинации для списка оборудования
type InventoryFilter struct {
	Type            *string `form:"type"`       // ✅ ?type=
	Search          *string `form:"search"`     // ✅ ?search=
	InventoryNumber *string `form:"inventory"`  // ✅ ?inventory=
	Status          *bool   `form:"-"`          // ✅ игнорируем (парсится вручную)
	Paginated       Paginated
}

type ListInventory struct {
	PaginatedMetadata PaginatedMetadata `json:"paginated_metadata"`
	Inventory         []Inventory       `json:"inventory"`
}
