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

const inventoryNumberColumns = `id, number, normalized, name, source, created_at`

// Search отдаёт номера для подсказки в форме объекта.
// Пустой запрос — первые limit записей (форма грузит справочник один раз).
func (r *InventoryNumberRepo) Search(ctx context.Context, query string, limit int) ([]models.InventoryNumber, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	items := []models.InventoryNumber{}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		if err := r.db.SelectContext(ctx, &items,
			`SELECT `+inventoryNumberColumns+` FROM inventory_numbers
			 ORDER BY LENGTH(number), number LIMIT ?`, limit); err != nil {
			return nil, fmt.Errorf("list inventory numbers: %w", err)
		}
		return items, nil
	}
	normalized := models.NormalizeInventoryNumber(trimmed)
	pattern := "%" + normalized + "%"
	if err := r.db.SelectContext(ctx, &items,
		`SELECT `+inventoryNumberColumns+` FROM inventory_numbers
		 WHERE normalized LIKE ? OR UPPER(number) LIKE ?
		 ORDER BY LENGTH(number), number LIMIT ?`, pattern, "%"+strings.ToUpper(trimmed)+"%", limit); err != nil {
		return nil, fmt.Errorf("search inventory numbers: %w", err)
	}
	return items, nil
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

// Import загружает таблицу номеров: новые строки добавляются, существующие
// обновляются по нормализованному номеру. replace очищает справочник перед загрузкой.
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
		source := strings.TrimSpace(it.Source)
		if source == "" {
			source = "таблица кафедры"
		}

		var id int64
		lookupErr := tx.GetContext(ctx, &id, `SELECT id FROM inventory_numbers WHERE normalized = ?`, normalized)
		switch {
		case errors.Is(lookupErr, sql.ErrNoRows):
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO inventory_numbers (number, normalized, name, source) VALUES (?, ?, ?, ?)`,
				number, normalized, name, source); err != nil {
				return 0, 0, fmt.Errorf("insert inventory number %q: %w", it.Number, err)
			}
			added++
		case lookupErr != nil:
			return 0, 0, fmt.Errorf("lookup inventory number %q: %w", it.Number, lookupErr)
		default:
			if _, err = tx.ExecContext(ctx,
				`UPDATE inventory_numbers SET number = ?, name = ? WHERE id = ?`,
				number, name, id); err != nil {
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
