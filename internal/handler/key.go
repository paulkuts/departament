package handler

import (
	"mitm-departament/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type KeyHandler struct {
	svc KeyService
}

func NewKeyHandler(svc KeyService) *KeyHandler {
	return &KeyHandler{svc: svc}
}

func (h *KeyHandler) RegisterRoutes(rg *gin.RouterGroup) {
	keys := rg.Group("/keys")
	{
		keys.POST("", requireRoles(adminKey), h.create)
		keys.DELETE("/:id", requireRoles(adminKey), h.delete)
		keys.GET("", h.list)
		keys.GET("/:id", h.getByID)
		keys.PUT("/:id", requireRoles(adminKey), h.update)
		keys.POST("/:id/issue", h.issue)
		keys.POST("/:id/return", h.returnKey)
		keys.POST("/:id/lost", requireRoles(adminKey), h.markLost)
		keys.GET("/:id/history", h.history)
		keys.GET("/:id/holder", h.getCurrentHolder)
	}
}

func (h *KeyHandler) create(c *gin.Context) {
	var req CreateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	key := &models.Key{
		KeyNumber:       req.KeyNumber,
		RoomDescription: req.RoomDescription,
		Status:          models.KeyStatusAvailable,
		Notes:           req.Notes,
	}

	if err := h.svc.Create(c.Request.Context(), key); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToKeyResponse(key))
}

func (h *KeyHandler) getByID(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	key, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToKeyResponse(key))
}

func (h *KeyHandler) list(c *gin.Context) {
	statusFilter := c.Query("status")

	var keys []models.Key
	var err error

	if statusFilter != "" {
		keys, err = h.svc.ListByStatus(c.Request.Context(), models.KeyStatus(statusFilter))
	} else {
		keys, err = h.svc.ListAll(c.Request.Context())
	}

	if err != nil {
		handleError(c, err)
		return
	}

	resp := make([]KeyResponse, 0, len(keys))
	for i := range keys {
		resp = append(resp, ToKeyResponse(&keys[i]))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *KeyHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	key, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	key.KeyNumber = req.KeyNumber
	key.RoomDescription = req.RoomDescription
	key.Notes = req.Notes
	if req.Status != "" {
		key.Status = models.KeyStatus(req.Status)
	}

	if err := h.svc.Update(c.Request.Context(), key); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToKeyResponse(key))
}

func (h *KeyHandler) issue(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req IssueKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}

	if c.GetString(roleKey) != adminKey && req.UserID != c.GetString(userIDKey) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "можно взять ключ только на своё имя"})
		return
	}
	if err := h.svc.Issue(c.Request.Context(), id, req.UserID, comment); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "ключ выдан"})
}

func (h *KeyHandler) returnKey(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req ReturnKeyRequest
	_ = c.ShouldBindJSON(&req)

	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}

	var err error
	if c.GetString(roleKey) == adminKey {
		err = h.svc.Return(c.Request.Context(), id, comment)
	} else {
		err = h.svc.ReturnForUser(c.Request.Context(), id, c.GetString(userIDKey), comment)
	}
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "ключ возвращён"})
}

func (h *KeyHandler) markLost(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req struct {
		Comment *string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&req)

	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}

	if err := h.svc.MarkLost(c.Request.Context(), id, comment); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "ключ помечен как утерянный"})
}

func (h *KeyHandler) history(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	logs, err := h.svc.HistoryForKey(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	resp := make([]KeyLogResponse, 0, len(logs))
	for i := range logs {
		resp = append(resp, ToKeyLogResponse(&logs[i]))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *KeyHandler) getCurrentHolder(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	holder, err := h.svc.GetCurrentHolder(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	if holder == nil {
		c.JSON(http.StatusOK, MessageResponse{Message: "ключ свободен"})
		return
	}

	c.JSON(http.StatusOK, ToKeyLogResponse(holder))
}

func (h *KeyHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "ключ удалён"})
}
