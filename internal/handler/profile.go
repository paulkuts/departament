package handler

import (
	"context"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type ProfileService interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
}

type ProfileHandler struct {
	svc ProfileService
	cfg config.PhotoConfig
}

func NewProfileHandler(svc ProfileService, cfg config.PhotoConfig) *ProfileHandler {
	_ = os.MkdirAll(cfg.AvatarPhotoDir, 0755)
	return &ProfileHandler{svc: svc, cfg: cfg}
}

// Защищённые маршруты
func (h *ProfileHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", h.me)
}

// Публичный маршрут — отдача файла (для <img src>)
func (h *ProfileHandler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	rg.GET("/avatars/:user_id", h.serveAvatar)
}

func (h *ProfileHandler) me(c *gin.Context) {
	uid, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "не авторизован"})
		return
	}
	user, err := h.svc.GetByID(c.Request.Context(), uid.String())
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, ToUserResponse(user))
}

func (h *ProfileHandler) serveAvatar(c *gin.Context) {
	userID := c.Param("user_id")
	user, err := h.svc.GetByID(c.Request.Context(), userID)
	if err != nil || user == nil || user.Avatar == nil || *user.Avatar == "" {
		c.Status(http.StatusNotFound)
		return
	}
	filePath := filepath.Join(h.cfg.AvatarPhotoDir, *user.Avatar)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.File(filePath)
}
