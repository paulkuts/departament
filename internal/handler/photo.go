package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"mitm-departament/internal/service"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
)

// Что принимаем: только то, что браузер умеет показать. Тип определяется по
// содержимому файла (первые байты), а не по имени и не по заголовку запроса:
// иначе в хранилище попадал бы переименованный файл или снимок в формате,
// который потом не открывается на телефоне.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

type InventoryPhotoHandler struct {
	svc InventoryPhotoService
	cfg config.PhotoConfig
}

type InventoryPhotoService interface {
	Create(ctx context.Context, file multipart.File, ext string, photo *models.InventoryPhoto) error
	ListByInventory(ctx context.Context, InventoryID int64) ([]models.InventoryPhoto, error)
	GetByID(ctx context.Context, id int64) (*models.InventoryPhoto, error)
	Delete(ctx context.Context, id int64) error
	GetInventoryByID(ctx context.Context, id int64) (*models.Inventory, error)
}

func NewPhotoHandler(svc InventoryPhotoService, cfg config.PhotoConfig) *InventoryPhotoHandler {
	// Создаём папку для фото при старте
	_ = os.MkdirAll(cfg.InventoryPhotoDir, 0755)
	return &InventoryPhotoHandler{svc: svc, cfg: cfg}
}

func (h *InventoryPhotoHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/inventory/:id/photos", requireRoles(adminKey), h.upload)
	rg.DELETE("/photos/:photo_id", requireRoles(adminKey), h.delete)
}

// Публичный маршрут — отдача файла (для <img src>)
func (h *InventoryPhotoHandler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	rg.GET("/inventory/:id/photos", h.list)
	rg.GET("/photos/:photo_id", h.serve)
	rg.GET("/photos/:photo_id/thumb", h.thumb)
	rg.GET("/inventory/:id/qr", h.qrCode)
}

// POST /Inventory/:id/photos  (multipart/form-data, поле "photo")
func (h *InventoryPhotoHandler) upload(c *gin.Context) {
	equipID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// Фото с телефона весит несколько мегабайт и уходит по мобильной сети —
	// общий read_timeout сервера (5 с) обрывал такие загрузки на середине.
	extendDeadlines(c, uploadTimeout)

	file, header, err := c.Request.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "поле 'photo' обязательно"})
		return
	}
	defer file.Close()

	// Тип — по содержимому. Здесь же отбивается HEIC с iPhone: Go его не
	// прочитает, а браузер не покажет.
	contentType, ext := sniffImage(file)
	if contentType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "это не изображение. Принимаем JPEG, PNG, WebP и GIF; снимок в формате HEIC сохраните как JPEG."})
		return
	}

	// Метаданные в БД
	userID, _ := getUserID(c)
	var uploadedBy *string
	if s := userID.String(); s != "" {
		uploadedBy = &s
	}

	photo := &models.InventoryPhoto{
		InventoryID: equipID,
		Filename:    filepath.Base(header.Filename),
		ContentType: contentType,
		UploadedBy:  uploadedBy,
	}

	if err := h.svc.Create(c.Request.Context(), file, ext, photo); err != nil {
		switch {
		case errors.Is(err, service.ErrPhotoTooLarge):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("файл слишком большой (макс. %d МБ)", h.cfg.MaxPhotoSize/(1024*1024))})
		case errors.Is(err, service.ErrPhotoHugePixels):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "слишком большое разрешение снимка (макс. 40 Мп) — уменьшите фото"})
		case errors.Is(err, service.ErrPhotoEmpty):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "файл пустой"})
		case errors.Is(err, service.ErrPhotoUnreadable):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "изображение не читается: принимаем JPEG, PNG, WebP и GIF"})
		case errors.Is(err, service.ErrPhotoLimit):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("у объекта уже %d фотографий", h.cfg.MaxPhotos)})
		default:
			handleError(c, err)
		}
		return
	}

	c.JSON(http.StatusCreated, photo)
}

