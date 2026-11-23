package handler

import (
	"context"
	"errors"
	"mitm-departament/internal/models"
	"mitm-departament/internal/report"
	"net/http"
	"net/url"
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
	RoomInventory(ctx context.Context, location string) ([]models.Inventory, error)
	Locations(ctx context.Context) ([]string, error)
	ListExpiredVerification(ctx context.Context, limit, offset int64) (models.ListInventory, error)
	Update(ctx context.Context, e *models.Inventory) error
	Delete(ctx context.Context, id int64) error

	// инвентаризация — опись материального отдела (данные конфиденциальные)
	SearchNumbers(ctx context.Context, query, sort string, limit int) ([]models.InventoryNumber, error)
	SummaryNumbers(ctx context.Context, query string) (models.InventorySummary, error)
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
		Inventory.GET("/locations", requireRoles(adminKey), h.locations)
		Inventory.GET("/room-opis", requireRoles(adminKey), h.roomOpis)
		Inventory.GET("/expired-verification", h.listExpiredVerification)
		Inventory.PUT("/:id", requireRoles(adminKey), h.update)
		Inventory.DELETE("/:id", requireRoles(adminKey), h.delete)
	}

	// Инвентаризация — опись материального отдела. Информация конфиденциальная:
	// весь раздел доступен только администраторам.
	// Подсказки доступны любому вошедшему: сотруднику нужен номер и что за позиция.
	// Цен и сумм здесь нет — их отдают только администраторские маршруты.
	rg.GET("/inventory-numbers/hints", h.hints)
	rg.GET("/inventory-numbers", requireRoles(adminKey), h.numbers)
	rg.GET("/inventory-numbers/lookup", requireRoles(adminKey), h.lookupNumber)
	rg.GET("/inventory-numbers/summary", requireRoles(adminKey), h.summaryNumbers)
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

	// ?group=1 — реестр строками-группами (общий номер и/или название)
	if filter.Group {
		resp := GroupedInventoryResponse{
			PaginatedMetadata: data.PaginatedMetadata,
			ObjectsTotal:      data.ObjectsTotal,
			Groups:            make([]InventoryGroupResponse, 0, len(data.Groups)),
		}
		for _, g := range data.Groups {
			group := InventoryGroupResponse{
				Key:                g.Key,
				Title:              g.Title,
				InventoryNumber:    g.InventoryNumber,
				HasNumbers:         g.HasNumbers,
				NumbersDistinct:    g.NumbersDistinct,
				Count:              g.Count,
				Types:              g.Types,
				Locations:          g.Locations,
				NextVerifications:  g.NextVerifications,
				AllAvailable:       g.AllAvailable,
				NoNumberCount:      g.NoNumberCount,
				NotInRegistryCount: g.NotInRegistryCount,
				Members:            make([]InventoryResponse, 0, len(g.Members)),
			}
			for i := range g.Members {
				group.Members = append(group.Members, ToInventoryResponse(&g.Members[i]))
			}
			resp.Groups = append(resp.Groups, group)
		}
		c.JSON(http.StatusOK, resp)
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

// numbers отдаёт строки описи материального отдела: страница инвентаризации.
// ?sort= — порядок вывода (пустой — как в документах).
func (h *InventoryHandler) numbers(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "5000"))
	if err != nil || limit <= 0 {
		limit = 5000
	}
	items, err := h.svc.SearchNumbers(c.Request.Context(), c.Query("search"), c.Query("sort"), limit)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToInventoryNumberResponses(items))
}

// summaryNumbers — итог по описи для блока сверху страницы (только админ):
// строк, единиц и рублей. ?search= — тот же поиск, что и у списка.
func (h *InventoryHandler) summaryNumbers(c *gin.Context) {
	summary, err := h.svc.SummaryNumbers(c.Request.Context(), c.Query("search"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToInventorySummaryResponse(summary))
}

// importNumbers загружает таблицу номеров кафедры (только админ).
// hints — подсказки по описи материального отдела: без цен и сумм.
// ?number= — точная позиция по инвентарному номеру (карточка объекта),
// иначе список для автодополнения (?search= — подстрока номера или наименования).
func (h *InventoryHandler) hints(c *gin.Context) {
	ctx := c.Request.Context()
	if number := strings.TrimSpace(c.Query("number")); number != "" {
		item, err := h.svc.LookupNumber(ctx, number)
		if err != nil {
			handleError(c, err)
			return
		}
		if item == nil {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "запись инвентаризации не найдена"})
			return
		}
		c.JSON(http.StatusOK, ToInventoryNumberHint(item))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "5000"))
	if err != nil || limit <= 0 {
		limit = 5000
	}
	items, err := h.svc.SearchNumbers(ctx, c.Query("search"), "", limit)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToInventoryNumberHints(items))
}

