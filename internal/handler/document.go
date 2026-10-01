package handler

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"mitm-departament/internal/service"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type InventoryDocumentHandler struct {
	svc InventoryDocumentService
	cfg config.DocumentConfig
}

type InventoryDocumentService interface {
	Create(ctx context.Context, file multipart.File, ext string, doc *models.InventoryDocument) error
	List(ctx context.Context) ([]models.InventoryDocument, error)
	GetByID(ctx context.Context, id int64) (*models.InventoryDocument, error)
	Delete(ctx context.Context, id int64) error
}

func NewDocumentHandler(svc InventoryDocumentService, cfg config.DocumentConfig) *InventoryDocumentHandler {
	// Создаём папку для документов при старте
	_ = os.MkdirAll(cfg.DocumentDir, 0755)
	return &InventoryDocumentHandler{svc: svc, cfg: cfg}
}

// Документы раздела «Инвентаризация» — архив материального отдела, доступ только
// администраторам (как и весь раздел).
func (h *InventoryDocumentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/inventory-documents", requireRoles(adminKey), h.list)
	rg.POST("/inventory-documents", requireRoles(adminKey), h.upload)
	rg.GET("/inventory-documents/:id/download", requireRoles(adminKey), h.download)
	rg.DELETE("/inventory-documents/:id", requireRoles(adminKey), h.delete)
}

// POST /inventory-documents  (multipart/form-data, поле "document")
func (h *InventoryDocumentHandler) upload(c *gin.Context) {
	// Файл может быть крупным (до cfg.MaxDocumentSize) и уходить по медленному каналу,
	// поэтому на этом маршруте снимаем общие дедлайны сервера (read_timeout 5s).
	extendDeadlines(c, uploadTimeout)

	file, header, err := c.Request.FormFile("document")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "поле 'document' обязательно"})
		return
	}
	defer file.Close()

	if header.Size > int64(h.cfg.MaxDocumentSize) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("файл слишком большой (макс. %d МБ)", h.cfg.MaxDocumentSize/(1024*1024))})
		return
	}
	if header.Size == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "файл пустой"})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	userID, _ := getUserID(c)
	var uploadedBy *string
	if s := userID.String(); s != "" {
		uploadedBy = &s
	}

	doc := &models.InventoryDocument{
		Filename:    filepath.Base(header.Filename),
		ContentType: contentType,
		SizeBytes:   header.Size,
		UploadedBy:  uploadedBy,
	}

	if err := h.svc.Create(c.Request.Context(), file, safeExt(header.Filename), doc); err != nil {
		switch {
		case errors.Is(err, service.ErrDocumentTooLarge):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("файл слишком большой (макс. %d МБ)", h.cfg.MaxDocumentSize/(1024*1024))})
		case errors.Is(err, service.ErrDocumentEmpty):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "файл пустой"})
		case errors.Is(err, service.ErrDocumentsLimit):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("превышен лимит документов (%d)", h.cfg.MaxDocuments)})
		default:
			handleError(c, err)
		}
		return
	}

	c.JSON(http.StatusCreated, doc)
}

// GET /inventory-documents
func (h *InventoryDocumentHandler) list(c *gin.Context) {
	docs, err := h.svc.List(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, docs)
}

// GET /inventory-documents/:id/download — отдаёт файл под исходным именем
func (h *InventoryDocumentHandler) download(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	doc, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil || doc == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "документ не найден"})
		return
	}

	filePath := filepath.Join(h.cfg.DocumentDir, doc.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "файл отсутствует на диске"})
		return
	}

	extendDeadlines(c, downloadTimeout)

	c.Header("Cache-Control", "private, no-store")
	c.FileAttachment(filePath, doc.Filename)
}

// DELETE /inventory-documents/:id
func (h *InventoryDocumentHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "документ удалён"})
}

const (
	uploadTimeout   = 10 * time.Minute
	downloadTimeout = 10 * time.Minute
)

// extendDeadlines продлевает дедлайны чтения и записи для текущего запроса:
// общие лимиты сервера (read_timeout/write_timeout) рассчитаны на обычные
// запросы и обрывают передачу крупных файлов.
func extendDeadlines(c *gin.Context, d time.Duration) {
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetReadDeadline(time.Now().Add(d))
	_ = rc.SetWriteDeadline(time.Now().Add(d))
}

// safeExt берёт расширение из имени файла: строчные латинские буквы и цифры.
// Имя на диске собирается из него, поэтому ничего лишнего в путь не попадает.
func safeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if len(ext) < 2 || len(ext) > 12 {
		return ""
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return ext
}
