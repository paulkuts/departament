package handler

import (
	"context"
	"mitm-departament/internal/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// InventoryService — интерфейс сервиса оборудования
type InventoryService interface {
	Create(ctx context.Context, e *models.Inventory) error
	GetByID(ctx context.Context, id int64) (*models.Inventory, error)
	List(ctx context.Context, f models.InventoryFilter) (models.ListInventory, error)
	ListExpiredVerification(ctx context.Context, limit, offset int64) (models.ListInventory, error)
	Update(ctx context.Context, e *models.Inventory) error
	Delete(ctx context.Context, id int64) error

	// инвентаризация — опись материального отдела (данные конфиденциальные)
	SearchNumbers(ctx context.Context, query string, limit int) ([]models.InventoryNumber, error)
	NotInRegistryMark(ctx context.Context, number string) (bool, error)
	LookupNumber(ctx context.Context, number string) (*models.InventoryNumber, error)
	CreateNumber(ctx context.Context, item *models.InventoryNumber) error
	UpdateNumber(ctx context.Context, item *models.InventoryNumber) error
	ImportNumbers(ctx context.Context, items []models.InventoryNumber, replace bool) (int, int, int, error)
	DeleteNumber(ctx context.Context, id int64) error
}

type InventoryHandler struct {
	svc InventoryService
}

func NewInventoryHandler(svc InventoryService) *InventoryHandler {
	return &InventoryHandler{svc: svc}
}

func (h *InventoryHandler) RegisterRoutes(rg *gin.RouterGroup) {
	Inventory := rg.Group("/inventory")
	{
		Inventory.POST("", requireRoles(adminKey), h.create)
		Inventory.GET("", h.list)
		Inventory.GET("/expired-verification", h.listExpiredVerification)
		Inventory.PUT("/:id", requireRoles(adminKey), h.update)
		Inventory.DELETE("/:id", requireRoles(adminKey), h.delete)
	}

	// Инвентаризация — опись материального отдела. Информация конфиденциальная:
	// весь раздел доступен только администраторам.
	rg.GET("/inventory-numbers", requireRoles(adminKey), h.numbers)
	rg.GET("/inventory-numbers/lookup", requireRoles(adminKey), h.lookupNumber)
	rg.POST("/inventory-numbers", requireRoles(adminKey), h.createNumber)
	rg.PUT("/inventory-numbers/:id", requireRoles(adminKey), h.updateNumber)
	rg.POST("/inventory-numbers/import", requireRoles(adminKey), h.importNumbers)
	rg.DELETE("/inventory-numbers/:id", requireRoles(adminKey), h.deleteNumber)
}

func (h *InventoryHandler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	Inventory := rg.Group("/inventory")
	{
		Inventory.GET("/:id", h.getByID)
	}
}

// parseDate парсит дату формата "2006-01-02"
func parseDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (h *InventoryHandler) create(c *gin.Context) {
	var req CreateInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	last_verificationDate, err := parseDate(req.LastVerificationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат даты поверки (ожидается YYYY-MM-DD)"})
		return
	}

	next_verificationDate, err := parseDate(req.NextVerificationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат даты поверки (ожидается YYYY-MM-DD)"})
		return
	}

	status := true
	if req.Status != nil {
		status = *req.Status
	}

	noNumberOnItem, notInRegistry, err := h.resolveNumberFlags(c.Request.Context(), req.InventoryNumber, req.NoNumberOnItem, req.NotInRegistry)
	if err != nil {
		handleError(c, err)
		return
	}

	Inventory := &models.Inventory{
		Name:                 req.Name,
		Type:                 req.Type,
		Description:          req.Description,
		Location:             req.Location,
		Documentation:        req.Documentation,
		InventoryNumber:      req.InventoryNumber,
		ResponsibleID:        req.ResponsibleID,
		NoNumberOnItem:       noNumberOnItem,
		NotInRegistry:        notInRegistry,
		Status:               status,
		LastVerificationDate: last_verificationDate,
		NextVerificationDate: next_verificationDate,
	}

	Inventory.Status = status
	if status {
		reason := "Причина не указана"
		Inventory.UnavailableReason = &reason // доступен → причины нет
	} else {
		Inventory.UnavailableReason = req.UnavailableReason
	}

	if err := h.svc.Create(c.Request.Context(), Inventory); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToInventoryResponse(Inventory))
}

func (h *InventoryHandler) getByID(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	Inventory, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToInventoryResponse(Inventory))
}

func (h *InventoryHandler) list(c *gin.Context) {
	// Биндим query-параметры: limit, offset, search, inventory
	var filter models.InventoryFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		handleValidationError(c, err)
		return
	}

	filter.Paginated.Validate()

	// status парсим отдельно (строка → bool)
	switch c.Query("status") {
	case "available":
		v := true
		filter.Status = &v
	case "unavailable":
		v := false
		filter.Status = &v
	}

	data, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func (h *InventoryHandler) listExpiredVerification(c *gin.Context) {
	var p models.Paginated
	if err := c.ShouldBindQuery(&p); err != nil {
		handleValidationError(c, err)
		return
	}

	p.Validate()

	items, err := h.svc.ListExpiredVerification(c.Request.Context(), p.Limit, p.Offset)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, items)
}

