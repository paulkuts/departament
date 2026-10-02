package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Ошибки загрузки фотографии: хендлер превращает их в понятные ответы 400.
var (
	ErrPhotoTooLarge   = errors.New("photo too large")
	ErrPhotoEmpty      = errors.New("photo empty")
	ErrPhotoLimit      = errors.New("photos limit reached")
	ErrPhotoUnreadable = errors.New("photo unreadable")
	ErrPhotoHugePixels = errors.New("photo resolution too big")
)

// thumbSide — длинная сторона миниатюры для сетки превью, maxPixels — предел
// разрешения исходника (больше 40 Мп не читаем: разбор такого снимка надолго
// занимает процессор и память сервера).
const (
	thumbSide = 480
	maxPixels = 40 * 1000 * 1000
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
	repo      PhotoRepo
	inventory InventoryRepo
	cfg       config.PhotoConfig
	log       *zap.Logger
}

func NewPhotoService(repo PhotoRepo, inventory InventoryRepo, cfg config.PhotoConfig, log *zap.Logger) *PhotoService {
	return &PhotoService{repo: repo, inventory: inventory, cfg: cfg, log: log}
}

// Create сохраняет фотографию объекта в его собственный каталог и делает рядом
// миниатюру для сетки превью.
func (p *PhotoService) Create(ctx context.Context, file multipart.File, ext string, photo *models.InventoryPhoto) error {
	count, err := p.repo.CountByInventory(ctx, photo.InventoryID)
	if err != nil {
		return err
	}
	if count >= p.cfg.MaxPhotos {
		return ErrPhotoLimit
	}

	// Каталог объекта: data/photos/<объект>/<uuid>.<ext> и миниатюры рядом.
	// Плоский каталог на все фотографии неудобно разбирать и архивировать.
	objectDir := strconv.FormatInt(photo.InventoryID, 10)
	if err := os.MkdirAll(filepath.Join(p.cfg.InventoryPhotoDir, objectDir), 0755); err != nil {
		return fmt.Errorf("create photo dir: %w", err)
	}

	storedName := uuid.New().String() + ext
	photo.StoredName = filepath.Join(objectDir, storedName)
	dst := filepath.Join(p.cfg.InventoryPhotoDir, photo.StoredName)

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create photo file: %w", err)
	}
	// Размер считаем по факту записи: в multipart-заголовке он может быть -1
	// (запрос без Content-Length), и проверка заголовка ничего не поймала бы.
	written, copyErr := io.Copy(out, io.LimitReader(file, int64(p.cfg.MaxPhotoSize)+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(dst)
		if copyErr != nil {
			return fmt.Errorf("save photo: %w", copyErr)
		}
		return fmt.Errorf("save photo: %w", closeErr)
	}
	if written == 0 {
		os.Remove(dst)
		return ErrPhotoEmpty
	}
	if written > int64(p.cfg.MaxPhotoSize) {
		os.Remove(dst)
		return ErrPhotoTooLarge
	}
	photo.SizeBytes = written

	// Разбираем изображение: это и проверка, что файл — действительно картинка,
	// и источник размеров и миниатюры.
	if err := p.makeThumb(photo); err != nil {
		os.Remove(dst)
		return err
	}

	if err := p.repo.Create(ctx, photo); err != nil {
		os.Remove(dst)
		if photo.ThumbName != "" {
			os.Remove(filepath.Join(p.cfg.InventoryPhotoDir, photo.ThumbName))
		}
		return err
	}

	// created_at ставит база: в ответе отдаём запись из неё, а не структуру с
	// нулевым временем (в ответе на загрузку было 0001-01-01).
	if saved, err := p.repo.GetByID(ctx, photo.ID); err == nil && saved != nil {
		*photo = *saved
	}

	return nil
}

// decodable — форматы, которые читает стандартная библиотека Go: для них
// миниатюра обязательна, и сбой разбора означает битый файл.
func decodable(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif":
		return true
	}
	return false
}

// makeThumb запоминает размеры исходника и кладёт миниатюру рядом с ним.
func (p *PhotoService) makeThumb(photo *models.InventoryPhoto) error {
	src, err := os.Open(filepath.Join(p.cfg.InventoryPhotoDir, photo.StoredName))
	if err != nil {
		return err
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		// WebP Go не читает (в стандартной библиотеке только декодеры JPEG, PNG
		// и GIF): файл храним как есть, без миниатюры — браузер покажет его сам.
		if !decodable(photo.ContentType) {
			return nil
		}
		// Остальное — битый файл: тип определён по содержимому как картинка, а
		// разбирается как изображение только он. HEIC до этой точки не доходит.
		return ErrPhotoUnreadable
	}
	bounds := img.Bounds()
	photo.Width, photo.Height = bounds.Dx(), bounds.Dy()
	if photo.Width <= 0 || photo.Height <= 0 {
		return ErrPhotoUnreadable
	}
	if photo.Width*photo.Height > maxPixels {
		return ErrPhotoHugePixels
	}

	objectDir := strconv.FormatInt(photo.InventoryID, 10)
	thumbDir := filepath.Join(p.cfg.InventoryPhotoDir, objectDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return fmt.Errorf("create thumb dir: %w", err)
	}

	name := strings.TrimSuffix(filepath.Base(photo.StoredName), filepath.Ext(photo.StoredName)) + ".jpg"
	photo.ThumbName = filepath.Join(objectDir, "thumbs", name)

	dst, err := os.Create(filepath.Join(p.cfg.InventoryPhotoDir, photo.ThumbName))
	if err != nil {
		return fmt.Errorf("create thumb: %w", err)
	}
	defer dst.Close()

	if err := jpeg.Encode(dst, fitInside(img, thumbSide), &jpeg.Options{Quality: 82}); err != nil {
		return fmt.Errorf("encode thumb: %w", err)
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
	if photo == nil {
		return fmt.Errorf("photo %d not found", id)
	}

	if err := p.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete photo: %w", err)
	}
	removePhotoFiles(p.cfg.InventoryPhotoDir, photo)

	return nil
}

// RemoveByInventory убирает каталог объекта целиком. Строки фотографий уходят
// каскадом вместе с объектом, а файлы без этого оставались сиротами на диске.
func (p *PhotoService) RemoveByInventory(ctx context.Context, inventoryID int64) error {
	return removePhotoDir(p.cfg.InventoryPhotoDir, inventoryID)
}

func (p *PhotoService) GetInventoryByID(ctx context.Context, id int64) (*models.Inventory, error) {
	return p.inventory.GetByID(ctx, id)
}

// removePhotoDir удаляет каталог объекта с оригиналами и миниатюрами.
func removePhotoDir(dir string, inventoryID int64) error {
	return os.RemoveAll(filepath.Join(dir, strconv.FormatInt(inventoryID, 10)))
}

// removePhotoFiles удаляет файл фотографии и её миниатюру; отсутствие файла
// ошибкой не считается.
func removePhotoFiles(dir string, photo *models.InventoryPhoto) {
	if photo == nil {
		return
	}
	for _, name := range []string{photo.StoredName, photo.ThumbName} {
		if name == "" {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
