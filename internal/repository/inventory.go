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

type InventoryRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewInventoryRepo(db *sqlx.DB, log *zap.Logger) *InventoryRepo {
	return &InventoryRepo{db: db, log: log}
}

// inventoryColumns — список колонок для SELECT (без JOIN)
const inventoryColumns = `id, type, name, description, location, documentation, inventory_number,
	responsible_id, no_number_on_item, not_in_registry, status, unavailable_reason, last_verification_date, next_verification_date, created_at, updated_at`

// Create создаёт единицу оборудования
func (r *InventoryRepo) Create(ctx context.Context, e *models.Inventory) error {
	e.NameNormalized = NormalizeName(e.Name)
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO inventory 
			(name, name_normalized, type, description, location, documentation, inventory_number, responsible_id, no_number_on_item, not_in_registry, status, unavailable_reason, last_verification_date, next_verification_date)
		 VALUES 
			(:name, :name_normalized, :type, :description, :location, :documentation, :inventory_number, :responsible_id, :no_number_on_item, :not_in_registry, :status, :unavailable_reason, :last_verification_date, :next_verification_date)`, e)
	if err != nil {
		return fmt.Errorf("insert inventory: %w", err)
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return nil
}

// GetByID возвращает оборудование по ID
func (r *InventoryRepo) GetByID(ctx context.Context, id int64) (*models.Inventory, error) {
	e := &models.Inventory{}
	err := r.db.GetContext(ctx, e,
		`SELECT `+inventoryColumns+` FROM inventory WHERE id = ?`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory: %w", err)
	}
	return e, nil
}

// GetByInventoryNumber возвращает оборудование по точному инвентарному номеру
func (r *InventoryRepo) GetByInventoryNumber(ctx context.Context, invNumber string) (*models.Inventory, error) {
	e := &models.Inventory{}
	err := r.db.GetContext(ctx, e,
		`SELECT `+inventoryColumns+` FROM inventory WHERE inventory_number = ?`, invNumber)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory by inventory number: %w", err)
	}
	return e, nil
}

// List возвращает список оборудования с фильтрами, пагинацией и общим количеством.
//   - Search:          поиск по наименованию и инвентарному номеру (как в описи)
//   - InventoryNumber: поиск по инвентарному номеру (LIKE inventory%)
//   - Status:          фильтр по статусу (nil = без фильтра)
//   - Limit / Offset:  пагинация
func (r *InventoryRepo) List(ctx context.Context, f models.InventoryFilter) ([]models.Inventory, int64, error) {
	// Динамически собираем условия WHERE
	var conditions []string
	var args []interface{}

	if f.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *f.Type)
	}

	if f.Search != nil {
		// Одно поле ищет и наименование, и инвентарный номер — как в описи
		// материального отдела. Наименование сравниваем в нормализованном виде:
		// SQLite не поднимает регистр кириллицы, иначе «стол» не нашёл бы «Стол».
		// Номер — в верхнем регистре (запрос поднимает Go, а не SQLite).
		conditions = append(conditions, "(name_normalized LIKE ? OR UPPER(inventory_number) LIKE ?)")
		args = append(args, "%"+NormalizeName(*f.Search)+"%", "%"+strings.ToUpper(*f.Search)+"%") // содержит
	}

	if f.InventoryNumber != nil {
		conditions = append(conditions, "inventory_number LIKE ?")
		args = append(args, "%"+*f.InventoryNumber+"%") // содержит
	}

	if f.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *f.Status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 1. Общее количество записей под фильтр
	var total int64
	countQuery := `SELECT COUNT(*) FROM inventory` + whereClause
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count inventory: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	// 2. Сами записи с пагинацией
	listArgs := make([]interface{}, 0, len(args)+2)
	listArgs = append(listArgs, args...)
	listArgs = append(listArgs, f.Paginated.Limit, f.Paginated.Offset)

	var items []models.Inventory
	listQuery := `SELECT ` + inventoryColumns + ` FROM inventory` + whereClause + ` ORDER BY id LIMIT ? OFFSET ?`
	if err := r.db.SelectContext(ctx, &items, listQuery, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list inventory: %w", err)
	}

	return items, total, nil
}

// ListExpiredVerification возвращает оборудование с просроченной поверкой
func (r *InventoryRepo) ListExpiredVerification(ctx context.Context, limit, offset int64) ([]models.Inventory, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM inventory WHERE next_verification_date IS NOT NULL AND next_verification_date < DATE('now')`
	if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, fmt.Errorf("count inventory: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	var items []models.Inventory
	err := r.db.SelectContext(ctx, &items,
		`SELECT `+inventoryColumns+` FROM inventory 
		 WHERE next_verification_date IS NOT NULL AND next_verification_date < DATE('now')
		 ORDER BY next_verification_date`)
	if err != nil {
		return nil, 0, fmt.Errorf("list expired verification: %w", err)
	}
	return items, total, nil
}

// Update обновляет данные оборудования
func (r *InventoryRepo) Update(ctx context.Context, e *models.Inventory) error {
	e.NameNormalized = NormalizeName(e.Name)
	res, err := r.db.NamedExecContext(ctx,
		`UPDATE inventory SET 
			name = :name,
			name_normalized = :name_normalized,
			type = :type,
			description = :description,
			location = :location,
			documentation = :documentation,
			inventory_number = :inventory_number,
			responsible_id = :responsible_id,
			no_number_on_item = :no_number_on_item,
			not_in_registry = :not_in_registry,
			status = :status,
			unavailable_reason = :unavailable_reason,
			last_verification_date = :last_verification_date,
			next_verification_date = :next_verification_date,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = :id`, e)
	if err != nil {
		return fmt.Errorf("update inventory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("inventory not found")
	}
	return nil
}

// Delete удаляет оборудование
func (r *InventoryRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM inventory WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete inventory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("inventory not found")
	}
	return nil
}

// BackfillNames заполняет name_normalized у строк, заведённых до появления колонки.
// SQLite не поднимает регистр кириллицы, поэтому нормализация — на стороне Go.
// Вызывается при старте, повторный вызов ничего не меняет.
func (r *InventoryRepo) BackfillNames(ctx context.Context) error {
	rows := []struct {
		ID   int64  `db:"id"`
		Name string `db:"name"`
	}{}
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, name FROM inventory WHERE name_normalized = ''`); err != nil {
		return fmt.Errorf("select inventory for name backfill: %w", err)
	}
	for _, row := range rows {
		if _, err := r.db.ExecContext(ctx, `UPDATE inventory SET name_normalized = ? WHERE id = ?`, NormalizeName(row.Name), row.ID); err != nil {
			return fmt.Errorf("backfill inventory name %d: %w", row.ID, err)
		}
	}
	if len(rows) > 0 {
		r.log.Info("inventory name_normalized backfilled", zap.Int("rows", len(rows)))
	}
	return nil
}
