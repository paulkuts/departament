package models

// InventoryWriteoff — отметка позиции описи на списание.
// Пока status = draft, опись не меняется: администратор может скачать отчёт,
// распечатать и подписать его. После «Применить списание» отметка становится
// историей (applied), а строка описи корректируется.
// Поля позиции (наименование, номер, количество) подставляются из описи:
// вкладка «Списание» показывает те же колонки, что и опись.
type InventoryWriteoff struct {
	ID       int64   `json:"id" db:"id"`
	ItemID   int64   `json:"item_id" db:"item_id"`
	Quantity float64 `json:"quantity" db:"quantity"`
	Reason   string  `json:"reason" db:"reason"`
	Status   string  `json:"status" db:"status"`

	CreatedBy *string `json:"created_by,omitempty" db:"created_by"`
	CreatedAt string  `json:"created_at" db:"created_at"`
	AppliedBy *string `json:"applied_by,omitempty" db:"applied_by"`
	AppliedAt *string `json:"applied_at,omitempty" db:"applied_at"`

	// Позиция описи
	Name           string   `json:"name" db:"name"`
	Number         string   `json:"number" db:"number"`
	Unit           string   `json:"unit" db:"unit"`
	DocumentNumber string   `json:"document_number" db:"document_number"`
	ItemQuantity   *float64 `json:"item_quantity" db:"item_quantity"`
	Price          *float64 `json:"price" db:"price"`
}

// WriteoffResult — итог применения списаний: сколько позиций, сколько единиц
// и на какую сумму ушло из описи. Показывается администратору после нажатия
// «Применить списание».
type WriteoffResult struct {
	Items    int64   `json:"items" db:"items"`
	Quantity float64 `json:"quantity" db:"quantity"`
	Amount   float64 `json:"amount" db:"amount"`
}
