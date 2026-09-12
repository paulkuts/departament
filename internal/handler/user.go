package handler

import (
	"fmt"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userSvc UserService
	keySvc  KeyService
	cfg     config.PhotoConfig
}

func NewUserHandler(userSvc UserService, keySvc KeyService, cfg config.PhotoConfig) *UserHandler {
	_ = os.MkdirAll(cfg.AvatarPhotoDir, 0755)
	return &UserHandler{
		userSvc: userSvc,
		keySvc:  keySvc,
		cfg:     cfg,
	}
}

func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	users := rg.Group("/users")
	{
		users.POST("", requireRoles(adminKey), h.Create)
		users.GET("", h.ListAll)           // возвращает всех пользователей (активных и неактивных)
		users.GET("/active", h.ListActive) // возвращает только активных пользователей
		users.GET("/:id", h.GetByID)
		users.PUT("/:id", requireRoles(adminKey), h.Update)
		users.DELETE("/:id", requireRoles(adminKey), h.Deactivate)
		users.POST("/:id/activate", requireRoles(adminKey), h.Activate)
		users.GET("/:id/history", h.History)
		users.POST("/:id/avatar", requireRoles(adminKey), h.UploadAvatar)
		users.DELETE("/:id/avatar", requireRoles(adminKey), h.DeleteAvatar)
	}
}

func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	user := &models.User{
		Avatar:   req.Avatar,
		FullName: req.FullName,
		Password: req.Password,
		Role:     req.Role,
		Position: req.Position,
		Phone:    req.Phone,
		Email:    req.Email,
		IsActive: true,
	}

	// Парсим дату рождения если указана: в базу пишем TEXT 'YYYY-MM-DD'
	if req.DateOfBirth != nil && *req.DateOfBirth != "" {
		if _, err := time.Parse("2006-01-02", *req.DateOfBirth); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "некорректная дата рождения, ожидается формат ГГГГ-ММ-ДД"})
			return
		}
		user.DateOfBirth = req.DateOfBirth
	}

	// Устанавливаем кабинет если указан
	user.Office = req.Office

	if err := h.userSvc.Create(c.Request.Context(), user); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ToUserResponse(user))
}

func (h *UserHandler) GetByID(c *gin.Context) {
	id := c.Param("id")

	user, err := h.userSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToUserResponse(user))
}

func (h *UserHandler) ListActive(c *gin.Context) {
	// Игнорируем параметр role, всегда возвращаем всех активных кроме студентов
	users, err := h.userSvc.ListActive(c.Request.Context(), "")
	if err != nil {
		handleError(c, err)
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for i := range users {
		resp = append(resp, ToUserResponse(&users[i]))
	}

	c.JSON(http.StatusOK, resp)
}

// ListAll возвращает всех пользователей (активных и неактивных)
func (h *UserHandler) ListAll(c *gin.Context) {
	users, err := h.userSvc.ListAll(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for i := range users {
		resp = append(resp, ToUserResponse(&users[i]))
	}

	c.JSON(http.StatusOK, resp)
}

// Activate активирует пользователя
func (h *UserHandler) Activate(c *gin.Context) {
	id := c.Param("id")

	if err := h.userSvc.Activate(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "пользователь активирован"})
}

func (h *UserHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	user, err := h.userSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	user.FullName = req.FullName
	user.Role = req.Role
	user.Phone = req.Phone
	// Почта обязательна в базе: пустое или отсутствующее поле означает «не менять»,
	// иначе профиль падает с NOT NULL constraint failed: users.email
	if req.Email != nil && strings.TrimSpace(*req.Email) != "" {
		user.Email = req.Email
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	user.Position = req.Position
	if req.Avatar != nil {
		user.Avatar = req.Avatar
	}

	// Обновляем дату рождения если указана: в базу пишем TEXT 'YYYY-MM-DD'
	if req.DateOfBirth != nil && *req.DateOfBirth != "" {
		if _, err := time.Parse("2006-01-02", *req.DateOfBirth); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "некорректная дата рождения, ожидается формат ГГГГ-ММ-ДД"})
			return
		}
		user.DateOfBirth = req.DateOfBirth
	} else if req.DateOfBirth != nil {
		user.DateOfBirth = nil
	}

	// Обновляем кабинет
	user.Office = req.Office

	if err := h.userSvc.Update(c.Request.Context(), user); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ToUserResponse(user))
}

func (h *UserHandler) Deactivate(c *gin.Context) {
	id := c.Param("id")

	if err := h.userSvc.Deactivate(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "пользователь деактивирован"})
}

func (h *UserHandler) History(c *gin.Context) {
	id := c.Param("id")

	if _, err := h.userSvc.GetByID(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	logs, err := h.keySvc.HistoryForUser(c.Request.Context(), id)
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

func (h *UserHandler) UploadAvatar(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		handleError(c, fmt.Errorf("empty id param"))
		return
	}

	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "поле 'avatar' обязательно"})
		return
	}
	defer file.Close()

	if header.Size > int64(h.cfg.MaxPhotoSize) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "файл слишком большой (макс. 5 МБ)"})
		return
	}

	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedTypes[contentType] // из photo.go
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "допустимы: JPEG, PNG, WebP, GIF"})
		return
	}

	if err := h.userSvc.SetAvatar(c.Request.Context(), id, file, header, ext); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"avatar": "/api/v1/avatars/" + id})
}

func (h *UserHandler) DeleteAvatar(c *gin.Context) {
	id := c.Param("id")

	if err := h.userSvc.DeleteAvatar(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, MessageResponse{Message: "аватар удалён"})
}

func (h *UserHandler) Register(c *gin.Context) {
	var req struct {
		FullName string `json:"full_name" binding:"required,min=3,max=200"`
		Email    string `json:"email" binding:"required,email,max=254"`
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}
	req.FullName = strings.TrimSpace(req.FullName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if len([]rune(req.FullName)) < 3 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "укажите полное имя"})
		return
	}
	u := &models.User{FullName: req.FullName, Email: &req.Email, Password: req.Password, Role: "staff", IsActive: true}
	if err := h.userSvc.Create(c.Request.Context(), u); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ToUserResponse(u))
}
