package service

import (
	"context"
	"fmt"
	"mitm-departament/internal/models"

	"go.uber.org/zap"
)

type ArticleRepository interface {
	Create(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error
	Update(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error
	GetByID(ctx context.Context, id int64) (*models.Article, error)
	AuthorsByArticle(ctx context.Context, articleID int64) ([]models.ArticleAuthor, error)
	List(ctx context.Context, f models.ArticleFilter) ([]models.Article, []models.ArticleAuthor, int64, error)
	Delete(ctx context.Context, id int64) error
}

type ArticleS struct {
	repo ArticleRepository
	log  *zap.Logger
}

func NewArticleService(repo ArticleRepository, log *zap.Logger) *ArticleS {
	return &ArticleS{repo: repo, log: log}
}

func (s *ArticleS) Create(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error {
	return s.repo.Create(ctx, a, authors)
}

func (s *ArticleS) Update(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error {
	return s.repo.Update(ctx, a, authors)
}

func (s *ArticleS) GetByID(ctx context.Context, id int64) (*models.Article, []models.ArticleAuthor, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if a == nil {
		return nil, nil, fmt.Errorf("статья не найдена")
	}
	authors, err := s.repo.AuthorsByArticle(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return a, authors, nil
}

func (s *ArticleS) List(ctx context.Context, f models.ArticleFilter) (models.ListArticles, error) {
	articles, authors, total, err := s.repo.List(ctx, f)
	if err != nil {
		return models.ListArticles{}, err
	}
	return models.ListArticles{
		PaginatedMetadata: models.MakePaginatedMetadata(f.Limit, f.Offset, total),
		Articles:          articles,
		Authors:           authors,
	}, nil
}

func (s *ArticleS) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
