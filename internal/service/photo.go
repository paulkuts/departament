package service

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PhotoRepo interface {
	Create(ctx context.Context, p *models.InventoryPhoto) error
	ListByInventory(ctx context.Context, inventoryID int64) ([]models.InventoryPhoto, error)
	GetByID(ctx context.Context, id int64) (*models.InventoryPhoto, error)
	Delete(ctx context.Context, id int64) error
	CountByInventory(ctx context.Context, inventoryID int64) (int, error)
}

type InventoryRepo interface {
	GetByID(ctx context.Context, id int64) (*models.Inventory, error)
}

type PhotoService struct {
	repo       PhotoRepo
	inventory  InventoryRepo
	cfg        config.PhotoConfig
	log        *zap.Logger
}

func NewPhotoService(repo PhotoRepo, inventory InventoryRepo, cfg config.PhotoConfig, log *zap.Logger) *PhotoService {
	return &PhotoService{repo: repo, inventory: inventory, cfg: cfg, log: log}
}

func (p *PhotoService) Create(ctx context.Context, file multipart.File, ext string, photo *models.InventoryPhoto) error {
	count, err := p.repo.CountByInventory(ctx, photo.InventoryID)
	if err != nil {
		return err
	}
	if count >= p.cfg.MaxPhotos {
		return fmt.Errorf("Превышен лимит фотографий для этого оборудования")
	}

	storedName := uuid.New().String() + ext
	dst := filepath.Join(p.cfg.InventoryPhotoDir, storedName)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		os.Remove(dst)
		return err
	}

	photo.StoredName = storedName

	err = p.repo.Create(ctx, photo)
	if err != nil {
		os.Remove(dst) // откатываем файл
		return err
	}

	return nil
}

func (p *PhotoService) ListByInventory(ctx context.Context, inventoryID int64) ([]models.InventoryPhoto, error) {
	return p.repo.ListByInventory(ctx, inventoryID)
}

func (p *PhotoService) GetByID(ctx context.Context, id int64) (*models.InventoryPhoto, error) {
	return p.repo.GetByID(ctx, id)
}

func (p *PhotoService) Delete(ctx context.Context, id int64) error {
	photo, err := p.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("photo %d not found", id)
		}
		return fmt.Errorf("get photo by id: %w", err)
	}

	filePath := filepath.Join(p.cfg.InventoryPhotoDir, photo.StoredName)
	_ = os.Remove(filePath)

	if err := p.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete photo: %w", err)
	}

	return nil
}

func (p *PhotoService) GetInventoryByID(ctx context.Context, id int64) (*models.Inventory, error) {
	return p.inventory.GetByID(ctx, id)
}
