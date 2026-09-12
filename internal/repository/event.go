package repository

import (
	"context"
	"errors"
	"fmt"
	"mitm-departament/internal/models"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type EventRepo struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewEventRepository(db *sqlx.DB, log *zap.Logger) *EventRepo {
	return &EventRepo{
		db:  db,
		log: log,
	}
}

func (r *EventRepo) CreateEvent(ctx context.Context, event models.Event) (int64, error) {
	var id int64
	err := r.db.GetContext(ctx, &id,
		`INSERT INTO events (creator_id, title, description, location, start_time, is_public) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		event.CreatorID,
		event.Title,
		event.Description,
		event.Location,
		event.StartTime,
		event.IsPublic,
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}

	return id, nil
}

func (r *EventRepo) Event(ctx context.Context, id int64) (*models.Event, error) {
	event := &models.Event{}
	err := r.db.GetContext(ctx, event,
		`SELECT id, creator_id, title, description, location, start_time, is_public, created_at, updated_at
         FROM events WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	return event, nil
}

func (r *EventRepo) EventsList(ctx context.Context, f models.EventFilter) ([]models.Event, int64, error) {
	var (
		conditions []string
		args       []interface{}
	)

	if f.CreatorID != nil && *f.CreatorID != "" {
		// Если указан creator_id, показываем все события этого пользователя (и публичные, и приватные)
		conditions = append(conditions, "creator_id = ?")
		args = append(args, *f.CreatorID)
	} else if f.IsPublic == nil {
		// Если creator_id не указан и IsPublic не задан явно, показываем только публичные события
		conditions = append(conditions, "is_public = 1")
	} else {
		// Если IsPublic задан явно, используем этот фильтр
		conditions = append(conditions, "is_public = ?")
		args = append(args, *f.IsPublic)
	}

	if f.Title != nil {
		conditions = append(conditions, "title LIKE ?")
		args = append(args, "%"+*f.Title+"%") // содержит
	}

	if f.Location != nil {
		conditions = append(conditions, "location LIKE ?")
		args = append(args, "%"+*f.Location+"%") // содержит
	}

	if f.FirstDate != nil {
		conditions = append(conditions, "start_time >= ?")
		args = append(args, *f.FirstDate)
	}

	if f.LastDate != nil {
		conditions = append(conditions, "start_time <= ?")
		args = append(args, *f.LastDate)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 1. Общее количество записей под фильтр
	var total int64
	countQuery := `SELECT COUNT(*) FROM events` + whereClause
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	if total == 0 {
		return nil, 0, nil
	}

	listArgs := make([]interface{}, 0, len(args)+2)
	listArgs = append(listArgs, args...)
	listArgs = append(listArgs, f.Paginated.Limit, f.Paginated.Offset)

	events := make([]models.Event, 0, f.Paginated.Limit)
	query := `SELECT id, creator_id, title, description, location, start_time, is_public, created_at, updated_at FROM events` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	if err := r.db.SelectContext(ctx, &events, query, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}

	return events, total, nil
}

func (r *EventRepo) UpdateEvent(ctx context.Context, event *models.Event, id int64) error {
	res, err := r.db.NamedExecContext(ctx,
		`UPDATE events SET 
			title = :title,
			description = :description,
			location = :location,
			start_time = :start_time,
			is_public = :is_public,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = :id
		`, event)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("event not found")
	}
	return nil
}

func (r *EventRepo) DeleteEvent(ctx context.Context, eventID int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", eventID)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("inventory not found")
	}
	return nil
}
