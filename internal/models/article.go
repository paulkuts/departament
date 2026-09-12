package models

import "time"

type Article struct {
	ID             int64     `json:"id" db:"id"`
	Title          string    `json:"title" db:"title"`
	Details        *string   `json:"details,omitempty" db:"details"`
	Indexing       *string   `json:"indexing,omitempty" db:"indexing"`
	WhiteListLevel *string   `json:"white_list_level,omitempty" db:"white_list_level"`
	Funding        *string   `json:"funding,omitempty" db:"funding"`
	Link           *string   `json:"link,omitempty" db:"link"`
	Status         string    `json:"status" db:"status"`
	CreatedBy      *string   `json:"created_by,omitempty" db:"created_by"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

type ArticleAuthor struct {
	ID        int64   `json:"id" db:"id"`
	ArticleID int64   `json:"article_id" db:"article_id"`
	UserID    *string `json:"user_id,omitempty" db:"user_id"`
	Name      string  `json:"name" db:"name"`
	SortOrder int     `json:"sort_order" db:"sort_order"`
}

type ArticleFilter struct {
	Search   string `form:"search"`
	Status   string `form:"status"`
	Author   string `form:"author"`
	AuthorID string `form:"author_id"` // ← НОВОЕ: фильтр по ID автора
	Paginated
}

type ListArticles struct {
	PaginatedMetadata PaginatedMetadata
	Articles          []Article
	Authors           []ArticleAuthor // все авторы перечисленных статей
}
