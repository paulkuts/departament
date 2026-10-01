package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"mitm-departament/internal/models"
	"mitm-departament/internal/service"

	"github.com/gin-gonic/gin"
)

// WriteoffService — списание позиций описи материального отдела.
type WriteoffService interface {
	ListDrafts(ctx context.Context) ([]models.InventoryWriteoff, error)
	SaveDraft(ctx context.Context, itemID int64, quantity float64, reason string, actorID *string) error
	RemoveDraft(ctx context.Context, itemID int64) error
	ApplyDrafts(ctx context.Context, actorID *string) (models.WriteoffResult, error)
	BuildReport(ctx context.Context, params service.ReportParams) ([]byte, string, error)
}

type WriteoffHandler struct {
	svc WriteoffService
}

func NewWriteoffHandler(svc WriteoffService) *WriteoffHandler {
	return &WriteoffHandler{svc: svc}
}

// Списание — часть раздела «Инвентаризация»: данные конфиденциальные, поэтому
// доступ только у администраторов, как и у всей описи.
// :id в маршрутах — идентификатор позиции описи: отметка на списание у позиции
// одна, отдельного идентификатора отметки наружу не нужно.
func (h *WriteoffHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/inventory-writeoffs", requireRoles(adminKey), h.list)
	rg.PUT("/inventory-writeoffs/:id", requireRoles(adminKey), h.save)
	rg.DELETE("/inventory-writeoffs/:id", requireRoles(adminKey), h.remove)
	rg.POST("/inventory-writeoffs/apply", requireRoles(adminKey), h.apply)
	rg.GET("/inventory-writeoffs/report", requireRoles(adminKey), h.report)
}

// list — отмеченные на списание позиции: вкладка «Списание».
func (h *WriteoffHandler) list(c *gin.Context) {
	items, err := h.svc.ListDrafts(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToWriteoffResponses(items))
}

// save — галочка «Расх.» и поля под строкой: сколько списать и по какой причине.
func (h *WriteoffHandler) save(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req WriteoffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}
	if err := h.svc.SaveDraft(c.Request.Context(), id, req.Quantity, req.Reason, actorID(c)); err != nil {
		h.writeoffError(c, err)
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "позиция отмечена на списание"})
}

// remove — снятая галочка: позиция остаётся в описи как была.
func (h *WriteoffHandler) remove(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RemoveDraft(c.Request.Context(), id); err != nil {
		h.writeoffError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// apply — «Применить списание»: после подписанного отчёта опись корректируется.
func (h *WriteoffHandler) apply(c *gin.Context) {
	result, err := h.svc.ApplyDrafts(c.Request.Context(), actorID(c))
	if err != nil {
		h.writeoffError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToWriteoffResultResponse(result))
}

// report — отчёт по форме материального отдела для текущих отметок.
// Реквизиты шапки приходят запросом: их вводит администратор перед скачиванием.
func (h *WriteoffHandler) report(c *gin.Context) {
	params := service.ReportParams{
		Department: c.Query("department"),
		Person:     c.Query("person"),
		PeriodFrom: c.Query("from"),
		PeriodTo:   c.Query("to"),
	}
	file, name, err := h.svc.BuildReport(c.Request.Context(), params)
	if err != nil {
		h.writeoffError(c, err)
		return
	}

	extendDeadlines(c, downloadTimeout)

	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="writeoff.xlsx"; filename*=UTF-8''`+url.PathEscape(name))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file)
}

// writeoffError переводит ошибки сервиса в понятные ответы API: «позиции нет»
// и «нечего списывать» — не сбой сервера, а обычная ситуация в интерфейсе.
func (h *WriteoffHandler) writeoffError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWriteoffItemNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrWriteoffEmpty),
		errors.Is(err, service.ErrWriteoffQuantity),
		errors.Is(err, service.ErrWriteoffReason):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		handleError(c, err)
	}
}

// actorID — идентификатор администратора, выполнившего операцию: он пишется в
// отметку, чтобы в истории было видно, кто списал. Тот же приём, что у загрузки
// документов: без вошедшего пользователя поле остаётся пустым.
func actorID(c *gin.Context) *string {
	userID, err := getUserID(c)
	if err != nil {
		return nil
	}
	id := userID.String()
	if id == "" {
		return nil
	}
	return &id
}