// bindNumber разбирает тело запроса в строку описи и проверяет инвентарный номер.
func (h *InventoryHandler) bindNumber(c *gin.Context) (*models.InventoryNumber, bool) {
	var req InventoryNumberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return nil, false
	}
	item := &models.InventoryNumber{
		Number:         strings.TrimSpace(req.Number),
		Name:           strings.TrimSpace(req.Name),
		DocumentNumber: strings.TrimSpace(req.DocumentNumber),
		Unit:           strings.TrimSpace(req.Unit),
		Quantity:       req.Quantity,
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
		items = append(items, models.InventoryNumber{
			Number: it.Number, Name: it.Name, Source: req.Source, DocumentNumber: it.DocumentNumber,
			Unit: it.Unit, Quantity: it.Quantity, Price: it.Price, Amount: it.Amount,
		})
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

// locations — аудитории, по которым есть объекты реестра. Нужен списку
// выбора в описи кабинета: он показывает только реальные кабинеты, а не
// произвольный ввод.
func (h *InventoryHandler) locations(c *gin.Context) {
	list, err := h.svc.Locations(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	if list == nil {
		list = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"locations": list})
}

// roomOpis — опись имущества аудитории в Word. Доступ только администраторам:
// в бланке инвентарные номера, а раздел материального отдела конфиденциальный.
//
// Параметры: location (номер аудитории), date=today|empty (печатать ли дату
// формирования), zav (ФИО заведующего лабораторией), check=1 и check_name
// (строка «Проверил» для бухгалтера).
func (h *InventoryHandler) roomOpis(c *gin.Context) {
	location := strings.TrimSpace(c.Query("location"))
	if location == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "не указана аудитория"})
		return
	}

	items, err := h.svc.RoomInventory(c.Request.Context(), location)
	if err != nil {
		handleError(c, err)
		return
	}

	params := report.RoomOpisParams{
		Room:      location,
		Zav:       c.Query("zav"),
		Checked:   c.Query("check") == "1",
		CheckName: c.Query("check_name"),
		Items:     h.roomItems(c.Request.Context(), items),
	}
	// Дата: today (по умолчанию) — день скачивания, empty — без даты,
	// YYYY-MM-DD — выбранная вручную дата (печатается в бланке).
	switch raw := strings.TrimSpace(c.Query("date")); raw {
	case "", "today":
		params.Date = time.Now().In(msk)
	case "empty":
		// дата не печатается
	default:
		d, err := time.ParseInLocation("2006-01-02", raw, msk)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверная дата: нужен формат ГГГГ-ММ-ДД"})
			return
		}
		params.Date = d
	}

	file, err := report.BuildRoomOpis(params)
	if err != nil {
		if errors.Is(err, report.ErrRoomOpisEmpty) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "в этой аудитории нет объектов"})
			return
		}
		handleError(c, err)
		return
	}

	extendDeadlines(c, downloadTimeout)

	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="room-opis.docx"; filename*=UTF-8''`+url.PathEscape(report.RoomOpisFileName(params)))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", file)
}

// roomItems собирает строки описи и подставляет единицу измерения из описи
// материального отдела. Номера объектов и номера матотдела — разные учётные
// системы, совпадение бывает не всегда; без него считаем позицию штучной
// (по умолчанию "шт." ставит сам генератор бланка).
func (h *InventoryHandler) roomItems(ctx context.Context, items []models.Inventory) []report.RoomOpisItem {
	out := groupRoomItems(items)
	units := make(map[string]string)
	for i := range out {
		num := strings.TrimSpace(out[i].Number)
		if num == "" {
			continue
		}
		key := models.CanonicalInventoryNumber(num)
		unit, ok := units[key]
		if !ok {
			unit = ""
			if found, err := h.svc.LookupNumber(ctx, num); err == nil && found != nil {
				unit = strings.TrimSpace(found.Unit)
			}
			units[key] = unit
		}
		out[i].Unit = unit
	}
	return out
}

// groupRoomItems собирает позиции описи: одинаковые наименование и номер
// (комплект) идут одной строкой с количеством, разные номера — отдельными.
func groupRoomItems(items []models.Inventory) []report.RoomOpisItem {
	type key struct {
		section string
		name    string
		number  string
	}

	out := make([]report.RoomOpisItem, 0, len(items))
	seen := make(map[key]int, len(items))
	for _, it := range items {
		section := roomSection(it.Type)
		number := ""
		if it.InventoryNumber != nil {
			number = strings.TrimSpace(*it.InventoryNumber)
		}
		k := key{section: section, name: it.NameNormalized, number: models.CanonicalInventoryNumber(number)}
		if i, ok := seen[k]; ok {
			out[i].Qty++
			continue
		}
		seen[k] = len(out)
		out = append(out, report.RoomOpisItem{Section: section, Name: it.Name, Number: number, Qty: 1})
	}
	return out
}

// roomSection переводит категорию реестра в раздел бланка описи.
func roomSection(t string) string {
	switch t {
	case "equipment":
		return report.SectionEquipment
	case "inventory":
		return report.SectionFurniture
	default:
		return report.SectionOther
	}
}
