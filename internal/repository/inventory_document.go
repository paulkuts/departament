// internal/repository/inventory_document.go

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// DocumentRepo — архивные документы раздела «Инвентаризация».
type DocumentRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewDocumentRepo(db *sqlx.DB, log *zap.Logger) *DocumentRepo {
	return &DocumentRepo{db: db, log: log}
}

const documentColumns = `d.id, d.filename, d.stored_name, d.content_type, d.size_bytes, d.uploaded_by, d.created_at`

// List отдаёт документы от новых к старым; имя загрузившего — из users.
func (r *DocumentRepo) List(ctx context.Context) ([]models.InventoryDocument, error) {
	items := []models.InventoryDocument{}
	if err := r.db.SelectContext(ctx, &items,
		`SELECT `+documentColumns+`, u.full_name AS uploaded_by_name
		 FROM inventory_documents d
		 LEFT JOIN users u ON u.id = d.uploaded_by
		 ORDER BY d.id DESC`); err != nil {
		return nil, fmt.Errorf("list inventory documents: %w", err)
	}
	return items, nil
}

// GetByID отдаёт документ по id. Нет записи — (nil, nil).
func (r *DocumentRepo) GetByID(ctx context.Context, id int64) (*models.InventoryDocument, error) {
	item := &models.InventoryDocument{}
	err := r.db.GetContext(ctx, item,
		`SELECT `+documentColumns+`, u.full_name AS uploaded_by_name
		 FROM inventory_documents d
		 LEFT JOIN users u ON u.id = d.uploaded_by
		 WHERE d.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory document %d: %w", id, err)
	}
	return item, nil
}

func (r *DocumentRepo) Create(ctx context.Context, doc *models.InventoryDocument) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO inventory_documents (filename, stored_name, content_type, size_bytes, uploaded_by)
		 VALUES (?, ?, ?, ?, ?)`,
		doc.Filename, doc.StoredName, doc.ContentType, doc.SizeBytes, doc.UploadedBy,
	)
	if err != nil {
		return fmt.Errorf("create inventory document %q: %w", doc.Filename, err)
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		doc.ID = id
	}
	return nil
}

func (r *DocumentRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM inventory_documents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete inventory document %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("document not found")
	}
	return nil
}

func (r *DocumentRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM inventory_documents`); err != nil {
		return 0, fmt.Errorf("count inventory documents: %w", err)
	}
	return n, nil
}