// sniffImage определяет тип файла по первым байтам и возвращает его вместе с
// расширением для имени на диске. Пустые значения — файл не изображение.
func sniffImage(file multipart.File) (string, string) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", ""
	}
	// Дальше файл читается целиком — возвращаем указатель в начало.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", ""
	}

	contentType := strings.TrimSpace(strings.Split(http.DetectContentType(head[:n]), ";")[0])
	ext, ok := allowedTypes[contentType]
	if !ok {
		return "", ""
	}
	return contentType, ext
}

// GET /Inventory/:id/photos
func (h *InventoryPhotoHandler) list(c *gin.Context) {
	equipID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	photos, err := h.svc.ListByInventory(c.Request.Context(), equipID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, photos)
}

// GET /photos/:photo_id — отдаёт сам файл
func (h *InventoryPhotoHandler) serve(c *gin.Context) {
	photo := h.findPhoto(c)
	if photo == nil {
		return
	}
	path, ok := h.filePath(c, photo.StoredName)
	if !ok {
		return
	}

	// ?download=1 — отдать файл под исходным именем (кнопка «Скачать»).
	if c.Query("download") != "" {
		c.FileAttachment(path, photo.Filename)
		return
	}

	// Имя файла исходное, с русскими буквами: FormatMediaType сам соберёт
	// filename*=UTF-8''… — подставить имя в заголовок напрямую нельзя.
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": photo.Filename}))
	h.sendFile(c, path)
}

// GET /photos/:photo_id/thumb — миниатюра для сетки превью: карточка с
// фотографиями открывается быстро, полноразмерный файл тянется только в превью.
func (h *InventoryPhotoHandler) thumb(c *gin.Context) {
	photo := h.findPhoto(c)
	if photo == nil {
		return
	}
	name := photo.ThumbName
	if name == "" {
		// Миниатюры нет (запись до их появления) — отдаём оригинал.
		name = photo.StoredName
	}
	path, ok := h.filePath(c, name)
	if !ok {
		return
	}
	h.sendFile(c, path)
}

// DELETE /photos/:photo_id
func (h *InventoryPhotoHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "photo_id")
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "фото удалено"})
}

// GET /inventory/:id/qr — генерирует и отдаёт QR-код со ссылкой на страницу оборудования
func (h *InventoryPhotoHandler) qrCode(c *gin.Context) {
	equipID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// Проверяем, существует ли оборудование
	_, err := h.svc.GetInventoryByID(c.Request.Context(), equipID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "оборудование не найдено"})
		return
	}

	// Формируем URL страницы оборудования (относительный путь для фронтенда)
	baseURL := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	inventoryURL := scheme + "://" + baseURL + "/#/equipment/view/" + fmt.Sprintf("%d", equipID)

	// Генерируем QR-код
	pngData, err := qrcode.Encode(inventoryURL, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "ошибка генерации QR-кода"})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"inventory_%d_qr.png\"", equipID))
	c.Data(http.StatusOK, "image/png", pngData)
}

// findPhoto читает запись о фотографии; если её нет — отвечает 404 и nil.
func (h *InventoryPhotoHandler) findPhoto(c *gin.Context) *models.InventoryPhoto {
	id, ok := parseIDParam(c, "photo_id")
	if !ok {
		return nil
	}
	photo, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil || photo == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "фото не найдено"})
		return nil
	}
	return photo
}

// filePath собирает путь к файлу и проверяет, что он на месте.
func (h *InventoryPhotoHandler) filePath(c *gin.Context, name string) (string, bool) {
	path := filepath.Join(h.cfg.InventoryPhotoDir, filepath.FromSlash(name))
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "файл отсутствует на диске"})
		return "", false
	}
	return path, true
}

// sendFile отдаёт файл с частным кешем: по ссылке файл неизменен, поэтому
// повторный показ карточки не тянет его с сервера заново. Дедлайны продлеваем —
// на телефоне фото может скачиваться дольше общих 10 секунд.
func (h *InventoryPhotoHandler) sendFile(c *gin.Context, path string) {
	extendDeadlines(c, downloadTimeout)
	c.Header("Cache-Control", "private, max-age=86400")
	c.File(path)
}
