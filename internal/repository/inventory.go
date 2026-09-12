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
	responsible_id, status, unavailable_reason, last_verification_date, next_verification_date, created_at, updated_at`

// Create создаёт единицу оборудования
func (r *InventoryRepo) Create(ctx context.Context, e *models.Inventory) error {
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO inventory 
			(name, type, description, location, documentation, inventory_number, responsible_id, status, unavailable_reason, last_verification_date, next_verification_date)
		 VALUES 
			(:name, :type, :description, :location, :documentation, :inventory_number, :responsible_id, :status, :unavailable_reason, :last_verification_date, :next_verification_date)`, e)
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
//   - Search:          поиск по названию (LIKE %search%)
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
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+*f.Search+"%") // содержит
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
	res, err := r.db.NamedExecContext(ctx,
		`UPDATE inventory SET 
			name = :name,
			type = :type,
			description = :description,
			location = :location,
			documentation = :documentation,
			inventory_number = :inventory_number,
			responsible_id = :responsible_id,
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
