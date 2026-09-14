package handler

import (
	"context"
	"embed"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"mitm-departament/internal/service"
	"mitm-departament/pkg/ratelimiter"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserService — интерфейс сервиса пользователей
type UserService interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	ListActive(ctx context.Context, roleFilter string) ([]models.User, error)
	ListAll(ctx context.Context) ([]models.User, error)
	SetAvatar(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader, ext string) error
	DeleteAvatar(ctx context.Context, userID string) error
	Update(ctx context.Context, u *models.User) error
	Activate(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
}

// KeyService — интерфейс сервиса ключей
type KeyService interface {
	Create(ctx context.Context, k *models.Key) error
	GetByID(ctx context.Context, id int64) (*models.Key, error)
	GetByKeyNumber(ctx context.Context, keyNumber string) (*models.Key, error)
	ListAll(ctx context.Context) ([]models.Key, error)
	ListByStatus(ctx context.Context, status models.KeyStatus) ([]models.Key, error)
	Update(ctx context.Context, k *models.Key) error
	Delete(ctx context.Context, id int64) error
	ReturnForUser(ctx context.Context, keyID int64, userID, comment string) error
	Issue(ctx context.Context, keyID int64, userID string, comment string) error
	Return(ctx context.Context, keyID int64, comment string) error
	MarkLost(ctx context.Context, keyID int64, actorID, comment string) error
	RestoreLost(ctx context.Context, keyID int64, actorID, comment string) error
	HistoryForKey(ctx context.Context, keyID int64) ([]models.KeyLog, error)
	HistoryForUser(ctx context.Context, userID string) ([]models.KeyLog, error)
	GetCurrentHolder(ctx context.Context, keyID int64) (*models.KeyLog, error)
	HolderInfo(ctx context.Context, keyID int64) (*models.KeyLog, error)
	Scan(ctx context.Context, keyID int64, actor service.ScanActor) (*service.ScanOutcome, error)
}

type Handler struct {
	workspace *WorkspaceHandler
	auth      *AuthHandler
	article   *ArticleHandler
	user      *UserHandler
	profile   *ProfileHandler
	key       *KeyHandler
	inventory *InventoryHandler
	photo     *InventoryPhotoHandler
	event     *EventHandler
	userSvc   UserService
	log       *zap.Logger

	rateLimiter     *ratelimiter.RateLimiter
	frontendFS      embed.FS
	frontendFSReady bool
}

func New(authSvc AuthService, articleSvc ArticleService, userSvc UserService, keySvc KeyService, InventorySvc InventoryService, photoSvc InventoryPhotoService, eventSvc EventService, cfg *config.Config, log *zap.Logger) *Handler {
	rateLimiter := ratelimiter.NewRateLimiter(20, 10)
	return &Handler{
		auth:      NewAuthHandler(authSvc, cfg.Auth, log),
		article:   NewArticleHandler(articleSvc),
		user:      NewUserHandler(userSvc, keySvc, cfg.Photo),
		profile:   NewProfileHandler(userSvc, cfg.Photo),
		key:       NewKeyHandler(keySvc),
		inventory: NewInventoryHandler(InventorySvc),
		photo:     NewPhotoHandler(photoSvc, cfg.Photo),
		event:     NewEventHandler(eventSvc, userSvc),
		userSvc:   userSvc,
		log:       log,

		rateLimiter: rateLimiter,
	}
}

// SetFrontendFS устанавливает встроенную файловую систему фронтенда
func (h *Handler) SetFrontendFS(fs embed.FS) {
	h.frontendFS = fs
	h.frontendFSReady = true
}

// InitRoutes настраивает все маршруты
func (h *Handler) InitRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		gin.Recovery(),
		h.logging(),
		h.rateLimitMiddleware(h.rateLimiter),
	)

	// 1. Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ── Публичные API-роуты (БЕЗ auth middleware) ──
	public := router.Group("/api/v1")
	{
		h.auth.RegisterRoutes(public)
		public.POST("/auth/register", h.user.Register)
		if h.workspace != nil {
			// Ключ по QR: сотрудник определяется по токену, гость — по метке браузера.
			h.workspace.RegisterPublicRoutes(public, h.optionalAuth)
			public.POST("/public/keys/:public_id/scan", h.optionalAuth, h.workspace.ScanKey)
		}
	}

	// ── Защищённые API-роуты (С auth middleware) ──
	protected := router.Group("/api/v1", h.authMiddleware)
	{
		h.photo.RegisterPublicRoutes(protected)
		h.profile.RegisterPublicRoutes(protected)
		h.inventory.RegisterPublicRoutes(protected)
		if h.workspace != nil {
			h.workspace.RegisterRoutes(protected)
		}
		h.profile.RegisterRoutes(protected)
		h.article.RegisterRoutes(protected)
		h.key.RegisterRoutes(protected)
		h.inventory.RegisterRoutes(protected)
		h.photo.RegisterRoutes(protected)
		h.user.RegisterRoutes(protected)
		h.event.RegisterRoutes(protected)
	}

	// 3. Раздача фронтенда (SPA)
	router.NoRoute(h.serveFrontend)

	return router
}

func (h *Handler) serveFrontend(c *gin.Context) {
	path := c.Request.URL.Path

	// Игнорируем API-запросы
	if strings.HasPrefix(path, "/api/") {
		c.Status(http.StatusNotFound)
		return
	}

	if !h.frontendFSReady {
		c.Status(http.StatusNotFound)
		return
	}

	// Нормализуем путь: убираем ведущий слеш
	filePath := strings.TrimPrefix(path, "/")

	// Корень → index.html
	if filePath == "" {
		filePath = "index.html"
	}

	// Пытаемся прочитать файл из embed (файлы лежат в frontend/)
	fullPath := "frontend/" + filePath

	data, err := h.frontendFS.ReadFile(fullPath)
	if err == nil {
		contentType := getContentType(filePath)
		// Кэшируем статику, но не HTML
		if strings.HasSuffix(filePath, ".html") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		} else {
			c.Header("Cache-Control", "public, max-age=86400")
		}
		c.Data(http.StatusOK, contentType, data)
		return
	}

	// SPA fallback: если путь без расширения — отдаём index.html
	if !strings.Contains(filePath, ".") {
		indexData, errIndex := h.frontendFS.ReadFile("frontend/index.html")
		if errIndex == nil {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
			return
		}
	}

	c.Status(http.StatusNotFound)
}

func getContentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(path, ".woff"), strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
