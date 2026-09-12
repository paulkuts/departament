package repository

import (
	"context"
	"database/sql"
	"fmt"
	"mitm-departament/internal/models"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type ArticleRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewArticleRepo(db *sqlx.DB, log *zap.Logger) *ArticleRepo {
	return &ArticleRepo{db: db, log: log}
}

const articleCols = `id, title, details, indexing, white_list_level, funding, link, status, created_by, created_at, updated_at`

func (r *ArticleRepo) Create(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO articles (title, details, indexing, white_list_level, funding, link, status, created_by)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Title, a.Details, a.Indexing, a.WhiteListLevel, a.Funding, a.Link, a.Status, a.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert article: %w", err)
	}
	id, _ := res.LastInsertId()
	a.ID = id

	if err := insertAuthors(ctx, tx, id, authors); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ArticleRepo) Update(ctx context.Context, a *models.Article, authors []models.ArticleAuthor) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE articles SET title=?, details=?, indexing=?, white_list_level=?, funding=?, link=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		a.Title, a.Details, a.Indexing, a.WhiteListLevel, a.Funding, a.Link, a.Status, a.ID,
	)
	if err != nil {
		return fmt.Errorf("update article: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM article_authors WHERE article_id=?`, a.ID); err != nil {
		return fmt.Errorf("clear authors: %w", err)
	}
	if err := insertAuthors(ctx, tx, a.ID, authors); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAuthors(ctx context.Context, tx *sqlx.Tx, articleID int64, authors []models.ArticleAuthor) error {
	for i, au := range authors {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO article_authors (article_id, user_id, name, sort_order) VALUES (?, ?, ?, ?)`,
			articleID, au.UserID, au.Name, i,
		)
		if err != nil {
			return fmt.Errorf("insert author: %w", err)
		}
	}
	return nil
}

func (r *ArticleRepo) GetByID(ctx context.Context, id int64) (*models.Article, error) {
	a := &models.Article{}
	err := r.db.GetContext(ctx, a, `SELECT `+articleCols+` FROM articles WHERE id=?`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get article: %w", err)
	}
	return a, nil
}

func (r *ArticleRepo) AuthorsByArticle(ctx context.Context, articleID int64) ([]models.ArticleAuthor, error) {
	var authors []models.ArticleAuthor
	err := r.db.SelectContext(ctx, &authors,
		`SELECT id, article_id, user_id, name, sort_order FROM article_authors WHERE article_id=? ORDER BY sort_order`, articleID)
	return authors, err
}

func (r *ArticleRepo) List(ctx context.Context, f models.ArticleFilter) ([]models.Article, []models.ArticleAuthor, int64, error) {
	var where []string
	var args []interface{}

	if f.Search != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Author != "" {
		where = append(where, "id IN (SELECT article_id FROM article_authors WHERE name LIKE ?)")
		args = append(args, "%"+f.Author+"%")
	}
	if f.AuthorID != "" {
		where = append(where, "id IN (SELECT article_id FROM article_authors WHERE user_id = ?)")
		args = append(args, f.AuthorID)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM articles `+whereClause, args...); err != nil {
		return nil, nil, 0, fmt.Errorf("count articles: %w", err)
	}

	var articles []models.Article
	listQuery := fmt.Sprintf(
		`SELECT %s FROM articles %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, articleCols, whereClause)
	listArgs := append(append([]interface{}{}, args...), f.Limit, f.Offset)
	if err := r.db.SelectContext(ctx, &articles, listQuery, listArgs...); err != nil {
		return nil, nil, 0, fmt.Errorf("list articles: %w", err)
	}

	var authors []models.ArticleAuthor
	if len(articles) > 0 {
		ids := make([]int64, len(articles))
		for i, a := range articles {
			ids[i] = a.ID
		}
		q, inArgs, _ := sqlx.In(
			`SELECT id, article_id, user_id, name, sort_order FROM article_authors WHERE article_id IN (?) ORDER BY sort_order`, ids)
		q = r.db.Rebind(q)
		if err := r.db.SelectContext(ctx, &authors, q, inArgs...); err != nil {
			return nil, nil, 0, fmt.Errorf("list authors: %w", err)
		}
	}

	return articles, authors, total, nil
}

func (r *ArticleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM articles WHERE id=?`, id) // авторы удалятся по CASCADE
	return err
}
