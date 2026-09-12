// internal/repository/inventory_photo.go

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"mitm-departament/internal/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type PhotoRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewPhotoRepo(db *sqlx.DB, log *zap.Logger) *PhotoRepo {
	return &PhotoRepo{db: db, log: log}
}

func (r *PhotoRepo) Create(ctx context.Context, p *models.InventoryPhoto) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO inventory_photos (inventory_id, filename, stored_name, content_type, size_bytes, uploaded_by)
         VALUES (?, ?, ?, ?, ?, ?)`,
		p.InventoryID, p.Filename, p.StoredName, p.ContentType, p.SizeBytes, p.UploadedBy,
	)
	if err != nil {
		return fmt.Errorf("insert photo: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (r *PhotoRepo) ListByInventory(ctx context.Context, inventoryID int64) ([]models.InventoryPhoto, error) {
	var photos []models.InventoryPhoto
	err := r.db.SelectContext(ctx, &photos,
		`SELECT id, inventory_id, filename, stored_name, content_type, size_bytes, uploaded_by, created_at
     FROM inventory_photos WHERE inventory_id = ? ORDER BY id ASC`, inventoryID)
	if err != nil {
		return nil, fmt.Errorf("list photos: %w", err)
	}
	return photos, nil
}

func (r *PhotoRepo) GetByID(ctx context.Context, id int64) (*models.InventoryPhoto, error) {
	p := &models.InventoryPhoto{}
	err := r.db.GetContext(ctx, p,
		`SELECT id, inventory_id, filename, stored_name, content_type, size_bytes, uploaded_by, created_at
         FROM inventory_photos WHERE id = ?`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get photo: %w", err)
	}
	return p, nil
}

func (r *PhotoRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM inventory_photos WHERE id = ?`, id)
	return err
}

func (r *PhotoRepo) CountByInventory(ctx context.Context, inventoryID int64) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM inventory_photos WHERE inventory_id = ?`, inventoryID)
	return count, err
}
