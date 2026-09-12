package models

import "time"

type Event struct {
	ID          int64      `json:"id" db:"id"`
	CreatorID   string     `json:"creator_id" db:"creator_id"`
	Title       *string    `json:"title" db:"title"`
	Location    *string    `json:"location" db:"location"`
	Description *string    `json:"description" db:"description"`
	StartTime   *time.Time `json:"start_time" db:"start_time"`
	IsPublic    bool       `json:"is_public" db:"is_public"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

type UpdateEvent struct {
	Title       *string    `json:"title" db:"title"`
	Location    *string    `json:"location" db:"location"`
	Description *string    `json:"description" db:"description"`
	StartTime   *time.Time `json:"start_time" db:"start_time"`
	IsPublic    *bool      `json:"is_public" db:"is_public"`
}

type EventFilter struct {
	Title     *string `form:"title"`
	Location  *string `form:"location"`
	FirstDate *string `form:"first_date"`
	LastDate  *string `form:"last_date"`
	CreatorID *string `form:"creator_id"`
	IsPublic  *bool   `form:"is_public"`
	Paginated Paginated
}

type EventList struct {
	Events            []Event           `json:"events"`
	PaginatedMetadata PaginatedMetadata `json:"paginated_metadata"`
}
