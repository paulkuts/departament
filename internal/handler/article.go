package handler

import (
	"context"
	"mitm-departament/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ArticleService interface {
	Create(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error
	Update(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error
	GetByID(ctx context.Context, id int64) (*models.Article, []models.ArticleAuthor, error)
	List(ctx context.Context, f models.ArticleFilter) (models.ListArticles, error)
	Delete(ctx context.Context, id int64) error
}

type ArticleHandler struct {
	svc ArticleService
}

func NewArticleHandler(svc ArticleService) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

func (h *ArticleHandler) RegisterRoutes(rg *gin.RouterGroup) {
	articles := rg.Group("/articles")
	{
		articles.GET("", h.list)
		articles.GET("/:id", h.getByID)
		articles.POST("", h.create)
		articles.PUT("/:id", h.requireOwner, h.update)
		articles.DELETE("/:id", h.requireOwner, h.delete)
	}
}

func ToArticleResponse(a *models.Article, authors []models.ArticleAuthor) ArticleResponse {
	list := make([]ArticleAuthorResponse, 0, len(authors))
	for _, au := range authors {
		list = append(list, ArticleAuthorResponse{ID: au.ID, UserID: au.UserID, Name: au.Name})
	}
	return ArticleResponse{
		ID: a.ID, Title: a.Title, Details: a.Details, Indexing: a.Indexing,
		WhiteListLevel: a.WhiteListLevel, Funding: a.Funding, Link: a.Link,
		Status: a.Status, Authors: list, CreatedBy: a.CreatedBy,
		CreatedAt: a.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: a.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func toAuthorsModel(dtos []ArticleAuthorDTO) []models.ArticleAuthor {
	authors := make([]models.ArticleAuthor, 0, len(dtos))
	for i, d := range dtos {
		authors = append(authors, models.ArticleAuthor{UserID: d.UserID, Name: d.Name, SortOrder: i})
	}
	return authors
}

// ─── Handlers ───
func (h *ArticleHandler) create(c *gin.Context) {
	var req CreateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	var createdBy *string
	if uid, err := getUserID(c); err == nil {
		s := uid.String()
		createdBy = &s
	}

	article := &models.Article{
		Title: req.Title, Details: req.Details, Indexing: req.Indexing,
		WhiteListLevel: req.WhiteListLevel, Funding: req.Funding, Link: req.Link,
		Status: req.Status, CreatedBy: createdBy,
	}

	if err := h.svc.Create(c.Request.Context(), article, toAuthorsModel(req.Authors)); err != nil {
		handleError(c, err)
		return
	}

	a, authors, _ := h.svc.GetByID(c.Request.Context(), article.ID)
	c.JSON(http.StatusCreated, ToArticleResponse(a, authors))
}

func (h *ArticleHandler) update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateArticleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleValidationError(c, err)
		return
	}

	article := &models.Article{
		ID: id, Title: req.Title, Details: req.Details, Indexing: req.Indexing,
		WhiteListLevel: req.WhiteListLevel, Funding: req.Funding, Link: req.Link, Status: req.Status,
	}

	if err := h.svc.Update(c.Request.Context(), article, toAuthorsModel(req.Authors)); err != nil {
		handleError(c, err)
		return
	}

	a, authors, _ := h.svc.GetByID(c.Request.Context(), id)
	c.JSON(http.StatusOK, ToArticleResponse(a, authors))
}

func (h *ArticleHandler) getByID(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	a, authors, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ToArticleResponse(a, authors))
}

func (h *ArticleHandler) list(c *gin.Context) {
	var filter models.ArticleFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		handleValidationError(c, err)
		return
	}
	filter.Paginated.Validate()

	data, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		handleError(c, err)
		return
	}

	byArticle := make(map[int64][]models.ArticleAuthor)
	for _, au := range data.Authors {
		byArticle[au.ArticleID] = append(byArticle[au.ArticleID], au)
	}

	resp := make([]ArticleResponse, 0, len(data.Articles))
	for i := range data.Articles {
		resp = append(resp, ToArticleResponse(&data.Articles[i], byArticle[data.Articles[i].ID]))
	}

	c.JSON(http.StatusOK, gin.H{
		"paginated_metadata": data.PaginatedMetadata,
		"articles":           resp,
	})
}

func (h *ArticleHandler) delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "статья удалена"})
}

func (h *ArticleHandler) requireOwner(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		c.Abort()
		return
	}
	a, _, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		c.Abort()
		return
	}
	if c.GetString(roleKey) != adminKey && (a == nil || a.CreatedBy == nil || *a.CreatedBy != c.GetString(userIDKey)) {
		c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "изменять статью может её создатель или администратор"})
		return
	}
	c.Next()
}