func (h *InventoryHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	Inventory, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	last_verificationDate, err := parseDate(req.LastVerificationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат даты поверки (ожидается YYYY-MM-DD)"})
		return
	}

	next_verificationDate, err := parseDate(req.NextVerificationDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат даты поверки (ожидается YYYY-MM-DD)"})
		return
	}

	if req.UnavailableReason != nil && *req.UnavailableReason != "" {
		Inventory.UnavailableReason = req.UnavailableReason
	}

	noNumberOnItem, notInRegistry, err := h.resolveNumberFlags(c.Request.Context(), req.InventoryNumber, req.NoNumberOnItem, req.NotInRegistry)
	if err != nil {
		handleError(c, err)
		return
	}
	Inventory.NoNumberOnItem = noNumberOnItem
	Inventory.NotInRegistry = notInRegistry

	Inventory.Name = req.Name
	Inventory.Description = req.Description
	Inventory.Location = req.Location
	Inventory.Documentation = req.Documentation
	Inventory.InventoryNumber = req.InventoryNumber
	Inventory.ResponsibleID = req.ResponsibleID
	Inventory.LastVerificationDate = last_verificationDate
	Inventory.NextVerificationDate = next_verificationDate
	if req.Status != nil {
		Inventory.Status = *req.Status
	}

	if err := h.svc.Update(c.Request.Context(), Inventory); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToInventoryResponse(Inventory))
}

func (h *InventoryHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "оборудование удалено"})
}

// resolveNumberFlags определяет пометки объекта.
// «Нет инв. номера на оборудовании» — как прислал клиент (обычная галочка).
// «Нет в таблице» — присланное значение важнее расчёта: сотрудник может снять
// пометку вручную. Если поле не прислано вовсе, пометка считается по справочнику.
func (h *InventoryHandler) resolveNumberFlags(ctx context.Context, number *string, noNumberOnItem, notInRegistry *bool) (bool, bool, error) {
	noNumber := noNumberOnItem != nil && *noNumberOnItem
	if notInRegistry != nil {
		return noNumber, *notInRegistry, nil
	}
	if number == nil {
		return noNumber, false, nil
	}
	mark, err := h.svc.NotInRegistryMark(ctx, *number)
	if err != nil {
		return noNumber, false, err
	}
	return noNumber, mark, nil
}

// numbers отдаёт справочник инвентарных номеров для подсказки в форме объекта.
func (h *InventoryHandler) numbers(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "5000"))
	if err != nil || limit <= 0 {
		limit = 5000
	}
	items, err := h.svc.SearchNumbers(c.Request.Context(), c.Query("search"), limit)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToInventoryNumberResponses(items))
}

// importNumbers загружает таблицу номеров кафедры (только админ).
// bindNumber разбирает тело запроса в строку описи и проверяет инвентарный номер.
func (h *InventoryHandler) bindNumber(c *gin.Context) (*models.InventoryNumber, bool) {
	var req InventoryNumberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return nil, false
	}
	item := &models.InventoryNumber{
		Number:   strings.TrimSpace(req.Number),
		Name:     strings.TrimSpace(req.Name),
		Unit:     strings.TrimSpace(req.Unit),
		Quantity: req.Quantity,
		Price:    req.Price,
		Amount:   req.Amount,
	}
	canonical := models.CanonicalInventoryNumber(item.Number)
	if canonical == "" || strings.Trim(canonical, "/") == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "инвентарный номер не заполнен"})
		return nil, false
	}
	return item, true
}

// lookupNumber — строка инвентаризации по инвентарному номеру объекта:
// карточка объекта показывает администратору запись описи. Записи нет — 404.
func (h *InventoryHandler) lookupNumber(c *gin.Context) {
	item, err := h.svc.LookupNumber(c.Request.Context(), c.Query("number"))
	if err != nil {
		handleError(c, err)
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "запись инвентаризации не найдена"})
		return
	}
	c.JSON(http.StatusOK, ToInventoryNumberResponse(item))
}

// createNumber — новая строка инвентаризации.
func (h *InventoryHandler) createNumber(c *gin.Context) {
	item, ok := h.bindNumber(c)
	if !ok {
		return
	}
	if err := h.svc.CreateNumber(c.Request.Context(), item); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ToInventoryNumberResponse(item))
}

// updateNumber — правка строки инвентаризации.
func (h *InventoryHandler) updateNumber(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	item, ok := h.bindNumber(c)
	if !ok {
		return
	}
	item.ID = id
	if err := h.svc.UpdateNumber(c.Request.Context(), item); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToInventoryNumberResponse(item))
}

func (h *InventoryHandler) importNumbers(c *gin.Context) {
	var req ImportInventoryNumbersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}
	items := make([]models.InventoryNumber, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, models.InventoryNumber{Number: it.Number, Name: it.Name, Source: req.Source})
	}
	added, updated, total, err := h.svc.ImportNumbers(c.Request.Context(), items, req.Replace)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ImportInventoryNumbersResponse{Added: added, Updated: updated, Total: total})
}

// deleteNumber удаляет номер из справочника (только админ).
func (h *InventoryHandler) deleteNumber(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteNumber(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
