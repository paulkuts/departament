package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mitm-departament/internal/models"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// InventoryNumberRepo — справочник инвентарных номеров кафедры (таблица учёта).
type InventoryNumberRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewInventoryNumberRepo(db *sqlx.DB, log *zap.Logger) *InventoryNumberRepo {
	return &InventoryNumberRepo{db: db, log: log}
}

const inventoryNumberColumns = `id, number, normalized, name, name_normalized, source, document_number, unit, quantity, price, amount, created_at`

// Search отдаёт строки описи: страница инвентаризации и подсказки в форме
// объекта. Пустой запрос — весь список (страница грузит опись целиком).
// sort — порядок вывода: «duplicates» (рядом позиции с повторяющимся номером),
// «name» (по алфавиту), «price» (сначала дорогие); пустой — как в документах.
func (r *InventoryNumberRepo) Search(ctx context.Context, query, sort string, limit int) ([]models.InventoryNumber, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	items := []models.InventoryNumber{}
	where, args := inventoryNumberFilter(query)
	args = append(args, limit)
	if err := r.db.SelectContext(ctx, &items,
		`SELECT `+inventoryNumberColumns+` FROM inventory_numbers `+where+
			` ORDER BY `+inventoryNumberOrder(sort)+` LIMIT ?`, args...); err != nil {
		return nil, fmt.Errorf("search inventory numbers: %w", err)
	}
	return items, nil
}

// inventoryNumberFilter — условие поиска по описи: подстрока инвентарного
// номера или наименования. Список и итог используют одно и то же условие,
// иначе итог разошёлся бы с тем, что человек видит в таблице.
// Полностью списанные позиции в опись не попадают: они ушли из списка, но
// остались в базе — с историей списания и суммой.
func inventoryNumberFilter(query string) (string, []interface{}) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "WHERE is_written_off = 0", nil
	}
	upper := strings.ToUpper(trimmed)
	return `WHERE is_written_off = 0 AND (normalized LIKE ? OR UPPER(number) LIKE ? OR name_normalized LIKE ?)`,
		[]interface{}{"%" + models.NormalizeInventoryNumber(trimmed) + "%", "%" + upper + "%", "%" + upper + "%"}
}

// inventoryNumberOrder — порядок строк описи. «По дублям» поднимает наверх
// позиции, которые встречаются в документах несколько раз: строки с одним
// номером оказываются рядом.
func inventoryNumberOrder(sort string) string {
	switch sort {
	case "duplicates":
		return `(SELECT COUNT(*) FROM inventory_numbers d WHERE d.normalized = inventory_numbers.normalized) DESC, normalized, name_normalized, id`
	case "name":
		return `name_normalized, number, id`
	case "price":
		return `price IS NULL, price DESC, name_normalized, id`
	default:
		return `id`
	}
}

// Summary — итог по описи: строк, единиц и рублей. Учитывает тот же поиск, что
// и список, и складывает суммы строк (они уже округлены до копеек), поэтому
// итог сходится с тем, что видно в колонке «Сумма». Строки без цены в денежный
// итог не попадают — считать нечего.
func (r *InventoryNumberRepo) Summary(ctx context.Context, query string) (models.InventorySummary, error) {
	where, args := inventoryNumberFilter(query)
	var out models.InventorySummary
	if err := r.db.GetContext(ctx, &out,
		`SELECT COUNT(*) AS count, COALESCE(SUM(quantity), 0) AS quantity, COALESCE(SUM(amount), 0) AS amount
		 FROM inventory_numbers `+where, args...); err != nil {
		return models.InventorySummary{}, fmt.Errorf("summarize inventory numbers: %w", err)
	}
	return out, nil
}

// Count — сколько номеров в справочнике (пустой справочник = сверять не с чем).
func (r *InventoryNumberRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM inventory_numbers`); err != nil {
		return 0, fmt.Errorf("count inventory numbers: %w", err)
	}
	return n, nil
}

// Exists проверяет, есть ли номер в справочнике (с учётом вариантов записи).
func (r *InventoryNumberRepo) Exists(ctx context.Context, number string) (bool, error) {
	canonical := models.CanonicalInventoryNumber(number)
	if canonical == "" {
		return false, nil
	}
	var n int
	if err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM inventory_numbers WHERE normalized = ?`, canonical); err != nil {
		return false, fmt.Errorf("lookup inventory number: %w", err)
	}
	return n > 0, nil
}

// NormalizeName приводит наименование к виду для поиска: верхний регистр, без
// лишних пробелов. Нужен свой (а не SQL UPPER), потому что SQLite не поднимает
// регистр кириллицы.
func NormalizeName(name string) string {
	return strings.ToUpper(strings.Join(strings.Fields(name), " "))
}

// GetByNumber отдаёт строку описи по инвентарному номеру: по ней карточка
// объекта показывает запись инвентаризации. Сравнение — в каноническом виде,
// «025» и «25» считаются одним номером. Номеров-дублей может быть несколько
// (список из документов), поэтому берём первую строку. Нет записи — (nil, nil).
func (r *InventoryNumberRepo) GetByNumber(ctx context.Context, number string) (*models.InventoryNumber, error) {
	canonical := models.CanonicalInventoryNumber(number)
	if canonical == "" {
		return nil, nil
	}
	var item models.InventoryNumber
	err := r.db.GetContext(ctx, &item,
		`SELECT `+inventoryNumberColumns+` FROM inventory_numbers WHERE normalized = ? ORDER BY id LIMIT 1`, canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup inventory number %q: %w", number, err)
	}
	return &item, nil
}

