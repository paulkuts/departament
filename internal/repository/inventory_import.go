package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mitm-departament/internal/models"
	"strings"

	"go.uber.org/zap"
)

// ErrImportBatchNotFound — пакет импорта с таким id не найден (откат).
var ErrImportBatchNotFound = errors.New("import batch not found")

// importSnapshot — прежние значения строки описи: их возвращает откат пакета.
// Хранится в inventory_import_rows.before_json только для обновлённых строк.
type importSnapshot struct {
	ID             int64    `db:"id" json:"id"`
	Number         string   `db:"number" json:"number"`
	Name           string   `db:"name" json:"name"`
	Unit           string   `db:"unit" json:"unit"`
	Quantity       *float64 `db:"quantity" json:"quantity"`
	DocumentNumber string   `db:"document_number" json:"document_number"`
	AcceptedAt     string   `db:"accepted_at" json:"accepted_at"`
	WearPercent    *float64 `db:"wear_percent" json:"wear_percent"`
}

// ImportBatch — пакетная вставка строк описи («Добавить таблицей»).
// Строка ищется по каноническому номеру и нормализованному наименованию —
// так же, как в Import: новая добавляется, известная обновляется. Пакет
// записывается со своей меткой, а каждая строка — со снимком прежних значений,
// поэтому откат возвращает опись в состояние до загрузки.
// Одна транзакция: либо пакет записан целиком, либо ничего.
func (r *InventoryNumberRepo) ImportBatch(ctx context.Context, batch *models.InventoryImportBatch, items []models.InventoryNumber) (*models.InventoryImportReport, error) {
	report := &models.InventoryImportReport{Problems: []models.InventoryImportProblem{}}
	if len(items) == 0 {
		return report, nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin import batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO inventory_import_batches (label, document_number, created_by) VALUES (?, ?, ?)`,
		strings.TrimSpace(batch.Label), strings.TrimSpace(batch.DocumentNumber), strings.TrimSpace(batch.CreatedBy))
	if err != nil {
		return nil, fmt.Errorf("create import batch: %w", err)
	}
	batchID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create import batch id: %w", err)
	}
	batch.ID = batchID
	report.BatchID = batchID

	for i := range items {
		it := &items[i]
		number := strings.TrimSpace(it.Number)
		name := strings.TrimSpace(it.Name)
		normalized := models.CanonicalInventoryNumber(number)
		nameNormalized := NormalizeName(name)
		unit := strings.TrimSpace(it.Unit)
		documentNumber := strings.TrimSpace(it.DocumentNumber)
		acceptedAt := strings.TrimSpace(it.AcceptedAt)

		var before importSnapshot
		lookupErr := tx.GetContext(ctx, &before,
			`SELECT id, number, name, unit, quantity, document_number, accepted_at, wear_percent
			 FROM inventory_numbers WHERE normalized = ? AND name_normalized = ? ORDER BY id LIMIT 1`,
			normalized, nameNormalized)

		switch {
		case errors.Is(lookupErr, sql.ErrNoRows):
			insert, err := tx.ExecContext(ctx,
				`INSERT INTO inventory_numbers (number, normalized, name, name_normalized, source, document_number, unit, quantity, accepted_at, wear_percent)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				number, normalized, name, nameNormalized, "вставка таблицей",
				documentNumber, unit, it.Quantity, acceptedAt, it.WearPercent)
			if err != nil {
				return nil, fmt.Errorf("insert inventory number %q: %w", number, err)
			}
			itemID, err := insert.LastInsertId()
			if err != nil {
				return nil, fmt.Errorf("insert inventory number id %q: %w", number, err)
			}
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO inventory_import_rows (batch_id, item_id, action, before_json) VALUES (?, ?, 'added', '')`,
				batchID, itemID); err != nil {
				return nil, fmt.Errorf("record import row %q: %w", number, err)
			}
			report.Added++
		case lookupErr != nil:
			return nil, fmt.Errorf("lookup inventory number %q: %w", number, lookupErr)
		default:
			beforeJSON, err := json.Marshal(before)
			if err != nil {
				return nil, fmt.Errorf("snapshot inventory number %q: %w", number, err)
			}
			if _, err = tx.ExecContext(ctx,
				`UPDATE inventory_numbers SET number = ?, name = ?, name_normalized = ?, document_number = ?, unit = ?, quantity = ?,
				 accepted_at = COALESCE(NULLIF(?, ''), accepted_at), wear_percent = COALESCE(?, wear_percent)
				 WHERE id = ?`,
				number, name, nameNormalized, documentNumber, unit, it.Quantity, acceptedAt, it.WearPercent, before.ID); err != nil {
				return nil, fmt.Errorf("update inventory number %q: %w", number, err)
			}
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO inventory_import_rows (batch_id, item_id, action, before_json) VALUES (?, ?, 'updated', ?)`,
				batchID, before.ID, string(beforeJSON)); err != nil {
				return nil, fmt.Errorf("record import row %q: %w", number, err)
			}
			report.Updated++
		}
	}

	if _, err = tx.ExecContext(ctx,
		`UPDATE inventory_import_batches SET added = ?, updated = ? WHERE id = ?`,
		report.Added, report.Updated, batchID); err != nil {
		return nil, fmt.Errorf("update import batch: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import batch: %w", err)
	}
	r.log.Info("инвентаризация: пакет вставки записан",
		zap.Int64("пакет", batchID), zap.Int("добавлено", report.Added), zap.Int("обновлено", report.Updated))
	return report, nil
}

