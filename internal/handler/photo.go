package handler

import (
	"context"
	"fmt"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
)

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
	rg.GET("/inventory/:id/qr", h.qrCode)
}

// POST /Inventory/:id/photos  (multipart/form-data, поле "photo")
func (h *InventoryPhotoHandler) upload(c *gin.Context) {
	equipID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	file, header, err := c.Request.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "поле 'photo' обязательно"})
		return
	}
	defer file.Close()

	// Размер
	if header.Size > int64(h.cfg.MaxPhotoSize) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "файл слишком большой (макс. 10 МБ)"})
		return
	}

	// Тип
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedTypes[contentType]
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "допустимы: JPEG, PNG, WebP, GIF"})
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
		Filename:    header.Filename,
		ContentType: contentType,
		SizeBytes:   header.Size,
		UploadedBy:  uploadedBy,
	}

	if err := h.svc.Create(c.Request.Context(), file, ext, photo); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, photo)
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
	id, ok := parseIDParam(c, "photo_id")
	if !ok {
		return
	}

	photo, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil || photo == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "фото не найдено"})
		return
	}

	filePath := filepath.Join(h.cfg.InventoryPhotoDir, photo.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "файл отсутствует на диске"})
		return
	}

	c.Header("Cache-Control", "private, no-store")
	c.File(filePath)
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
