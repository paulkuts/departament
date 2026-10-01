package models

import "strings"

// InventoryNumber — строка инвентаризации: опись материального отдела.
// По ней сверяется номер, введённый в карточке объекта (пометка «нет в таблице»),
// а в карточке оборудования администратор видит саму запись описи.
// Данные конфиденциальные — API раздела доступен только администраторам.
// name_normalized — служебное поле поиска: наименование в верхнем регистре
// (SQLite не поднимает регистр кириллицы, поэтому его приводит приложение).
type InventoryNumber struct {
	ID             int64    `json:"id" db:"id"`
	Number         string   `json:"number" db:"number"`
	Normalized     string   `json:"-" db:"normalized"`
	Name           string   `json:"name" db:"name"`
	NameNormalized string   `json:"-" db:"name_normalized"`
	Source         string   `json:"source" db:"source"`
	Unit           string   `json:"unit" db:"unit"`
	Quantity       *float64 `json:"quantity" db:"quantity"`
	Price          *float64 `json:"price" db:"price"`
	Amount         *float64 `json:"amount" db:"amount"`
	CreatedAt      string   `json:"created_at" db:"created_at"`
}

// InventorySummary — итог по описи: сколько строк, сколько всего единиц и на
// какую сумму. Считается сервером по тому же поиску, что и список, поэтому
// после правки строки итог пересчитывается сам.
type InventorySummary struct {
	Count    int64   `json:"count" db:"count"`
	Quantity float64 `json:"quantity" db:"quantity"`
	Amount   float64 `json:"amount" db:"amount"`
}

// NormalizeInventoryNumber приводит номер к сравнимому виду: верхний регистр,
// без пробелов, дефисов, точек, подчёркиваний и знака «№».
// Слэш сохраняется — в таблице есть номера вида «026/025».
func NormalizeInventoryNumber(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch {
		case r >= '0' && r <= '9',
			r >= 'A' && r <= 'Z',
			r >= 'А' && r <= 'Я',
			r == 'Ё',
			r == '/':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CanonicalInventoryNumber — вид номера, по которому идёт сверка: как
// NormalizeInventoryNumber, но ещё и без ведущих нулей. «025» и «25» — один и
// тот же номер: так пишут в таблице учёта и так на наклейке прибора.
// Сверка сравнивает канонические виды обеих сторон (сам номер хранится как введён).
func CanonicalInventoryNumber(s string) string {
	n := NormalizeInventoryNumber(s)
	if n == "" {
		return ""
	}
	if trimmed := strings.TrimLeft(n, "0"); trimmed != "" {
		return trimmed
	}
	return "0"
}
