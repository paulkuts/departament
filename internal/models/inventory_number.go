package models

import "strings"

// InventoryNumber — строка справочника инвентарных номеров кафедры
// (таблица учёта, из которой сверяется номер, введённый в карточке объекта).
type InventoryNumber struct {
	ID         int64  `json:"id" db:"id"`
	Number     string `json:"number" db:"number"`
	Normalized string `json:"-" db:"normalized"`
	Name       string `json:"name" db:"name"`
	Source     string `json:"source" db:"source"`
	CreatedAt  string `json:"created_at" db:"created_at"`
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
