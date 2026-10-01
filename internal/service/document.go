// internal/service/document.go
//
// Архивные документы раздела «Инвентаризация»: метаданные в БД, файлы — в каталоге
// cfg.DocumentDir. Документы не разбираются приложением — их только хранят, отдают
// и удаляют. Приём тот же, что у фотографий объектов (service/photo.go).

package service

import (
	"context"
	"errors"
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

type DocumentRepo interface {
	Create(ctx context.Context, doc *models.InventoryDocument) error
	List(ctx context.Context) ([]models.InventoryDocument, error)
	GetByID(ctx context.Context, id int64) (*models.InventoryDocument, error)
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int, error)
}

var (
	ErrDocumentEmpty    = errors.New("файл пустой")
	ErrDocumentTooLarge = errors.New("файл слишком большой")
	ErrDocumentsLimit   = errors.New("превышен лимит документов")
)

type DocumentService struct {
	repo DocumentRepo
	cfg  config.DocumentConfig
	log  *zap.Logger
}

func NewDocumentService(repo DocumentRepo, cfg config.DocumentConfig, log *zap.Logger) *DocumentService {
	return &DocumentService{repo: repo, cfg: cfg, log: log}
}

// Create сохраняет файл на диск и записывает метаданные в БД.
// Размер проверяется по фактически записанным байтам: браузер присылает
// Content-Length, но запрос без него (chunked) обошёл бы проверку заголовка.
func (d *DocumentService) Create(ctx context.Context, file multipart.File, ext string, doc *models.InventoryDocument) error {
	count, err := d.repo.Count(ctx)
	if err != nil {
		return err
	}
	if d.cfg.MaxDocuments > 0 && count >= d.cfg.MaxDocuments {
		return ErrDocumentsLimit
	}

	storedName := uuid.New().String() + ext
	dst := filepath.Join(d.cfg.DocumentDir, storedName)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, io.LimitReader(file, int64(d.cfg.MaxDocumentSize)+1))
	if err != nil {
		os.Remove(dst)
		return err
	}
	if written == 0 {
		os.Remove(dst)
		return ErrDocumentEmpty
	}
	if d.cfg.MaxDocumentSize > 0 && written > int64(d.cfg.MaxDocumentSize) {
		os.Remove(dst)
		return ErrDocumentTooLarge
	}

	doc.StoredName = storedName
	doc.SizeBytes = written

	if err := d.repo.Create(ctx, doc); err != nil {
		os.Remove(dst) // откатываем файл
		return err
	}

	return nil
}

func (d *DocumentService) List(ctx context.Context) ([]models.InventoryDocument, error) {
	return d.repo.List(ctx)
}

func (d *DocumentService) GetByID(ctx context.Context, id int64) (*models.InventoryDocument, error) {
	return d.repo.GetByID(ctx, id)
}

// Delete убирает файл с диска и запись из БД.
func (d *DocumentService) Delete(ctx context.Context, id int64) error {
	doc, err := d.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get document by id: %w", err)
	}
	if doc == nil {
		return errors.New("document not found")
	}

	_ = os.Remove(filepath.Join(d.cfg.DocumentDir, doc.StoredName))

	if err := d.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}

	return nil
}
