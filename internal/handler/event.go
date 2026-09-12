package handler

import (
	"context"
	"mitm-departament/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// EventService — интерфейс сервиса событий
type EventService interface {
	CreateEvent(ctx context.Context, event models.Event) (int64, error)
	Event(ctx context.Context, id int64) (*models.Event, error)
	EventsList(ctx context.Context, f models.EventFilter) (models.EventList, error)
	UpdateEvent(ctx context.Context, id int64, u *models.UpdateEvent) error
	DeleteEvent(ctx context.Context, id int64) error
}

type EventHandler struct {
	svc     EventService
	userSvc UserService
}

func NewEventHandler(svc EventService, userSvc UserService) *EventHandler {
	return &EventHandler{svc: svc, userSvc: userSvc}
}

func (h *EventHandler) RegisterRoutes(rg *gin.RouterGroup) {
	events := rg.Group("/events")
	{
		events.POST("", h.create)
		events.GET("", h.list)
		events.GET("/:id", h.getByID)
		events.PUT("/:id", h.update)
		events.DELETE("/:id", h.delete)
	}
}

func (h *EventHandler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	events := rg.Group("/events")
	{
		events.GET("", h.list)
		events.GET("/:id", h.getByID)
	}
}

// parseDateTime парсит дату-время формата "2006-01-02T15:04" или "2006-01-02 15:04"
func parseDateTime(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}

	// Пробуем формат ISO 8601 с T
	t, err := time.Parse("2006-01-02T15:04", *s)
	if err != nil {
		// Пробуем формат с пробелом
		t, err = time.Parse("2006-01-02 15:04", *s)
		if err != nil {
			// Пробуем полный формат ISO 8601
			t, err = time.Parse("2006-01-02T15:04:05", *s)
			if err != nil {
				// Пробуем полный формат с пробелом
				t, err = time.Parse("2006-01-02 15:04:05", *s)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return &t, nil
}

// ToEventResponse преобразует модель события в ответ API
func ToEventResponse(e *models.Event, creatorFullName string) EventResponse {
	resp := EventResponse{
		ID:              e.ID,
		CreatorID:       e.CreatorID,
		CreatorFullName: creatorFullName,
		Title:           *e.Title,
		Location:        *e.Location,
		Description:     e.Description,
		StartTime:       e.StartTime.Format("2006-01-02 15:04:05"),
		IsPublic:        e.IsPublic,
		CreatedAt:       e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	return resp
}

// create обрабатывает POST /api/v1/events
func (h *EventHandler) create(c *gin.Context) {
	var req CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	// Получаем userID из контекста (если есть)
	var creatorID string
	if uid, err := getUserID(c); err == nil {
		creatorID = uid.String()
	} else {
		// Если нет в контексте, пробуем из запроса
		creatorID = req.CreatorID
	}

	if creatorID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "creator_id is required"})
		return
	}

	startTime, err := parseDateTime(req.StartTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат времени начала (ожидается YYYY-MM-DDTHH:MM или YYYY-MM-DD HH:MM)"})
		return
	}

	event := models.Event{
		CreatorID:   creatorID,
		Title:       req.Title,
		Location:    req.Location,
		Description: req.Description,
		StartTime:   startTime,
		IsPublic:    req.IsPublic,
	}

	id, err := h.svc.CreateEvent(c.Request.Context(), event)
	if err != nil {
		handleError(c, err)
		return
	}

	// Получаем созданное событие для ответа
	createdEvent, err := h.svc.Event(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	// Получаем имя создателя
	creatorFullName := h.getCreatorFullName(c, createdEvent.CreatorID)

	c.JSON(http.StatusCreated, ToEventResponse(createdEvent, creatorFullName))
}

// getByID обрабатывает GET /api/v1/events/:id
func (h *EventHandler) getByID(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	event, err := h.svc.Event(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	// Получаем имя создателя
	creatorFullName := h.getCreatorFullName(c, event.CreatorID)

	c.JSON(http.StatusOK, ToEventResponse(event, creatorFullName))
}

// list обрабатывает GET /api/v1/events
func (h *EventHandler) list(c *gin.Context) {
	var filter models.EventFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		handleValidationError(c, err)
		return
	}

	filter.Paginated.Validate()

	data, err := h.svc.EventsList(c.Request.Context(), filter)
	if err != nil {
		handleError(c, err)
		return
	}

	// Преобразуем список событий в ответ API
	events := make([]EventResponse, 0, len(data.Events))
	for i := range data.Events {
		creatorFullName := h.getCreatorFullName(c, data.Events[i].CreatorID)
		events = append(events, ToEventResponse(&data.Events[i], creatorFullName))
	}

	c.JSON(http.StatusOK, gin.H{
		"paginated_metadata": data.PaginatedMetadata,
		"events":             events,
	})
}

// update обрабатывает PUT /api/v1/events/:id
func (h *EventHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	updateData := &models.UpdateEvent{}

	if req.Title != nil {
		updateData.Title = req.Title
	}
	if req.Location != nil {
		updateData.Location = req.Location
	}
	if req.Description != nil {
		updateData.Description = req.Description
	}
	if req.StartTime != nil {
		startTime, err := parseDateTime(req.StartTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "неверный формат времени начала (ожидается YYYY-MM-DDTHH:MM или YYYY-MM-DD HH:MM)"})
			return
		}
		updateData.StartTime = startTime
	}
	if req.IsPublic != nil {
		updateData.IsPublic = req.IsPublic
	}

	if err := h.svc.UpdateEvent(c.Request.Context(), id, updateData); err != nil {
		handleError(c, err)
		return
	}

	// Получаем обновленное событие для ответа
	updatedEvent, err := h.svc.Event(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	// Получаем имя создателя
	creatorFullName := h.getCreatorFullName(c, updatedEvent.CreatorID)

	c.JSON(http.StatusOK, ToEventResponse(updatedEvent, creatorFullName))
}

// delete обрабатывает DELETE /api/v1/events/:id
func (h *EventHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteEvent(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "событие удалено"})
}

// getCreatorFullName получает полное имя создателя события
func (h *EventHandler) getCreatorFullName(c *gin.Context, creatorID string) string {
	if h.userSvc == nil {
		return ""
	}

	user, err := h.userSvc.GetByID(c.Request.Context(), creatorID)
	if err != nil {
		return ""
	}
	if user == nil {
		return ""
	}
	return user.FullName
}