// Create добавляет строку в опись материального отдела.
// Номер-дубль разрешён: опись собирается из документов заявок, где одна и та же
// позиция встречается в нескольких заявках.
func (r *InventoryNumberRepo) Create(ctx context.Context, item *models.InventoryNumber) error {
	normalized := models.CanonicalInventoryNumber(item.Number)
	source := strings.TrimSpace(item.Source)
	if source == "" {
		source = "материальный отдел"
	}
	item.NameNormalized = NormalizeName(item.Name)
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO inventory_numbers (number, normalized, name, name_normalized, source, document_number, unit, quantity, price, amount)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(item.Number), normalized, strings.TrimSpace(item.Name), item.NameNormalized, source,
		strings.TrimSpace(item.DocumentNumber), strings.TrimSpace(item.Unit), item.Quantity, item.Price, item.Amount)
	if err != nil {
		return fmt.Errorf("create inventory number %q: %w", item.Number, err)
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		item.ID = id
	}
	item.Normalized, item.Source = normalized, source
	return nil
}

// Update правит строку описи; инвентарный номер тоже может измениться.
func (r *InventoryNumberRepo) Update(ctx context.Context, item *models.InventoryNumber) error {
	normalized := models.CanonicalInventoryNumber(item.Number)
	item.NameNormalized = NormalizeName(item.Name)
	res, err := r.db.ExecContext(ctx,
		`UPDATE inventory_numbers SET number = ?, normalized = ?, name = ?, name_normalized = ?, document_number = ?, unit = ?, quantity = ?, price = ?, amount = ?
		 WHERE id = ?`,
		strings.TrimSpace(item.Number), normalized, strings.TrimSpace(item.Name), item.NameNormalized,
		strings.TrimSpace(item.DocumentNumber), strings.TrimSpace(item.Unit), item.Quantity, item.Price, item.Amount, item.ID)
	if err != nil {
		return fmt.Errorf("update inventory number %d: %w", item.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("inventory number not found")
	}
	item.Normalized = normalized
	return nil
}

// Import загружает таблицу номеров: новые строки добавляются, существующие
// обновляются по нормализованному номеру и наименованию (номер может
// повторяться в разных заявках, поэтому одного номера для поиска строки мало).
// Загружаются и данные описи: единица измерения, количество, цена, сумма.
// replace очищает опись перед загрузкой.
func (r *InventoryNumberRepo) Import(ctx context.Context, items []models.InventoryNumber, replace bool) (added, updated int, err error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin registry import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if replace {
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_numbers`); err != nil {
			return 0, 0, fmt.Errorf("clear registry: %w", err)
		}
	}

	skipped := 0
	for _, it := range items {
		normalized := models.CanonicalInventoryNumber(it.Number)
		if normalized == "" {
			skipped++
			continue
		}
		number := strings.TrimSpace(it.Number)
		name := strings.TrimSpace(it.Name)
		nameNormalized := NormalizeName(name)
		source := strings.TrimSpace(it.Source)
		if source == "" {
			source = "таблица кафедры"
		}

		var id int64
		lookupErr := tx.GetContext(ctx, &id,
			`SELECT id FROM inventory_numbers WHERE normalized = ? AND name_normalized = ? ORDER BY id LIMIT 1`,
			normalized, nameNormalized)
		switch {
		case errors.Is(lookupErr, sql.ErrNoRows):
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO inventory_numbers (number, normalized, name, name_normalized, source, document_number, unit, quantity, price, amount)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				number, normalized, name, nameNormalized, source,
				strings.TrimSpace(it.DocumentNumber), strings.TrimSpace(it.Unit), it.Quantity, it.Price, it.Amount); err != nil {
				return 0, 0, fmt.Errorf("insert inventory number %q: %w", it.Number, err)
			}
			added++
		case lookupErr != nil:
			return 0, 0, fmt.Errorf("lookup inventory number %q: %w", it.Number, lookupErr)
		default:
			if _, err = tx.ExecContext(ctx,
				`UPDATE inventory_numbers SET number = ?, name = ?, name_normalized = ?, document_number = ?, unit = ?, quantity = ?, price = ?, amount = ?
				 WHERE id = ?`,
				number, name, nameNormalized, strings.TrimSpace(it.DocumentNumber),
				strings.TrimSpace(it.Unit), it.Quantity, it.Price, it.Amount, id); err != nil {
				return 0, 0, fmt.Errorf("update inventory number %q: %w", it.Number, err)
			}
			updated++
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit registry import: %w", err)
	}
	if skipped > 0 {
		r.log.Warn("inventory numbers import: пустые номера пропущены", zap.Int("skipped", skipped))
	}
	return added, updated, nil
}

// Delete удаляет номер из справочника.
func (r *InventoryNumberRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM inventory_numbers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete inventory number: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("inventory number not found")
	}
	return nil
}
