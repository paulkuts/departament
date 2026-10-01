package repository

import (
	"context"
	"fmt"
	"math"
	"strings"

	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// InventoryWriteoffRepo — отметки позиций описи на списание (таблица
// inventory_writeoffs). Отметка одна на позицию: повторная галочка обновляет
// количество и причину, снятая галочка отметку удаляет.
// Пока отметка draft, опись не меняется — администратор скачивает отчёт,
// подписывает его и только потом применяет списание.
type InventoryWriteoffRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewInventoryWriteoffRepo(db *sqlx.DB, log *zap.Logger) *InventoryWriteoffRepo {
	return &InventoryWriteoffRepo{db: db, log: log}
}

// Колонки отметки и позиции: вкладка «Списание» показывает те же данные, что
// опись, — наименование, номер, количество, цену и № документа.
const inventoryWriteoffColumns = `w.id, w.item_id, w.quantity, w.reason, w.status, w.created_by, w.created_at, w.applied_by, w.applied_at,
	i.name, i.number, i.unit, i.document_number, i.quantity AS item_quantity, i.price`

// ListDrafts — отмеченные позиции вкладки «Списание» в порядке описи.
func (r *InventoryWriteoffRepo) ListDrafts(ctx context.Context) ([]models.InventoryWriteoff, error) {
	items := []models.InventoryWriteoff{}
	if err := r.db.SelectContext(ctx, &items,
		`SELECT `+inventoryWriteoffColumns+` FROM inventory_writeoffs w
		 JOIN inventory_numbers i ON i.id = w.item_id
		 WHERE w.status = 'draft' ORDER BY i.id`); err != nil {
		return nil, fmt.Errorf("list writeoff drafts: %w", err)
	}
	return items, nil
}

// ItemExists — есть ли такая позиция в описи. Списать можно только то, что
// в описи есть и ещё не списано целиком: иначе внешний ключ отбил бы запись
// ошибкой сервера вместо понятного ответа.
func (r *InventoryWriteoffRepo) ItemExists(ctx context.Context, itemID int64) (bool, error) {
	var count int
	if err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM inventory_numbers WHERE id = ? AND is_written_off = 0`, itemID); err != nil {
		return false, fmt.Errorf("check writeoff item %d: %w", itemID, err)
	}
	return count > 0, nil
}

// SaveDraft ставит или обновляет отметку на списание: количество и причина.
func (r *InventoryWriteoffRepo) SaveDraft(ctx context.Context, itemID int64, quantity float64, reason string, actorID *string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO inventory_writeoffs (item_id, quantity, reason, status, created_by)
		 VALUES (?, ?, ?, 'draft', ?)
		 ON CONFLICT(item_id) WHERE status = 'draft'
		 DO UPDATE SET quantity = excluded.quantity, reason = excluded.reason`,
		itemID, quantity, strings.TrimSpace(reason), actorID); err != nil {
		return fmt.Errorf("save writeoff draft for item %d: %w", itemID, err)
	}
	return nil
}

// DeleteDraft снимает отметку: позиция остаётся в описи без изменений.
// Применённые списания (история) не трогаются.
func (r *InventoryWriteoffRepo) DeleteDraft(ctx context.Context, itemID int64) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM inventory_writeoffs WHERE item_id = ? AND status = 'draft'`, itemID); err != nil {
		return fmt.Errorf("delete writeoff draft for item %d: %w", itemID, err)
	}
	return nil
}

// writeoffDraft — что нужно для применения списания по одной позиции.
type writeoffDraft struct {
	ID           int64    `db:"id"`
	ItemID       int64    `db:"item_id"`
	Quantity     float64  `db:"quantity"`
	ItemQuantity *float64 `db:"item_quantity"`
	Price        *float64 `db:"price"`
}

// ApplyDrafts применяет все отметки: количество в описи уменьшается, полностью
// израсходованная позиция уходит из списка, отметки становятся историей.
// Всё одной транзакцией: половина применённого списания хуже неприменённого —
// отменить это можно только руками.
// Списать больше, чем есть в описи, нельзя: количество ограничивается остатком
// (в отчёте, который уже подписан, могла стоять иная цифра, поэтому итог
// возвращается вызывающему — он показывает его администратору).
func (r *InventoryWriteoffRepo) ApplyDrafts(ctx context.Context, actorID *string) (models.WriteoffResult, error) {
	result := models.WriteoffResult{}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin writeoff apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	drafts := []writeoffDraft{}
	if err := tx.SelectContext(ctx, &drafts,
		`SELECT w.id, w.item_id, w.quantity, i.quantity AS item_quantity, i.price
		 FROM inventory_writeoffs w
		 JOIN inventory_numbers i ON i.id = w.item_id
		 WHERE w.status = 'draft' ORDER BY w.id`); err != nil {
		return result, fmt.Errorf("load writeoff drafts: %w", err)
	}
	if len(drafts) == 0 {
		return result, nil
	}

	for _, d := range drafts {
		effective := d.Quantity
		var newQuantity *float64
		var newAmount *float64
		fully := true

		if d.ItemQuantity != nil {
			if effective > *d.ItemQuantity {
				effective = *d.ItemQuantity
			}
			remaining := math.Round((*d.ItemQuantity-effective)*1000) / 1000
			if remaining < 0 {
				remaining = 0
			}
			newQuantity = &remaining
			fully = remaining <= 0
			if d.Price != nil {
				amount := math.Round(*d.Price*remaining*100) / 100
				newAmount = &amount
			}
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory_numbers SET quantity = ?, amount = ?, written_off = written_off + ?, is_written_off = ? WHERE id = ?`,
			newQuantity, newAmount, effective, boolToInt(fully), d.ItemID); err != nil {
			return result, fmt.Errorf("write off item %d: %w", d.ItemID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory_writeoffs SET status = 'applied', applied_by = ?, applied_at = CURRENT_TIMESTAMP WHERE id = ?`,
			actorID, d.ID); err != nil {
			return result, fmt.Errorf("mark writeoff %d applied: %w", d.ID, err)
		}

		result.Items++
		result.Quantity += effective
		if d.Price != nil {
			result.Amount += math.Round(*d.Price*effective*100) / 100
		}
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit writeoff apply: %w", err)
	}
	return result, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
