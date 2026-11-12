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
	NameNormalized       string     `json:"-" db:"name_normalized"` // наименование для поиска без учёта регистра (SQLite не поднимает кириллицу)
	ResponsibleID        *string    `json:"responsible_id,omitempty" db:"responsible_id"`
	Status               bool       `json:"status" db:"status"`
	UnavailableReason    *string    `json:"unavailable_reason,omitempty" db:"unavailable_reason"` // ← новое

	NoNumberOnItem      bool       `json:"no_number_on_item" db:"no_number_on_item"`   // на приборе номера нет
	NotInRegistry       bool       `json:"not_in_registry" db:"not_in_registry"`       // номера нет в таблице кафедры

	// Общий номер: у нескольких объектов один и тот же канонический номер.
	// Считается на чтении, в базе не хранится.
	NumberSharedCount int            `json:"number_shared_count" db:"-"`
	SharedWith        []SharedNumber `json:"shared_with,omitempty" db:"-"`
	LastVerificationDate *time.Time `json:"last_verification_date,omitempty" db:"last_verification_date"`
	NextVerificationDate *time.Time `json:"next_verification_date,omitempty" db:"next_verification_date"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// SharedNumber — другой объект с тем же инвентарным номером (до 3 в ответе)
type SharedNumber struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Location *string `json:"location,omitempty"`
}

// InventoryNumberIndex — плоский список номеров всех объектов: по нему ищутся общие номера
type InventoryNumberIndex struct {
	ID              int64   `db:"id"`
	Name            string  `db:"name"`
	Location        *string `db:"location"`
	InventoryNumber *string `db:"inventory_number"`
}

// InventoryFilter — параметры фильтрации и пагинации для списка оборудования
type InventoryFilter struct {
	Type            *string `form:"type"`       // ✅ ?type=
	Search          *string `form:"search"`     // ✅ ?search=
	InventoryNumber *string `form:"inventory"`  // ✅ ?inventory=
	Status          *bool   `form:"-"`          // ✅ игнорируем (парсится вручную)
	Group           bool    `form:"group"`      // ✅ ?group=1 — вернуть группы, а не плоский список
	Sort            string  `form:"sort"`       // ?sort=name|type|number|marks|location|verification|status
	Dir             string  `form:"dir"`        // ?dir=asc|desc
	Paginated       Paginated
}

// InventoryGroup — строка реестра: несколько единиц с общим номером и/или названием.
// Считается на чтении, в базе не хранится.
type InventoryGroup struct {
	Key                string
	Title              string // название самой ранней записи группы
	InventoryNumber    *string // номер, если он общий у всех единиц; иначе nil
	HasNumbers         bool    // хотя бы у кого-то есть номер
	NumbersDistinct    bool    // у единиц разные номера
	Count              int
	Types              []string
	Locations          []string
	NextVerifications  []string // уникальные даты следующей поверки (YYYY-MM-DD)
	AllAvailable       bool
	NoNumberCount      int
	NotInRegistryCount int
	Members            []Inventory
}

type ListInventory struct {
	PaginatedMetadata PaginatedMetadata `json:"paginated_metadata"`
	Inventory         []Inventory       `json:"inventory"`
	Groups            []InventoryGroup  `json:"-"`
	ObjectsTotal      int64             `json:"-"`
}