// RollbackImportBatch откатывает пакет одной операцией: добавленные строки
// удаляются, обновлённым возвращаются прежние значения. Строку, которую уже
// отметили к списанию, не удаляем — иначе каскад снёс бы историю списания;
// такие строки попадают в «оставлено».
func (r *InventoryNumberRepo) RollbackImportBatch(ctx context.Context, batchID int64) (*models.InventoryImportRollback, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin import rollback: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err = tx.GetContext(ctx, &exists, `SELECT COUNT(*) FROM inventory_import_batches WHERE id = ?`, batchID); err != nil {
		return nil, fmt.Errorf("find import batch: %w", err)
	}
	if exists == 0 {
		return nil, ErrImportBatchNotFound
	}

	type importRow struct {
		ItemID     int64  `db:"item_id"`
		Action     string `db:"action"`
		BeforeJSON string `db:"before_json"`
	}
	var rows []importRow
	if err = tx.SelectContext(ctx, &rows,
		`SELECT item_id, action, before_json FROM inventory_import_rows WHERE batch_id = ? ORDER BY id`, batchID); err != nil {
		return nil, fmt.Errorf("read import rows: %w", err)
	}

	out := &models.InventoryImportRollback{}
	for _, row := range rows {
		switch row.Action {
		case "added":
			var marks int
			if err = tx.GetContext(ctx, &marks, `SELECT COUNT(*) FROM inventory_writeoffs WHERE item_id = ?`, row.ItemID); err != nil {
				return nil, fmt.Errorf("check writeoffs %d: %w", row.ItemID, err)
			}
			if marks > 0 {
				out.Kept++
				continue
			}
			res, err := tx.ExecContext(ctx, `DELETE FROM inventory_numbers WHERE id = ?`, row.ItemID)
			if err != nil {
				return nil, fmt.Errorf("delete imported row %d: %w", row.ItemID, err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				out.Kept++
			} else {
				out.Removed++
			}
		case "updated":
			var before importSnapshot
			if err = json.Unmarshal([]byte(row.BeforeJSON), &before); err != nil {
				return nil, fmt.Errorf("read snapshot %d: %w", row.ItemID, err)
			}
			res, err := tx.ExecContext(ctx,
				`UPDATE inventory_numbers SET number = ?, normalized = ?, name = ?, name_normalized = ?, document_number = ?, unit = ?, quantity = ?, accepted_at = ?, wear_percent = ?
				 WHERE id = ?`,
				before.Number, models.CanonicalInventoryNumber(before.Number), before.Name, NormalizeName(before.Name),
				before.DocumentNumber, before.Unit, before.Quantity, before.AcceptedAt, before.WearPercent, row.ItemID)
			if err != nil {
				return nil, fmt.Errorf("restore imported row %d: %w", row.ItemID, err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				out.Kept++
			} else {
				out.Restored++
			}
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_import_batches WHERE id = ?`, batchID); err != nil {
		return nil, fmt.Errorf("delete import batch: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import rollback: %w", err)
	}
	r.log.Info("инвентаризация: пакет вставки откачен",
		zap.Int64("пакет", batchID), zap.Int("удалено", out.Removed),
		zap.Int("восстановлено", out.Restored), zap.Int("оставлено", out.Kept))
	return out, nil
}
